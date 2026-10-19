package safehttp

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func isolatedDependencies(t *testing.T) clientDependencies {
	t.Helper()
	return clientDependencies{
		lookupNetIP: func(context.Context, string, string) ([]netip.Addr, error) {
			t.Error("unexpected system lookup")
			return nil, errors.New("unexpected lookup")
		},
		dialContext: func(context.Context, string, string) (net.Conn, error) {
			t.Error("unexpected dial (real network is forbidden)")
			return nil, errors.New("unexpected dial")
		},
		dohTransport: func(dohProvider) http.RoundTripper {
			t.Error("unexpected DoH fallback")
			return roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("unexpected DoH")
			})
		},
	}
}

func addresses(values ...string) []netip.Addr {
	result := make([]netip.Addr, len(values))
	for i, value := range values {
		result[i] = netip.MustParseAddr(value)
	}
	return result
}

func assertKind(t *testing.T, err error, want ErrorKind) {
	t.Helper()
	if got := ErrorKindOf(err); got != want || (want != "" && err == nil) {
		t.Fatalf("error kind = %q, want %q; error = %v", got, want, err)
	}
}

func assertSecretFree(t *testing.T, err error, secrets ...string) {
	t.Helper()
	for current := err; current != nil; current = errors.Unwrap(current) {
		for _, secret := range secrets {
			if strings.Contains(current.Error(), secret) {
				t.Fatalf("error retained a secret canary")
			}
		}
		if current != context.Canceled && current != context.DeadlineExceeded {
			if _, ok := current.(*FetchError); !ok {
				t.Fatalf("error retained unsafe upstream type %T", current)
			}
		}
	}
}

func response(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), ContentLength: -1}
}

func dohMessage(host string, kind int, values ...string) dohResponse {
	status := 0
	msg := dohResponse{Status: &status, Question: []dohQuestion{{Name: host + ".", Type: kind}}}
	for _, value := range values {
		msg.Answer = append(msg.Answer, dohRecord{Name: host + ".", Type: kind, Data: value})
	}
	return msg
}

func dohJSON(msg dohResponse) string {
	body, err := json.Marshal(msg)
	if err != nil {
		panic(err)
	}
	return string(body)
}

func publicDoH(t *testing.T, provider dohProvider) http.RoundTripper {
	t.Helper()
	return roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != provider.host || req.URL.Scheme != "https" || req.URL.Path != provider.path {
			t.Error("DoH request used an unexpected provider endpoint")
		}
		kind := 1
		values := []string{"8.8.4.4"}
		if req.URL.Query().Get("type") == "28" {
			kind = 28
			values = []string{"2606:4700:4700::1111"}
		}
		return response(dohJSON(dohMessage(req.URL.Query().Get("name"), kind, values...))), nil
	})
}

// A locally generated CA/leaf avoids cert-verification bypasses and uses no
// external hostname/IP even when testing the numeric production dial boundary.
func localTLSServer(t *testing.T, names []string, handler http.Handler) (*httptest.Server, *x509.CertPool) {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "safehttp local test"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		DNSNames: names, IsCA: true, BasicConstraintsValid: true,
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, pub, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	s := httptest.NewUnstartedServer(handler)
	s.Config.ErrorLog = log.New(io.Discard, "", 0)
	s.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
	s.StartTLS()
	t.Cleanup(s.Close)
	return s, roots
}

func baseTransport(client *http.Client) *http.Transport {
	return client.Transport.(*guardedTransport).base.(*http.Transport)
}

type timeoutError struct{}

func (timeoutError) Error() string   { return "SECRET_TIMEOUT_CANARY" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }
