package safehttp

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDoHResponseValidation(t *testing.T) {
	valid := func() dohResponse { return dohMessage("source.example", 1, "8.8.4.4") }
	tests := []struct {
		name   string
		change func(*dohResponse)
		want   ErrorKind
		count  int
	}{
		{name: "valid", count: 1},
		{name: "nodata", change: func(m *dohResponse) { m.Answer = nil }},
		{name: "status required", change: func(m *dohResponse) { m.Status = nil }, want: KindDNS},
		{name: "NXDOMAIN", change: func(m *dohResponse) { *m.Status = 3 }, want: KindDNS},
		{name: "truncated", change: func(m *dohResponse) { m.TC = true }, want: KindDNS},
		{name: "wrong question", change: func(m *dohResponse) { m.Question[0].Name = "OTHER_SECRET.example" }, want: KindDNS},
		{name: "wrong type", change: func(m *dohResponse) { m.Question[0].Type = 28 }, want: KindDNS},
		{name: "missing question", change: func(m *dohResponse) { m.Question = nil }, want: KindDNS},
		{name: "unrelated answer", change: func(m *dohResponse) { m.Answer[0].Name = "OTHER_SECRET.example" }, want: KindDNS},
		{name: "invalid address", change: func(m *dohResponse) { m.Answer[0].Data = "https://SECRET_CANARY/path" }, want: KindDNS},
		{name: "wrong family", change: func(m *dohResponse) { m.Answer[0].Data = "2606:4700:4700::1111" }, want: KindDNS},
		{name: "private", change: func(m *dohResponse) { m.Answer[0].Data = "10.0.0.1" }, want: KindPrivateAddress},
		{name: "fake", change: func(m *dohResponse) { m.Answer[0].Data = "198.18.0.2" }, want: KindPrivateAddress},
		{name: "metadata", change: func(m *dohResponse) { m.Answer[0].Data = "169.254.169.254" }, want: KindPrivateAddress},
		{name: "mixed public private", change: func(m *dohResponse) {
			m.Answer = append(m.Answer, dohRecord{Name: "source.example", Type: 1, Data: "127.0.0.1"})
		}, want: KindPrivateAddress},
		{name: "additional private", change: func(m *dohResponse) { m.Additional = []dohRecord{{Name: "PRIVATE_CANARY", Type: 1, Data: "10.0.0.1"}} }, want: KindPrivateAddress},
		{name: "authority private", change: func(m *dohResponse) {
			m.Authority = []dohRecord{{Name: "PRIVATE_CANARY", Type: 28, Data: "::ffff:127.0.0.1"}}
		}, want: KindPrivateAddress},
		{name: "zoned global", change: func(m *dohResponse) {
			m.Additional = []dohRecord{{Name: "PRIVATE_CANARY", Type: 28, Data: "2606:4700:4700::1111%zone"}}
		}, want: KindPrivateAddress},
		{name: "oversized record set", change: func(m *dohResponse) { m.Authority = make([]dohRecord, maxDoHRecords) }, want: KindDNS},
		{name: "CNAME public", change: func(m *dohResponse) {
			m.Answer[0].Name = "edge.example."
			m.Answer = append(m.Answer, dohRecord{Name: "source.example.", Type: 5, Data: "edge.example."})
		}, count: 1},
		{name: "CNAME cycle", change: func(m *dohResponse) {
			m.Answer = []dohRecord{{Name: "source.example", Type: 5, Data: "source.example"}}
		}, want: KindDNS},
		{name: "CNAME unrelated", change: func(m *dohResponse) {
			m.Answer = append(m.Answer, dohRecord{Name: "OTHER_SECRET.example", Type: 5, Data: "edge.example"})
		}, want: KindDNS},
		{name: "CNAME ambiguous", change: func(m *dohResponse) {
			m.Answer = []dohRecord{{Name: "source.example", Type: 5, Data: "first.example"}, {Name: "source.example", Type: 5, Data: "second.example"}}
		}, want: KindDNS},
		{name: "CNAME URL", change: func(m *dohResponse) {
			m.Answer = []dohRecord{{Name: "source.example", Type: 5, Data: "https://SECRET_CANARY/path"}}
		}, want: KindDNS},
		{name: "CNAME address at alias", change: func(m *dohResponse) {
			m.Answer = append(m.Answer, dohRecord{Name: "source.example", Type: 5, Data: "edge.example"})
		}, want: KindDNS},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := valid()
			if tt.change != nil {
				tt.change(&msg)
			}
			ips, err := parseDoH([]byte(dohJSON(msg)), "source.example", 1)
			assertKind(t, err, tt.want)
			assertSecretFree(t, err, "SECRET", "PRIVATE_CANARY", "source.example", "edge.example")
			if err == nil && len(ips) != tt.count {
				t.Fatalf("addresses=%d want %d", len(ips), tt.count)
			}
		})
	}
	for _, body := range []string{"{}", "null", "[]", "<html>SECRET_CANARY</html>", dohJSON(valid()) + "{}", "{\"Status\":0,\"Status\":\"SECRET\"}"} {
		_, err := parseDoH([]byte(body), "source.example", 1)
		assertKind(t, err, KindDNS)
		assertSecretFree(t, err, "SECRET_CANARY", "SECRET")
	}
	_, err := parseDoH([]byte(dohJSON(dohMessage("source.example", 28, "2606:4700:4700::1111"))), "source.example", 28)
	if err != nil {
		t.Fatal(err)
	}
	_, err = parseDoH([]byte(dohJSON(dohMessage("source.example", 28, "::ffff:10.0.0.1"))), "source.example", 28)
	assertKind(t, err, KindPrivateAddress)
}

func TestDoHFallbackProviderOrderAndBounds(t *testing.T) {
	deps := isolatedDependencies(t)
	var providers []string
	var queries []string
	deps.dohTransport = func(provider dohProvider) http.RoundTripper {
		providers = append(providers, provider.host)
		return roundTripFunc(func(req *http.Request) (*http.Response, error) {
			deadline, ok := req.Context().Deadline()
			if !ok || time.Until(deadline) > dohProviderTimeout {
				t.Error("provider request is not bounded")
			}
			queries = append(queries, req.URL.Query().Get("name"))
			if provider.host == "cloudflare-dns.com" {
				res := response("SECRET_RESPONSE_CANARY")
				res.StatusCode = http.StatusServiceUnavailable
				return res, nil
			}
			kind, _ := strconv.Atoi(req.URL.Query().Get("type"))
			values := []string(nil)
			if kind == 1 {
				values = []string{"8.8.4.4"}
			}
			return response(dohJSON(dohMessage("source.example", kind, values...))), nil
		})
	}
	d := &targetDialer{deps: deps}
	ips, err := d.resolveFakeIP(context.Background(), "SOURCE.example.")
	if err != nil || len(ips) != 1 || len(providers) != 2 || providers[0] != "cloudflare-dns.com" || providers[1] != "dns.google" || len(queries) != 3 {
		t.Fatalf("provider fallback failed: addresses=%v providers=%v error=%v", ips, providers, err)
	}
	for _, host := range queries {
		if host != "source.example" {
			t.Error("query did not use normalized original hostname")
		}
	}
}

func TestDoHPrivateAnswerStopsFallbackAndTargetDial(t *testing.T) {
	for _, privateFamily := range []int{1, 28} {
		t.Run(strconv.Itoa(privateFamily), func(t *testing.T) {
			deps := isolatedDependencies(t)
			deps.lookupNetIP = func(context.Context, string, string) ([]netip.Addr, error) { return addresses("198.18.0.230"), nil }
			providers := 0
			deps.dohTransport = func(provider dohProvider) http.RoundTripper {
				providers++
				return roundTripFunc(func(req *http.Request) (*http.Response, error) {
					kind, _ := strconv.Atoi(req.URL.Query().Get("type"))
					value := "8.8.4.4"
					if kind == 28 {
						value = "2606:4700:4700::1111"
					}
					if kind == privateFamily {
						value = "127.0.0.1"
						if kind == 28 {
							value = "::ffff:127.0.0.1"
						}
					}
					return response(dohJSON(dohMessage("source.example", kind, value))), nil
				})
			}
			_, err := Fetch(context.Background(), newClient(false, deps), "http://source.example/PRIVATE_CANARY?token=SECRET_CANARY")
			assertKind(t, err, KindFakeIPFallback)
			assertKind(t, errors.Unwrap(err), KindPrivateAddress)
			if providers != 1 {
				t.Fatal("private result tried a second provider")
			}
			assertSecretFree(t, err, "PRIVATE_CANARY", "SECRET_CANARY", "source.example")
		})
	}
}

func TestDoHRejectsRedirectMalformedAndOversizedResponses(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		code     int
		length   int64
		location string
		want     ErrorKind
	}{
		{name: "malformed JSON", body: "SECRET_RESPONSE_CANARY", want: KindDNS},
		{name: "too much body", body: strings.Repeat("x", maxDoHBody+1), want: KindBodyTooLarge},
		{name: "too much header", body: "{}", length: maxDoHBody + 1, want: KindBodyTooLarge},
		{name: "http failure", body: "SECRET_RESPONSE_CANARY", code: 503, want: KindHTTPStatus},
		{name: "redirect", code: 302, location: "https://OTHER_SECRET.example/PRIVATE_CANARY?token=SECRET_CANARY", want: KindRedirect},
		{name: "malformed redirect", code: 302, location: "https://SECRET_CANARY/%invalid", want: KindRedirect},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := isolatedDependencies(t)
			calls := 0
			deps.dohTransport = func(dohProvider) http.RoundTripper {
				return roundTripFunc(func(req *http.Request) (*http.Response, error) {
					calls++
					res := response(tt.body)
					if tt.code != 0 {
						res.StatusCode = tt.code
					}
					if tt.length != 0 {
						res.ContentLength = tt.length
					}
					if tt.location != "" {
						res.Header.Set("Location", tt.location)
					}
					return res, nil
				})
			}
			d := &targetDialer{deps: deps}
			_, err := d.resolveFakeIP(context.Background(), "source.example")
			assertKind(t, err, KindFakeIPFallback)
			assertKind(t, errors.Unwrap(err), tt.want)
			assertSecretFree(t, err, "SECRET", "PRIVATE_CANARY", "source.example")
			if calls != 2 {
				t.Fatalf("unexpected request count %d", calls)
			}
		})
	}
}

func TestDoHBootstrapUsesFixedNumericConnectAndVerifiedProviderTLS(t *testing.T) {
	for _, provider := range dohProviders() {
		t.Run(provider.host, func(t *testing.T) {
			for _, key := range []string{"HTTPS_PROXY", "ALL_PROXY"} {
				t.Setenv(key, "http://127.0.0.1:1")
			}
			var httpCalls atomic.Int32
			s, roots := localTLSServer(t, []string{provider.host}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				httpCalls.Add(1)
				if r.TLS.ServerName != provider.host || r.Host != provider.host || r.URL.Path != provider.path {
					t.Error("bootstrap lost provider TLS/Host identity")
				}
				if strings.Contains(r.URL.String(), "CANARY") {
					t.Error("bootstrap query leaked private URL")
				}
				io.WriteString(w, dohJSON(dohMessage("source.example", 1, "8.8.4.4")))
			}))
			dials := atomic.Int32{}
			dial := func(ctx context.Context, network, address string) (net.Conn, error) {
				dials.Add(1)
				if address != provider.address || network != "tcp" {
					t.Error("DoH did not pin numeric provider connect")
				}
				return (&net.Dialer{}).DialContext(ctx, network, s.Listener.Addr().String())
			}
			transport := newDoHTransport(provider, dial)
			defer transport.CloseIdleConnections()
			if transport.Proxy != nil || transport.TLSClientConfig.InsecureSkipVerify || transport.TLSClientConfig.ServerName != provider.host || transport.TLSClientConfig.MinVersion < tls.VersionTLS12 {
				t.Fatal("unsafe DoH TLS/proxy policy")
			}
			transport.TLSClientConfig.RootCAs = roots
			client := &http.Client{Transport: &guardedTransport{base: transport}}
			ips, err := queryDoH(context.Background(), client, provider, "source.example", 1)
			if err != nil || len(ips) != 1 || dials.Load() != 1 || httpCalls.Load() != 1 {
				t.Fatalf("verified provider request failed: %v", err)
			}
			// A different provider URL must not inherit this provider's dial pin.
			wrong := provider
			wrong.host = "OTHER_SECRET.example"
			_, err = queryDoH(context.Background(), client, wrong, "source.example", 1)
			if err == nil || dials.Load() != 1 {
				t.Fatal("bootstrap permitted arbitrary provider destination")
			}
			// Without an explicit test CA, verification fails before any HTTP bytes.
			untrusted := newDoHTransport(provider, dial)
			defer untrusted.CloseIdleConnections()
			_, err = queryDoH(context.Background(), &http.Client{Transport: &guardedTransport{base: untrusted}}, provider, "source.example", 1)
			assertKind(t, err, KindTLS)
			assertSecretFree(t, err, provider.host, "source.example")
			if httpCalls.Load() != 1 {
				t.Fatal("untrusted provider received HTTP request")
			}
		})
	}
}

func TestDoHTimeoutAndCancellationAreSanitized(t *testing.T) {
	deps := isolatedDependencies(t)
	deps.dohTransport = func(dohProvider) http.RoundTripper {
		return roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, timeoutError{} })
	}
	d := &targetDialer{deps: deps}
	_, err := d.resolveFakeIP(context.Background(), "source.example")
	assertKind(t, err, KindFakeIPFallback)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("fallback lost timeout sentinel")
	}
	assertSecretFree(t, err, "SECRET_TIMEOUT_CANARY", "source.example")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = d.resolveFakeIP(ctx, "source.example")
	assertKind(t, err, KindCanceled)
	if !errors.Is(err, context.Canceled) {
		t.Fatal("fallback lost cancellation sentinel")
	}
}
