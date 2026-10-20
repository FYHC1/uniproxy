package safehttp

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"github.com/uniproxy/uniproxy/internal/version"
)

const MaxBody = 5 * 1024 * 1024

func PublicIP(ip netip.Addr) bool {
	if ip.Zone() != "" {
		return false
	}
	ip = ip.Unmap()
	if !ip.IsValid() || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
		return false
	}
	for _, s := range []string{"0.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4", "2001:db8::/32", "fc00::/7", "fe80::/10", "fec0::/10", "64:ff9b::/96", "64:ff9b:1::/48", "2002::/16"} {
		if netip.MustParsePrefix(s).Contains(ip) {
			return false
		}
	}
	return true
}
func ValidateURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Fragment != "" {
		return nil, failure(KindURL)
	}
	for _, c := range u.Host {
		if c < 33 || c == 92 || c == 127 {
			return nil, failure(KindURL)
		}
	}
	if port := u.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return nil, failure(KindURL)
		}
	}
	return u, nil
}

func Fetch(ctx context.Context, client *http.Client, raw string) (string, error) {
	if ctx.Err() != nil {
		return "", contextFailure(ctx.Err())
	}
	u, err := ValidateURL(raw)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", failure(KindURL)
	}
	req.Header.Set("User-Agent", version.UserAgent()+" Mihomo")
	req.Header.Set("Accept", "text/plain, application/yaml, application/json, */*")
	if client == nil {
		client = NewClient(false)
	}
	res, err := client.Do(req)
	if err != nil {
		return "", sanitizeError(ctx, err, KindDownload)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", &FetchError{Kind: KindHTTPStatus, StatusCode: res.StatusCode}
	}
	if res.ContentLength > MaxBody {
		return "", failure(KindBodyTooLarge)
	}
	// The default transport decompresses gzip first, so this also bounds a
	// compressed expansion, not just the wire Content-Length.
	body, err := io.ReadAll(io.LimitReader(res.Body, MaxBody+1))
	if err != nil {
		return "", sanitizeError(ctx, err, KindBodyRead)
	}
	if ctx.Err() != nil {
		return "", contextFailure(ctx.Err())
	}
	if len(body) > MaxBody {
		return "", failure(KindBodyTooLarge)
	}
	if looksLikeHTML(body) {
		return "", failure(KindHTML)
	}
	// Empty success bodies intentionally remain valid. The caller owns empty
	// subscription/forced-refresh deletion protection, not this transport layer.
	return string(body), nil
}

func looksLikeHTML(body []byte) bool {
	// Only confirmed markup at the beginning is rejected, never Content-Type
	// alone (subscription servers commonly mislabel YAML/plain text).
	if len(body) > 4096 {
		body = body[:4096]
	}
	body = bytes.TrimSpace(body)
	body = bytes.TrimSpace(bytes.TrimPrefix(body, []byte{0xef, 0xbb, 0xbf}))
	for i := 0; i < 8 && bytes.HasPrefix(body, []byte("<!--")); i++ {
		end := bytes.Index(body[4:], []byte("-->"))
		if end < 0 {
			return false
		}
		body = bytes.TrimSpace(body[end+7:])
	}
	prefix := strings.ToLower(string(body))
	for _, tag := range []string{"<!doctype html", "<html", "<head", "<body"} {
		if strings.HasPrefix(prefix, tag) && len(prefix) > len(tag) {
			switch prefix[len(tag)] {
			case '>', ' ', '\t', '\r', '\n', '/':
				return true
			}
		}
	}
	return false
}

func RedactURL(raw string) string {
	u, e := url.Parse(raw)
	if e != nil {
		return "[隐藏的订阅地址]"
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	if u.Path != "" && u.Path != "/" {
		u.Path = "/[redacted]"
		u.RawPath = ""
	}
	return u.String()
}
