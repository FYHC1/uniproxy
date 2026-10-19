package safehttp

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	dohTimeout         = 6 * time.Second
	dohProviderTimeout = 3 * time.Second
	maxDoHBody         = 64 * 1024
	maxDoHRecords      = 64
)

type dohProvider struct {
	host    string
	address string
	path    string
}

// Numeric bootstrap addresses never go through the system resolver. HTTPS Host,
// SNI, and certificate verification remain bound to the corresponding provider.
func dohProviders() []dohProvider {
	return []dohProvider{
		{host: "cloudflare-dns.com", address: "1.1.1.1:443", path: "/dns-query"},
		{host: "dns.google", address: "8.8.8.8:443", path: "/resolve"},
	}
}

func newDoHTransport(provider dohProvider, dial func(context.Context, string, string) (net.Conn, error)) *http.Transport {
	transport := &http.Transport{
		Proxy:                  nil,
		TLSClientConfig:        &tls.Config{MinVersion: tls.VersionTLS12, ServerName: provider.host, NextProtos: []string{"http/1.1"}},
		TLSHandshakeTimeout:    dohProviderTimeout,
		ResponseHeaderTimeout:  dohProviderTimeout,
		MaxResponseHeaderBytes: 16384,
		DisableKeepAlives:      true,
	}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != net.JoinHostPort(provider.host, "443") {
			return nil, failure(KindDNS)
		}
		ctx, cancel := linkedDialContext(ctx)
		defer cancel()
		conn, err := dial(ctx, "tcp", provider.address)
		if err != nil {
			return nil, sanitizeError(ctx, err, KindConnect)
		}
		return conn, nil
	}
	transport.DialTLSContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != net.JoinHostPort(provider.host, "443") {
			return nil, failure(KindDNS)
		}
		ctx, cancel := linkedDialContext(ctx)
		defer cancel()
		conn, err := dial(ctx, "tcp", provider.address)
		if err != nil {
			return nil, sanitizeError(ctx, err, KindConnect)
		}
		return handshake(ctx, conn, transport.TLSClientConfig.Clone())
	}
	return transport
}

func (d *targetDialer) resolveFakeIP(ctx context.Context, host string) ([]netip.Addr, error) {
	host, valid := dnsName(host)
	if !valid {
		return nil, &FetchError{Kind: KindFakeIPFallback, cause: failure(KindDNS)}
	}
	fallbackCtx, cancel := context.WithTimeout(ctx, dohTimeout)
	defer cancel()
	var last error = failure(KindDNS)
	for _, provider := range dohProviders() {
		if ctx.Err() != nil {
			return nil, contextFailure(ctx.Err())
		}
		if fallbackCtx.Err() != nil {
			last = contextFailure(fallbackCtx.Err())
			break
		}
		providerCtx, stop := context.WithTimeout(fallbackCtx, dohProviderTimeout)
		ips, err := d.lookupDoH(providerCtx, provider, host)
		stop()
		if ctx.Err() != nil {
			return nil, contextFailure(ctx.Err())
		}
		if err == nil {
			return ips, nil
		}
		last = err
		// A private/Fake-IP answer is a policy rejection, not a reason to shop
		// another provider for a more permissive response.
		if ErrorKindOf(err) == KindPrivateAddress || errors.Is(err, context.Canceled) {
			break
		}
	}
	return nil, &FetchError{Kind: KindFakeIPFallback, cause: last}
}

func (d *targetDialer) lookupDoH(ctx context.Context, provider dohProvider, host string) ([]netip.Addr, error) {
	var transport http.RoundTripper
	if d.deps.dohTransport != nil {
		transport = d.deps.dohTransport(provider)
	} else {
		transport = newDoHTransport(provider, d.deps.dialContext)
	}
	guard := &guardedTransport{base: transport}
	defer guard.CloseIdleConnections()
	client := &http.Client{
		Transport: guard,
		Timeout:   dohProviderTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return failure(KindRedirect)
		},
	}
	var result []netip.Addr
	// Query both families using only the original hostname. A private answer in
	// either family rejects the whole result; never retain just the public half.
	for _, kind := range []int{1, 28} {
		ips, err := queryDoH(ctx, client, provider, host, kind)
		if err != nil {
			return nil, err
		}
		result = append(result, ips...)
	}
	if len(result) == 0 || len(result) > maxDNSAddresses {
		return nil, failure(KindDNS)
	}
	for _, ip := range result {
		if !PublicIP(ip) {
			return nil, failure(KindPrivateAddress)
		}
	}
	return uniqueAddresses(result), nil
}

func queryDoH(ctx context.Context, client *http.Client, provider dohProvider, host string, kind int) ([]netip.Addr, error) {
	u := url.URL{Scheme: "https", Host: provider.host, Path: provider.path}
	// Suppress client-subnet forwarding. No subscription path, query, port,
	// credentials, cookies, Referer, or caller-provided headers are sent here.
	u.RawQuery = url.Values{"name": {host}, "type": {strconv.Itoa(kind)}, "edns_client_subnet": {"0.0.0.0/0"}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, failure(KindDNS)
	}
	req.Header.Set("Accept", "application/dns-json")
	res, err := client.Do(req)
	if err != nil {
		return nil, sanitizeError(ctx, err, KindDownload)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, &FetchError{Kind: KindHTTPStatus, StatusCode: res.StatusCode}
	}
	if res.ContentLength > maxDoHBody {
		return nil, failure(KindBodyTooLarge)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, maxDoHBody+1))
	if err != nil {
		return nil, sanitizeError(ctx, err, KindBodyRead)
	}
	if len(body) > maxDoHBody {
		return nil, failure(KindBodyTooLarge)
	}
	return parseDoH(body, host, kind)
}

type dohQuestion struct {
	Name string
	Type int
}

type dohRecord struct {
	Name string
	Type int
	Data string
}

type dohResponse struct {
	Status     *int
	TC         bool
	Question   []dohQuestion
	Answer     []dohRecord
	Authority  []dohRecord
	Additional []dohRecord
}

func parseDoH(body []byte, host string, kind int) ([]netip.Addr, error) {
	var msg dohResponse
	if err := json.Unmarshal(body, &msg); err != nil || msg.Status == nil || *msg.Status != 0 || msg.TC || len(msg.Question) != 1 {
		return nil, failure(KindDNS)
	}
	question, valid := dnsName(msg.Question[0].Name)
	if !valid || question != host || msg.Question[0].Type != kind {
		return nil, failure(KindDNS)
	}
	if len(msg.Answer)+len(msg.Authority)+len(msg.Additional) > maxDoHRecords {
		return nil, failure(KindDNS)
	}
	// Reject ALL address records, including unrelated/additional ones, before
	// considering a CNAME chain. DNS text is never dialed or returned in errors.
	for _, section := range [][]dohRecord{msg.Answer, msg.Authority, msg.Additional} {
		for _, record := range section {
			if record.Type != 1 && record.Type != 28 {
				continue
			}
			ip, err := netip.ParseAddr(record.Data)
			if err != nil || (record.Type == 1 && !ip.Is4()) || (record.Type == 28 && !ip.Is6()) {
				return nil, failure(KindDNS)
			}
			if !PublicIP(ip) {
				return nil, failure(KindPrivateAddress)
			}
		}
	}
	aliases := make(map[string]string)
	for _, record := range msg.Answer {
		name, valid := dnsName(record.Name)
		if !valid {
			return nil, failure(KindDNS)
		}
		switch record.Type {
		case 5:
			target, ok := dnsName(record.Data)
			if !ok || (aliases[name] != "" && aliases[name] != target) {
				return nil, failure(KindDNS)
			}
			aliases[name] = target
		case kind:
		default:
			return nil, failure(KindDNS)
		}
	}
	terminal := host
	visited := make(map[string]bool)
	for {
		if visited[terminal] || len(visited) > 16 {
			return nil, failure(KindDNS)
		}
		visited[terminal] = true
		if aliases[terminal] == "" {
			break
		}
		terminal = aliases[terminal]
	}
	for name := range aliases {
		if !visited[name] {
			return nil, failure(KindDNS)
		}
	}
	var result []netip.Addr
	for _, record := range msg.Answer {
		if record.Type != kind {
			continue
		}
		name, _ := dnsName(record.Name)
		if name != terminal {
			return nil, failure(KindDNS)
		}
		ip, _ := netip.ParseAddr(record.Data) // already validated above
		result = append(result, ip.Unmap())
	}
	return result, nil
}

func dnsName(raw string) (string, bool) {
	name := strings.ToLower(strings.TrimSuffix(raw, "."))
	if len(name) == 0 || len(name) > 253 {
		return "", false
	}
	for _, label := range strings.Split(name, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", false
		}
		for _, c := range label {
			if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' && c != '_' {
				return "", false
			}
		}
	}
	return name, true
}
