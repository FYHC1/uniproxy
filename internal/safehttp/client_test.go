package safehttp

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestResolutionPolicyOnlyAllFakeHostnamesFallback(t *testing.T) {
	tests := []struct {
		name         string
		host         string
		ips          []netip.Addr
		lookupErr    error
		allowPrivate bool
		want         ErrorKind
		lookup       bool
		fallback     bool
	}{
		{name: "all fake", host: "source.example", ips: addresses("198.18.0.230", "198.19.255.254"), lookup: true, fallback: true},
		{name: "mapped fake", host: "source.example", ips: addresses("::ffff:198.18.1.2"), lookup: true, fallback: true},
		{name: "public", host: "source.example", ips: addresses("8.8.4.4", "2606:4700:4700::1111"), lookup: true},
		{name: "fake and public", host: "source.example", ips: addresses("198.18.0.230", "8.8.4.4"), lookup: true, want: KindPrivateAddress},
		{name: "fake and private", host: "source.example", ips: addresses("198.18.0.230", "10.0.0.1"), lookup: true, want: KindPrivateAddress},
		{name: "fake and loopback", host: "source.example", ips: addresses("198.18.0.230", "::1"), lookup: true, want: KindPrivateAddress},
		{name: "public and private", host: "source.example", ips: addresses("8.8.4.4", "192.168.1.1"), lookup: true, want: KindPrivateAddress},
		{name: "private", host: "source.example", ips: addresses("10.0.0.1"), lookup: true, want: KindPrivateAddress},
		{name: "reserved", host: "source.example", ips: addresses("192.0.2.1"), lookup: true, want: KindPrivateAddress},
		{name: "fake literal", host: "198.18.0.230", want: KindPrivateAddress},
		{name: "mapped fake literal", host: "::ffff:198.18.0.230", want: KindPrivateAddress},
		{name: "private literal", host: "10.0.0.1", want: KindPrivateAddress},
		{name: "public literal", host: "8.8.4.4"},
		{name: "zoned IPv6 literal", host: "2606:4700:4700::1111%SECRET_ZONE", want: KindPrivateAddress},
		{name: "lookup failure", host: "source.example", lookupErr: &net.DNSError{Err: "SECRET_DNS_CANARY", Name: "PRIVATE_CANARY"}, lookup: true, want: KindDNS},
		{name: "lookup timeout", host: "source.example", lookupErr: timeoutError{}, lookup: true, want: KindDNS},
		{name: "empty", host: "source.example", lookup: true, want: KindDNS},
		{name: "invalid answer", host: "source.example", ips: []netip.Addr{{}}, lookup: true, want: KindDNS},
		{name: "fake invalid hostname", host: "bad/name", ips: addresses("198.18.0.2"), lookup: true, want: KindFakeIPFallback},
		{name: "explicit private mode", host: "source.example", ips: addresses("198.18.0.230", "10.0.0.1"), lookup: true, allowPrivate: true},
		{name: "explicit fake literal", host: "198.18.0.230", allowPrivate: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := isolatedDependencies(t)
			lookups, fallbacks := 0, 0
			if tt.lookup {
				deps.lookupNetIP = func(ctx context.Context, network, host string) ([]netip.Addr, error) {
					lookups++
					if network != "ip" || host != tt.host {
						t.Error("unexpected lookup arguments")
					}
					deadline, ok := ctx.Deadline()
					if !ok || time.Until(deadline) > lookupTimeout {
						t.Error("system lookup did not receive bounded deadline")
					}
					return tt.ips, tt.lookupErr
				}
			}
			if tt.fallback {
				deps.dohTransport = func(provider dohProvider) http.RoundTripper {
					fallbacks++
					return publicDoH(t, provider)
				}
			}
			d := &targetDialer{allowPrivate: tt.allowPrivate, deps: deps}
			ips, err := d.resolve(context.Background(), tt.host)
			assertKind(t, err, tt.want)
			assertSecretFree(t, err, "SECRET", "PRIVATE_CANARY", "bad/name")
			if (lookups > 0) != tt.lookup || (fallbacks > 0) != tt.fallback {
				t.Fatalf("unexpected resolution route: lookups=%d fallbacks=%d", lookups, fallbacks)
			}
			if err == nil {
				if len(ips) == 0 {
					t.Fatal("successful resolution has no addresses")
				}
				if !tt.allowPrivate {
					for _, ip := range ips {
						if !PublicIP(ip) {
							t.Fatal("successful default resolution contains a nonpublic address")
						}
					}
				}
			}
		})
	}
}

func TestFakeIPFetchPinnedPublicDialAndHostnameOnlyQuery(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "source.example" || r.URL.Path != "/PRIVATE_PATH_CANARY" || r.URL.Query().Get("token") != "SECRET_QUERY_CANARY" {
			t.Error("target request lost its original host/path")
		}
		w.Write([]byte("proxies: []"))
	}))
	defer s.Close()
	deps := isolatedDependencies(t)
	lookups, dohCalls, dials := 0, 0, 0
	deps.lookupNetIP = func(context.Context, string, string) ([]netip.Addr, error) {
		lookups++
		return addresses("198.18.0.230"), nil
	}
	deps.dohTransport = func(provider dohProvider) http.RoundTripper {
		return roundTripFunc(func(req *http.Request) (*http.Response, error) {
			dohCalls++
			query := req.URL.Query()
			if req.Method != "GET" || req.URL.User != nil || req.URL.Host != "cloudflare-dns.com" || req.URL.Path != "/dns-query" || req.URL.Scheme != "https" {
				t.Error("DoH request did not use fixed HTTPS provider")
			}
			if len(query) != 3 || query.Get("name") != "source.example" || query.Get("edns_client_subnet") != "0.0.0.0/0" {
				t.Error("DoH query leaked more than hostname or lost privacy parameters")
			}
			if req.Header.Get("Referer") != "" || req.Header.Get("Cookie") != "" || req.Header.Get("Authorization") != "" || req.Body != nil {
				t.Error("DoH request forwarded sensitive request material")
			}
			if strings.Contains(req.URL.String(), "CANARY") {
				t.Error("DoH URL leaked target token/path")
			}
			kind, _ := strconv.Atoi(query.Get("type"))
			values := []string(nil)
			if kind == 1 {
				values = []string{"8.8.4.4", "8.8.4.4"}
			}
			return response(dohJSON(dohMessage("source.example", kind, values...))), nil
		})
	}
	deps.dialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		dials++
		if address != "8.8.4.4:80" {
			t.Fatal("target dial not pinned to validated public IP")
		}
		return (&net.Dialer{}).DialContext(ctx, network, s.Listener.Addr().String())
	}
	body, err := Fetch(context.Background(), newClient(false, deps), "http://source.example/PRIVATE_PATH_CANARY?token=SECRET_QUERY_CANARY")
	if err != nil || body != "proxies: []" || lookups != 1 || dohCalls != 2 || dials != 1 {
		t.Fatalf("body=%q error=%v lookups=%d DoH=%d dials=%d", body, err, lookups, dohCalls, dials)
	}
}

func TestAllAddressDialHasSharedBudgetAndTriesNext(t *testing.T) {
	deps := isolatedDependencies(t)
	ips := addresses("8.8.4.4", "1.0.0.1", "9.9.9.9")
	calls := 0
	var previousBudget time.Duration
	deps.dialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != net.JoinHostPort(ips[calls].String(), "443") {
			t.Error("numeric dial order changed")
		}
		deadline, ok := ctx.Deadline()
		budget := time.Until(deadline)
		if !ok || budget <= 0 || budget > connectTimeout || (calls == 0 && budget > connectTimeout/3) {
			t.Error("dial attempt did not divide the shared deadline")
		}
		if calls > 0 && budget <= previousBudget {
			t.Error("unused budget was not available to next address")
		}
		previousBudget = budget
		calls++
		if calls == 1 {
			return nil, timeoutError{}
		}
		if calls == 2 {
			return nil, errors.New("SECRET_CONNECTION_CANARY")
		}
		client, peer := net.Pipe()
		peer.Close()
		return client, nil
	}
	d := &targetDialer{deps: deps}
	conn, err := d.dialAddresses(context.Background(), "tcp", "443", ips)
	if err != nil || calls != 3 {
		t.Fatalf("dial did not try all usable addresses: calls=%d error=%v", calls, err)
	}
	conn.Close()
	deps.dialContext = func(context.Context, string, string) (net.Conn, error) { return nil, timeoutError{} }
	d.deps = deps
	_, err = d.dialAddresses(context.Background(), "tcp", "443", ips)
	assertKind(t, err, KindConnectTimeout)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("connect timeout lost errors.Is deadline semantics")
	}
	assertSecretFree(t, err, "SECRET_CONNECTION_CANARY", "SECRET_TIMEOUT_CANARY")
}

func TestFetchCancellationStopsDetachedDNSAndDoH(t *testing.T) {
	for _, stage := range []string{"lookup", "doh", "dial"} {
		t.Run(stage, func(t *testing.T) {
			started, stopped := make(chan struct{}), make(chan struct{})
			deps := isolatedDependencies(t)
			block := func(ctx context.Context) error {
				close(started)
				<-ctx.Done()
				close(stopped)
				return ctx.Err()
			}
			deps.lookupNetIP = func(ctx context.Context, _, _ string) ([]netip.Addr, error) {
				if stage == "lookup" {
					return nil, block(ctx)
				}
				if stage == "doh" {
					return addresses("198.18.0.230"), nil
				}
				return addresses("8.8.4.4"), nil
			}
			if stage == "doh" {
				deps.dohTransport = func(dohProvider) http.RoundTripper {
					return roundTripFunc(func(r *http.Request) (*http.Response, error) { return nil, block(r.Context()) })
				}
			}
			if stage == "dial" {
				deps.dialContext = func(ctx context.Context, _, _ string) (net.Conn, error) { return nil, block(ctx) }
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				_, err := Fetch(ctx, newClient(false, deps), "http://source.example/PRIVATE_CANARY?token=SECRET_CANARY")
				done <- err
			}()
			select {
			case <-started:
			case <-time.After(2 * time.Second):
				t.Fatal("request did not start")
			}
			cancel()
			select {
			case err := <-done:
				assertKind(t, err, KindCanceled)
				if !errors.Is(err, context.Canceled) {
					t.Fatal("request lost context cancellation")
				}
				assertSecretFree(t, err, "PRIVATE_CANARY", "SECRET_CANARY")
			case <-time.After(2 * time.Second):
				t.Fatal("fetch ignored cancellation")
			}
			select {
			case <-stopped:
			case <-time.After(2 * time.Second):
				t.Fatal("transport left detached DNS/DoH/dial running")
			}
		})
	}
}

func TestDirectTransportIgnoresProxyAndPreservesTLSHostname(t *testing.T) {
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"} {
		t.Setenv(key, "http://127.0.0.1:1")
	}
	var seenSNI atomic.Bool
	s, roots := localTLSServer(t, []string{"source.example"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS.ServerName == "source.example" && r.Host == "source.example" {
			seenSNI.Store(true)
		}
		io.WriteString(w, "ok")
	}))
	deps := isolatedDependencies(t)
	deps.lookupNetIP = func(context.Context, string, string) ([]netip.Addr, error) { return addresses("8.8.4.4"), nil }
	deps.dialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != "8.8.4.4:443" {
			t.Fatal("proxy or unpinned connection attempted")
		}
		return (&net.Dialer{}).DialContext(ctx, network, s.Listener.Addr().String())
	}
	client := newClient(false, deps)
	transport := baseTransport(client)
	transport.TLSClientConfig.RootCAs = roots
	if transport.Proxy != nil || transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("unsafe production transport policy")
	}
	body, err := Fetch(context.Background(), client, "https://source.example/PRIVATE_CANARY?token=SECRET_CANARY")
	if err != nil || body != "ok" || !seenSNI.Load() {
		t.Fatalf("verified TLS fetch failed: body=%q error=%v SNI=%v", body, err, seenSNI.Load())
	}
	// Trusted root does not permit a hostname mismatch.
	_, err = Fetch(context.Background(), client, "https://wrong.example/PRIVATE_CANARY?token=SECRET_CANARY")
	assertKind(t, err, KindTLS)
	assertSecretFree(t, err, "wrong.example", "source.example", "PRIVATE_CANARY", "SECRET_CANARY")
	// The default client must not trust the fixture's self-signed certificate.
	_, err = Fetch(context.Background(), newClient(false, deps), "https://source.example/PRIVATE_CANARY?token=SECRET_CANARY")
	assertKind(t, err, KindTLS)
	assertSecretFree(t, err, "source.example", "PRIVATE_CANARY", "SECRET_CANARY")
}
