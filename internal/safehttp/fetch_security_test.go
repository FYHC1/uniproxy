package safehttp

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestFetchSanitizedErrorCategoryMap(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want ErrorKind
	}{
		{name: "DNS", err: &FetchError{Kind: KindDNS}, want: KindDNS},
		{name: "private", err: &FetchError{Kind: KindPrivateAddress}, want: KindPrivateAddress},
		{name: "fake fallback", err: &FetchError{Kind: KindFakeIPFallback, cause: failure(KindDNS)}, want: KindFakeIPFallback},
		{name: "connect", err: &FetchError{Kind: KindConnect}, want: KindConnect},
		{name: "connect timeout", err: &FetchError{Kind: KindConnectTimeout, cause: context.DeadlineExceeded}, want: KindConnectTimeout},
		{name: "TLS typed", err: &FetchError{Kind: KindTLS}, want: KindTLS},
		{name: "TLS raw certificate", err: x509.HostnameError{Certificate: &x509.Certificate{}, Host: "SECRET_HOST_CANARY"}, want: KindTLS},
		{name: "TLS record", err: tls.RecordHeaderError{Msg: "SECRET_RECORD_CANARY"}, want: KindTLS},
		{name: "redirect", err: &FetchError{Kind: KindRedirect}, want: KindRedirect},
		{name: "HTTP", err: &FetchError{Kind: KindHTTPStatus, StatusCode: 403}, want: KindHTTPStatus},
		{name: "read", err: &FetchError{Kind: KindBodyRead}, want: KindBodyRead},
		{name: "size", err: &FetchError{Kind: KindBodyTooLarge}, want: KindBodyTooLarge},
		{name: "HTML", err: &FetchError{Kind: KindHTML}, want: KindHTML},
		{name: "request timeout", err: timeoutError{}, want: KindTimeout},
		{name: "deadline", err: fmt.Errorf("SECRET_ERROR_CANARY: %w", context.DeadlineExceeded), want: KindTimeout},
		{name: "cancellation", err: fmt.Errorf("SECRET_ERROR_CANARY: %w", context.Canceled), want: KindCanceled},
		{name: "unknown", err: errors.New("https://SECRET_HOST_CANARY/PRIVATE_PATH_CANARY?token=SECRET_QUERY_CANARY"), want: KindDownload},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, &url.Error{Op: "SECRET_OP_CANARY", URL: "https://SECRET_HOST_CANARY/PRIVATE_PATH_CANARY?token=SECRET_QUERY_CANARY", Err: tt.err}
			})}
			_, err := Fetch(context.Background(), client, "https://source.example/PRIVATE_PATH_CANARY?token=SECRET_QUERY_CANARY")
			assertKind(t, err, tt.want)
			assertSecretFree(t, err, "CANARY", "source.example")
			for _, sentinel := range []error{context.Canceled, context.DeadlineExceeded} {
				if errors.Is(tt.err, sentinel) && !errors.Is(err, sentinel) {
					t.Fatal("safe error lost context sentinel")
				}
			}
		})
	}
	for _, code := range []int{401, 403, 404, 429, 500, 503} {
		client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			res := response("SECRET_BODY_CANARY")
			res.StatusCode = code
			res.Status = "SECRET_STATUS_CANARY"
			return res, nil
		})}
		_, err := Fetch(context.Background(), client, "https://source.example/PRIVATE_PATH_CANARY")
		assertKind(t, err, KindHTTPStatus)
		if !strings.Contains(err.Error(), strconv.Itoa(code)) {
			t.Fatal("HTTP status was not preserved")
		}
		assertSecretFree(t, err, "CANARY", "source.example")
	}
}

type brokenBody struct {
	err    error
	closed bool
}

func (b *brokenBody) Read([]byte) (int, error) { return 0, b.err }
func (b *brokenBody) Close() error             { b.closed = true; return nil }

func TestFetchReadFailuresPreserveContextAndCloseBody(t *testing.T) {
	for _, tt := range []struct {
		err  error
		want ErrorKind
	}{
		{errors.New("SECRET_BODY_CANARY"), KindBodyRead},
		{timeoutError{}, KindBodyRead},
		{fmt.Errorf("SECRET_BODY_CANARY: %w", context.Canceled), KindCanceled},
	} {
		body := &brokenBody{err: tt.err}
		client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			res := response("")
			res.Body = body
			return res, nil
		})}
		_, err := Fetch(context.Background(), client, "https://source.example/PRIVATE_CANARY")
		assertKind(t, err, tt.want)
		assertSecretFree(t, err, "CANARY", "source.example")
		if !body.closed {
			t.Fatal("body was not closed on read failure")
		}
	}
}

func TestFetchBodyLimitsDecodedGzipAndHTMLDetection(t *testing.T) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	io.Copy(writer, io.LimitReader(zeroReader{}, MaxBody+1))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/empty":
		case "/exact":
			io.Copy(w, io.LimitReader(zeroReader{}, MaxBody))
		case "/length":
			w.Header().Set("Content-Length", strconv.Itoa(MaxBody+1))
		case "/stream":
			w.(http.Flusher).Flush()
			io.Copy(w, io.LimitReader(zeroReader{}, MaxBody+1))
		case "/gzip":
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Set("Content-Length", strconv.Itoa(compressed.Len()))
			w.Write(compressed.Bytes())
		case "/bad-gzip":
			w.Header().Set("Content-Encoding", "gzip")
			io.WriteString(w, "SECRET_GZIP_CANARY")
		case "/html":
			w.Header().Set("Content-Type", "text/plain")
			io.WriteString(w, "\ufeff \n<!--comment--><!DOCTYPE HTML><html>SECRET_LOGIN_CANARY</html>")
		case "/mislabeled":
			w.Header().Set("Content-Type", "text/html")
			io.WriteString(w, "proxies: []")
		default:
			t.Error("unexpected fixture request")
		}
	}))
	defer s.Close()
	for _, tt := range []struct {
		path   string
		want   ErrorKind
		length int
	}{
		{"/empty", "", 0}, {"/exact", "", MaxBody}, {"/length", KindBodyTooLarge, 0}, {"/stream", KindBodyTooLarge, 0},
		{"/gzip", KindBodyTooLarge, 0}, {"/bad-gzip", KindBodyRead, 0}, {"/html", KindHTML, 0}, {"/mislabeled", "", len("proxies: []")},
	} {
		t.Run(tt.path, func(t *testing.T) {
			body, err := Fetch(context.Background(), NewClient(true), s.URL+tt.path)
			assertKind(t, err, tt.want)
			assertSecretFree(t, err, "CANARY")
			if err == nil && len(body) != tt.length {
				t.Fatalf("body bytes=%d want %d", len(body), tt.length)
			}
		})
	}
	for _, body := range []string{"<html><head></head></html>", "<!--a--> <BoDy class='x'>", "<!doctype html PUBLIC 'x'>", " \n\ufeff <html>login</html>"} {
		if !looksLikeHTML([]byte(body)) {
			t.Fatal("confirmed HTML was not detected")
		}
	}
	for _, body := range []string{"", "proxies: []", "<htmlish>not HTML", "node: '<html>label</html>'", "<!-- unclosed"} {
		if looksLikeHTML([]byte(body)) {
			t.Fatal("non-HTML body was rejected")
		}
	}
}

type zeroReader struct{}

func (zeroReader) Read(b []byte) (int, error) { clear(b); return len(b), nil }

func TestRedirectRevalidatesDNSNoRefererAndBound(t *testing.T) {
	for _, targetIP := range []string{"8.8.4.4", "127.0.0.1", "198.18.0.2"} {
		t.Run(targetIP, func(t *testing.T) {
			var targetHits atomic.Int32
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Host == "first.example" {
					http.Redirect(w, r, "http://second.example/final", http.StatusFound)
					return
				}
				targetHits.Add(1)
				if r.Header.Get("Referer") != "" {
					t.Error("redirect leaked Referer")
				}
				io.WriteString(w, "ok")
			}))
			defer s.Close()
			deps := isolatedDependencies(t)
			lookups := 0
			deps.lookupNetIP = func(_ context.Context, _, host string) ([]netip.Addr, error) {
				lookups++
				if host == "first.example" {
					return addresses("8.8.4.4"), nil
				}
				if host != "second.example" {
					t.Error("unexpected redirect lookup")
				}
				return addresses(targetIP), nil
			}
			if fakeIP(addresses(targetIP)[0]) {
				deps.dohTransport = func(provider dohProvider) http.RoundTripper { return publicDoH(t, provider) }
			}
			deps.dialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
				if address != "8.8.4.4:80" {
					t.Fatal("redirect connected to unvalidated address")
				}
				return (&net.Dialer{}).DialContext(ctx, network, s.Listener.Addr().String())
			}
			body, err := Fetch(context.Background(), newClient(false, deps), "http://first.example/PRIVATE_CANARY?token=SECRET_CANARY")
			if targetIP == "127.0.0.1" {
				assertKind(t, err, KindPrivateAddress)
				if targetHits.Load() != 0 {
					t.Fatal("private redirect was contacted")
				}
			} else if err != nil || body != "ok" || targetHits.Load() != 1 {
				t.Fatalf("safe redirect failed: %v", err)
			}
			if lookups != 2 {
				t.Fatal("redirect did not re-resolve target")
			}
			assertSecretFree(t, err, "CANARY", "first.example", "second.example")
		})
	}
	var hits atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Header.Get("Referer") != "" {
			t.Error("redirect chain leaked Referer")
		}
		http.Redirect(w, r, "/next", http.StatusFound)
	}))
	defer s.Close()
	_, err := Fetch(context.Background(), NewClient(true), s.URL+"/PRIVATE_CANARY?token=SECRET_CANARY")
	assertKind(t, err, KindRedirect)
	if hits.Load() != 4 {
		t.Fatalf("redirect bound changed: requests=%d", hits.Load())
	}
	assertSecretFree(t, err, "CANARY")
}

func TestRedirectDowngradeAndMalformedLocationAreCategorized(t *testing.T) {
	for _, location := range []string{"http://source.example/PRIVATE_CANARY?token=SECRET_CANARY", "https://user:SECRET_CANARY@source.example/", "https://source.example/%SECRET_CANARY", "file:///PRIVATE_CANARY"} {
		t.Run(strconv.Itoa(len(location)), func(t *testing.T) {
			s, roots := localTLSServer(t, []string{"source.example"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", location)
				w.WriteHeader(http.StatusFound)
			}))
			deps := isolatedDependencies(t)
			deps.lookupNetIP = func(context.Context, string, string) ([]netip.Addr, error) { return addresses("8.8.4.4"), nil }
			dials := 0
			deps.dialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
				dials++
				return (&net.Dialer{}).DialContext(ctx, network, s.Listener.Addr().String())
			}
			client := newClient(false, deps)
			baseTransport(client).TLSClientConfig.RootCAs = roots
			_, err := Fetch(context.Background(), client, "https://source.example/PRIVATE_CANARY?token=SECRET_CANARY")
			assertKind(t, err, KindRedirect)
			assertSecretFree(t, err, "CANARY", "source.example")
			if dials != 1 {
				t.Fatal("unsafe redirect triggered another connection")
			}
		})
	}
}

func TestSameHostnameRedirectRebindingIsBlocked(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/final", http.StatusFound) }))
	defer s.Close()
	deps := isolatedDependencies(t)
	lookups, dials := 0, 0
	deps.lookupNetIP = func(context.Context, string, string) ([]netip.Addr, error) {
		lookups++
		if lookups == 1 {
			return addresses("8.8.4.4"), nil
		}
		return addresses("127.0.0.1"), nil
	}
	deps.dialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		dials++
		return (&net.Dialer{}).DialContext(ctx, network, s.Listener.Addr().String())
	}
	_, err := Fetch(context.Background(), newClient(false, deps), "http://source.example/start")
	assertKind(t, err, KindPrivateAddress)
	if lookups != 2 || dials != 1 {
		t.Fatal("same-host redirect reused stale DNS/connection")
	}
}

func TestFetchRequestDeadlineRemainsInspectable(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	_, err := Fetch(ctx, &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("expired request contacted transport")
		return nil, nil
	})}, "https://source.example/PRIVATE_CANARY")
	assertKind(t, err, KindTimeout)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("deadline was not preserved")
	}
}
