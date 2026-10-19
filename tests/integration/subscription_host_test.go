package integration

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/uniproxy/uniproxy/internal/safehttp"
)

// Opt-in diagnostics for a user-authorized PUBLIC hostname only. This is not a
// subscription-content test: it never accepts a URL/token, reads a body or follows redirects.
func TestSubscriptionHostTransport(t *testing.T) {
	host := os.Getenv("UNIPROXY_TEST_SUBSCRIPTION_HOST")
	if host == "" {
		t.Skip("set UNIPROXY_TEST_SUBSCRIPTION_HOST to an authorized bare public hostname")
	}
	if strings.ContainsAny(host, "/:?@#%\\") || net.ParseIP(host) != nil || !regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9.-]*[A-Za-z0-9])?$`).MatchString(host) || !strings.Contains(host, ".") {
		t.Fatal("test requires a bare public DNS hostname, never a URL, path or credential")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client := safehttp.NewClient(false)
	defer client.CloseIdleConnections()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	u := url.URL{Scheme: "https", Host: host, Path: "/"}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, u.String(), nil)
	if err != nil {
		t.Fatal("could not prepare root-only HEAD")
	}
	req.Header.Set("User-Agent", "UniProxy/0.1.0 Mihomo")
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("root-only TLS transport failed, safe category=%s", safehttp.ErrorKindOf(err))
	}
	res.Body.Close()
	if res.TLS == nil || len(res.TLS.VerifiedChains) == 0 {
		t.Fatal("HTTPS certificate was not verified")
	}
	t.Logf("Root-only HEAD completed: HTTP%d, verified TLS; no redirect followed, response body read, or subscription requested", res.StatusCode)
}
