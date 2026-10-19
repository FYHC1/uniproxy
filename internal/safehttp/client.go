package safehttp

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"time"
)

const (
	lookupTimeout   = 5 * time.Second
	connectTimeout  = 10 * time.Second
	tlsTimeout      = 10 * time.Second
	requestTimeout  = 25 * time.Second
	maxDNSAddresses = 64
)

// These seams are package-private: callers cannot supply a resolver URL, proxy,
// insecure TLS policy, or unchecked resolved IPs through the production API.
type clientDependencies struct {
	lookupNetIP  func(context.Context, string, string) ([]netip.Addr, error)
	dialContext  func(context.Context, string, string) (net.Conn, error)
	dohTransport func(dohProvider) http.RoundTripper
}

type targetDialer struct {
	allowPrivate bool
	deps         clientDependencies
}

// NewClient uses direct TCP connections, not system/environment HTTP proxies.
// The explicit allowPrivate mode retains its previous semantics (including
// local Fake-IP routing); automatic public DoH is only used in the default mode.
func NewClient(allowPrivate bool) *http.Client {
	return newClient(allowPrivate, clientDependencies{})
}

func newClient(allowPrivate bool, deps clientDependencies) *http.Client {
	if deps.lookupNetIP == nil {
		deps.lookupNetIP = net.DefaultResolver.LookupNetIP
	}
	if deps.dialContext == nil {
		deps.dialContext = (&net.Dialer{}).DialContext
	}
	d := &targetDialer{allowPrivate: allowPrivate, deps: deps}
	transport := &http.Transport{
		Proxy:                  nil,
		TLSClientConfig:        &tls.Config{MinVersion: tls.VersionTLS12, NextProtos: []string{"http/1.1"}},
		TLSHandshakeTimeout:    tlsTimeout,
		ResponseHeaderTimeout:  15 * time.Second,
		MaxResponseHeaderBytes: 65536,
		DisableKeepAlives:      true,
	}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		ctx, cancel := linkedDialContext(ctx)
		defer cancel()
		return d.dial(ctx, network, address)
	}
	transport.DialTLSContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		ctx, cancel := linkedDialContext(ctx)
		defer cancel()
		conn, err := d.dial(ctx, network, address)
		if err != nil {
			return nil, err
		}
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			conn.Close()
			return nil, failure(KindURL)
		}
		config := transport.TLSClientConfig.Clone()
		config.ServerName = host // verify the URL hostname, never the numeric dial IP
		return handshake(ctx, conn, config)
	}
	return &http.Client{
		Transport: &guardedTransport{base: transport},
		Timeout:   requestTimeout,
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			r.Header.Del("Referer")
			// Preserve the existing bound: at most four requests in one chain.
			if len(via) >= 4 {
				return failure(KindRedirect)
			}
			if _, err := ValidateURL(r.URL.String()); err != nil {
				return failure(KindRedirect)
			}
			if len(via) > 0 && via[len(via)-1].URL.Scheme == "https" && r.URL.Scheme != "https" {
				return failure(KindRedirect)
			}
			return nil
		},
	}
}

func (d *targetDialer) dial(ctx context.Context, network, address string) (net.Conn, error) {
	if ctx.Err() != nil {
		return nil, contextFailure(ctx.Err())
	}
	host, port, err := net.SplitHostPort(address)
	number, portErr := strconv.Atoi(port)
	if err != nil || host == "" || portErr != nil || number < 1 || number > 65535 {
		return nil, failure(KindURL)
	}
	ips, err := d.resolve(ctx, host)
	if err != nil {
		return nil, err
	}
	return d.dialAddresses(ctx, network, port, ips)
}

func (d *targetDialer) resolve(ctx context.Context, host string) ([]netip.Addr, error) {
	// IP literals are never resolved through DNS or eligible for the fallback.
	if ip, err := netip.ParseAddr(host); err == nil {
		if !d.allowPrivate && !PublicIP(ip) {
			return nil, failure(KindPrivateAddress)
		}
		return []netip.Addr{ip.Unmap()}, nil
	}
	lookupCtx, cancel := context.WithTimeout(ctx, lookupTimeout)
	ips, err := d.deps.lookupNetIP(lookupCtx, "ip", host)
	cancel()
	if ctx.Err() != nil {
		return nil, contextFailure(ctx.Err())
	}
	if err != nil {
		return nil, sanitizeError(ctx, err, KindDNS)
	}
	if len(ips) == 0 || len(ips) > maxDNSAddresses {
		return nil, failure(KindDNS)
	}
	allFake, allPublic := true, true
	for _, ip := range ips {
		if !ip.IsValid() {
			return nil, failure(KindDNS)
		}
		allFake = allFake && fakeIP(ip)
		allPublic = allPublic && PublicIP(ip)
	}
	if !d.allowPrivate {
		if allFake {
			return d.resolveFakeIP(ctx, host)
		}
		// Fail closed for every mixed set, including Fake-IP + public. Do not
		// drop the private answers or use DoH to bypass an ordinary private IP.
		if !allPublic {
			return nil, failure(KindPrivateAddress)
		}
	}
	return uniqueAddresses(ips), nil
}

func fakeIP(ip netip.Addr) bool {
	return ip.IsValid() && ip.Zone() == "" && netip.MustParsePrefix("198.18.0.0/15").Contains(ip.Unmap())
}

func uniqueAddresses(ips []netip.Addr) []netip.Addr {
	seen := make(map[netip.Addr]bool, len(ips))
	result := make([]netip.Addr, 0, len(ips))
	for _, ip := range ips {
		ip = ip.Unmap()
		if !seen[ip] {
			seen[ip] = true
			result = append(result, ip)
		}
	}
	return result
}

func (d *targetDialer) dialAddresses(ctx context.Context, network, port string, ips []netip.Addr) (net.Conn, error) {
	// One shared connect budget, not ten seconds per address. A dead first IP
	// must leave time for the other already validated (and numeric-pinned) IPs.
	dialCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	deadline, _ := dialCtx.Deadline()
	last := failure(KindConnect)
	for i, ip := range ips {
		if ctx.Err() != nil {
			return nil, contextFailure(ctx.Err())
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, &FetchError{Kind: KindConnectTimeout, cause: context.DeadlineExceeded}
		}
		attemptCtx, stop := context.WithTimeout(dialCtx, remaining/time.Duration(len(ips)-i))
		conn, err := d.deps.dialContext(attemptCtx, network, net.JoinHostPort(ip.String(), port))
		stop()
		if err == nil && conn != nil {
			if ctx.Err() != nil {
				conn.Close()
				return nil, contextFailure(ctx.Err())
			}
			return conn, nil
		}
		if conn != nil {
			conn.Close()
		}
		last = sanitizeError(ctx, err, KindConnect)
	}
	return nil, last
}

func handshake(ctx context.Context, conn net.Conn, config *tls.Config) (net.Conn, error) {
	tlsConn := tls.Client(conn, config)
	tlsCtx, cancel := context.WithTimeout(ctx, tlsTimeout)
	defer cancel()
	if err := tlsConn.HandshakeContext(tlsCtx); err != nil {
		conn.Close()
		return nil, sanitizeError(ctx, err, KindTLS)
	}
	return tlsConn, nil
}

type requestContextKey struct{}

// net/http may detach an in-progress dial from its initiating request so another
// request can reuse the connection. We disable reuse, and explicitly retain the
// initiating cancellation/deadline so DNS/DoH/dial do not outlive a canceled fetch.
func linkedDialContext(ctx context.Context) (context.Context, func()) {
	requestCtx, ok := ctx.Value(requestContextKey{}).(context.Context)
	if !ok {
		return ctx, func() {}
	}
	linked, cancel := context.WithCancel(requestCtx)
	stop := context.AfterFunc(ctx, cancel)
	if ctx.Err() != nil {
		cancel()
	}
	return linked, func() { stop(); cancel() }
}

type guardedTransport struct{ base http.RoundTripper }

func (t *guardedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if _, err := ValidateURL(req.URL.String()); err != nil {
		return nil, err
	}
	ctx := req.Context()
	r := req.Clone(context.WithValue(ctx, requestContextKey{}, ctx))
	r.Header.Del("Referer")
	res, err := t.base.RoundTrip(r)
	if err != nil {
		return nil, sanitizeError(ctx, err, KindDownload)
	}
	// Validate Location before net/http parses it; its raw parse errors can
	// otherwise contain a credential-bearing redirect URL and lose the category.
	if isRedirect(res.StatusCode) && res.Header.Get("Location") != "" {
		next, parseErr := r.URL.Parse(res.Header.Get("Location"))
		if parseErr == nil {
			_, parseErr = ValidateURL(next.String())
		}
		if parseErr != nil || (r.URL.Scheme == "https" && next.Scheme != "https") {
			res.Body.Close()
			return nil, failure(KindRedirect)
		}
	}
	return res, nil
}

func (t *guardedTransport) CloseIdleConnections() {
	if closer, ok := t.base.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

func isRedirect(code int) bool {
	return code == http.StatusMovedPermanently || code == http.StatusFound ||
		code == http.StatusSeeOther || code == http.StatusTemporaryRedirect || code == http.StatusPermanentRedirect
}
