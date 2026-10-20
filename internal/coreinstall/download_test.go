package coreinstall

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/uniproxy/uniproxy/internal/safehttp"
)

func TestTrustedURLUsesExactHTTPSGitHubAllowlist(t *testing.T) {
	for _, address := range []string{
		"https://github.com/MetaCubeX/mihomo", "https://api.github.com/", "https://github.com:443/",
		"https://release-assets.githubusercontent.com/path?sig=synthetic", "https://objects.githubusercontent.com/path", "https://github-releases.githubusercontent.com/path",
	} {
		u, err := url.Parse(address)
		if err != nil || trustedURL(u) != nil {
			t.Fatal("trusted address rejected", address)
		}
	}
	for _, address := range []string{
		"http://github.com/path", "ftp://github.com/path", "https://evil.test/path", "https://github.com.evil.test/path",
		"https://github.com@evil.test/path", "https://user:SECRET@github.com/path", "https://github.com:444/path", "https://github.com:/path",
		"https://github.com:0443/path", "https://github.com./path", "https://GITHUB.COM/path", "https://github.com/path#SECRET",
		"https://raw.githubusercontent.com/path", "https://127.0.0.1/path", "https://198.18.0.1/path", "https://[::1]/path", "//github.com/path",
		"https:github.com/path", "https://api.github.com.evil.test/", "https://[github.com]/path",
	} {
		u, err := url.Parse(address)
		if err == nil && trustedURL(u) == nil {
			t.Fatal("untrusted address accepted", address)
		}
	}
	if trustedURL(nil) == nil {
		t.Fatal("nil URL accepted")
	}
}

func TestDigestParserRequiresFullSHA256(t *testing.T) {
	for _, raw := range []string{"", "sha512:" + strings.Repeat("a", 64), "sha256:abc", "sha256:" + strings.Repeat("z", 64), " sha256:" + strings.Repeat("a", 64)} {
		if _, err := parseDigest(raw); err == nil {
			t.Fatal("unsafe digest accepted", raw)
		}
	}
	for _, letter := range []string{"a", "A"} {
		sum, err := parseDigest("sha256:" + strings.Repeat(letter, 64))
		if err != nil || hex.EncodeToString(sum[:]) != strings.Repeat("a", 64) {
			t.Fatal("valid SHA256 rejected", err)
		}
	}
}

func TestMetadataRejectsMismatchBeforeAnyArchiveDownload(t *testing.T) {
	for _, bad := range []string{"tag", "draft", "prerelease", "missing-asset", "duplicate-asset", "url", "query", "http", "zero", "negative", "large", "digest-empty", "digest-algorithm", "digest-invalid", "digest-mismatch", "no-pin"} {
		t.Run(bad, func(t *testing.T) {
			f := newFixture(t, "linux", "amd64")
			deps := f.deps()
			a := &f.release.Assets[0]
			switch bad {
			case "tag":
				f.release.TagName = "v1.20.0"
			case "draft":
				f.release.Draft = true
			case "prerelease":
				f.release.Prerelease = true
			case "missing-asset":
				a.Name = "mihomo-other.gz"
			case "duplicate-asset":
				f.release.Assets = append(f.release.Assets, *a)
			case "url":
				a.URL = "https://github.com/attacker/release"
			case "query":
				a.URL += "?override=1"
			case "http":
				a.URL = strings.Replace(a.URL, "https:", "http:", 1)
			case "zero":
				a.Size = 0
			case "negative":
				a.Size = -1
			case "large":
				a.Size = maxArchiveBytes + 1
			case "digest-empty":
				a.Digest = ""
			case "digest-algorithm":
				a.Digest = "sha512:" + strings.Repeat("a", 64)
			case "digest-invalid":
				a.Digest = "sha256:" + strings.Repeat("z", 64)
			case "digest-mismatch":
				a.Digest = "sha256:" + strings.Repeat("0", 64)
			case "no-pin":
				deps.pin = func(string) ([sha256.Size]byte, bool) { return [sha256.Size]byte{}, false }
			}
			output := filepath.Join(t.TempDir(), "mihomo")
			_, err := install(context.Background(), Options{OS: "linux", Arch: "amd64", Output: output}, nil, deps)
			if err == nil || f.downloadCalls.Load() != 0 {
				t.Fatal("bad metadata reached archive", err)
			}
			assertAbsent(t, output)
		})
	}
}

func TestMetadataBodyLimitsErrorsAndClose(t *testing.T) {
	for _, bad := range []string{"invalid-json", "trailing-json", "oversize-header", "oversize-body", "read-error", "canceled"} {
		t.Run(bad, func(t *testing.T) {
			f := newFixture(t, "linux", "amd64")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var body *trackedBody
			f.onResponse = func(_ *http.Request, response *http.Response) {
				body = &trackedBody{reader: strings.NewReader("{SECRET invalid")}
				response.Body = body
				response.ContentLength = -1
				switch bad {
				case "trailing-json":
					body.reader = strings.NewReader("{} {}")
				case "oversize-header":
					response.ContentLength = maxMetadataBytes + 1
				case "oversize-body":
					body.reader = io.LimitReader(repeatReader(' '), maxMetadataBytes+1)
				case "read-error":
					body.reader = failingReader{errors.New("SECRET response failure")}
				case "canceled":
					cancel()
				}
			}
			output := filepath.Join(t.TempDir(), "mihomo")
			_, err := install(ctx, Options{OS: "linux", Arch: "amd64", Output: output}, nil, f.deps())
			if err == nil || strings.Contains(err.Error(), "SECRET") || f.downloadCalls.Load() != 0 {
				t.Fatal("metadata failure mishandled", err)
			}
			if body == nil || !body.closed {
				t.Fatal("metadata body not closed")
			}
			if bad == "canceled" && !errors.Is(err, context.Canceled) {
				t.Fatal("cancellation identity lost", err)
			}
			assertAbsent(t, output)
		})
	}
}

type repeatReader byte

func (r repeatReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(r)
	}
	return len(p), nil
}

func TestRequestRejectsInitialURLBeforeTransport(t *testing.T) {
	calls := 0
	client := newDownloadClient(roundTripper(func(req *http.Request) (*http.Response, error) { calls++; return testResponse(req, nil), nil }))
	defer client.CloseIdleConnections()
	for _, address := range []string{"https://evil.test/?SECRET", "http://github.com/", "https://github.com/%zz", "https://github.com:444/", "https://user:SECRET@github.com/"} {
		_, err := request(context.Background(), client, address)
		if err == nil || strings.Contains(err.Error(), "SECRET") {
			t.Fatal("bad request accepted/leaked", err)
		}
	}
	if calls != 0 {
		t.Fatal("unsafe initial request reached transport")
	}
}

func TestRedirectsValidateEveryHopAndStripSensitiveHeaders(t *testing.T) {
	addresses := []string{
		"https://github.com/MetaCubeX/mihomo/releases/download/v1.19.32/asset?SECRET-SIGNED-QUERY",
		"https://release-assets.githubusercontent.com/asset?SECRET-SIGNED-QUERY",
		"https://objects.githubusercontent.com/asset?SECRET-SIGNED-QUERY",
		"https://github-releases.githubusercontent.com/asset?SECRET-SIGNED-QUERY",
	}
	var calls int
	var bodies []*trackedBody
	client := newDownloadClient(roundTripper(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != addresses[calls] {
			t.Errorf("unexpected redirect: %s", req.URL)
		}
		for _, header := range []string{"Referer", "Authorization", "Proxy-Authorization", "Cookie"} {
			if req.Header.Get(header) != "" {
				t.Errorf("sensitive %s forwarded", header)
			}
		}
		body := &trackedBody{reader: strings.NewReader("fixture")}
		bodies = append(bodies, body)
		response := testResponse(req, nil)
		response.Body = body
		response.ContentLength = -1
		if calls+1 < len(addresses) {
			response.StatusCode = 302
			response.Header.Set("Location", addresses[calls+1])
		}
		calls++
		return response, nil
	}))
	defer client.CloseIdleConnections()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, addresses[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, header := range []string{"Referer", "Authorization", "Proxy-Authorization", "Cookie"} {
		req.Header.Set(header, "SECRET")
	}
	response, err := client.Do(req)
	if err != nil {
		t.Fatal("trusted chain failed", err)
	}
	response.Body.Close()
	if calls != 4 {
		t.Fatal("trusted chain incomplete", calls)
	}
	for _, body := range bodies {
		if !body.closed {
			t.Fatal("redirect/final body leaked")
		}
	}
}

func TestUntrustedRedirectsCloseBodiesAndNeverDialTarget(t *testing.T) {
	for _, target := range []string{
		"http://github.com/?SECRET", "https://evil.test/?SECRET", "https://github.com.evil.test/?SECRET", "https://github.com:444/?SECRET",
		"https://user:SECRET@github.com/", "https://github.com/path#SECRET", "https://[invalid/?SECRET", "https://github.com/%zz?SECRET",
		"https://127.0.0.1/?SECRET", "https://198.18.0.1/?SECRET", "//evil.test/SECRET",
	} {
		t.Run(target, func(t *testing.T) {
			calls := 0
			body := &trackedBody{reader: strings.NewReader("SECRET body")}
			client := newDownloadClient(roundTripper(func(req *http.Request) (*http.Response, error) {
				calls++
				response := testResponse(req, nil)
				response.StatusCode = 302
				response.Header.Set("Location", target)
				response.Body = body
				return response, nil
			}))
			defer client.CloseIdleConnections()
			_, err := request(context.Background(), client, "https://github.com/release?SECRET-origin")
			if !errors.Is(err, errRedirect) || strings.Contains(err.Error(), "SECRET") || calls != 1 || !body.closed {
				t.Fatalf("unsafe redirect handling: calls=%d closed=%v err=%v", calls, body.closed, err)
			}
		})
	}
}

func TestLaterRedirectAndRedirectLoopAreBounded(t *testing.T) {
	for _, badLater := range []bool{true, false} {
		calls := 0
		client := newDownloadClient(roundTripper(func(req *http.Request) (*http.Response, error) {
			calls++
			response := testResponse(req, nil)
			response.StatusCode = 302
			response.Header.Set("Location", "https://release-assets.githubusercontent.com/loop")
			if badLater && calls == 2 {
				response.Header.Set("Location", "https://evil.test/SECRET")
			}
			return response, nil
		}))
		_, err := request(context.Background(), client, "https://github.com/loop")
		client.CloseIdleConnections()
		want := 4
		if badLater {
			want = 2
		}
		if !errors.Is(err, errRedirect) || calls != want {
			t.Fatalf("chain not guarded at every hop: %d %v", calls, err)
		}
	}
}

func TestRequestHeadersAndLongTimeout(t *testing.T) {
	client := newDownloadClient(roundTripper(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet || req.Header.Get("Accept-Encoding") != "identity" || req.Header.Get("User-Agent") == "" || req.Header.Get("Referer") != "" {
			t.Error("unsafe or incomplete request headers")
		}
		if req.URL.Hostname() == "api.github.com" && req.Header.Get("Accept") != "application/vnd.github+json" {
			t.Error("wrong metadata Accept")
		}
		return testResponse(req, nil), nil
	}))
	defer client.CloseIdleConnections()
	if client.Timeout != 4*time.Minute || client.Jar != nil {
		t.Fatal("wrong client policy")
	}
	for _, address := range []string{releaseAPI + "v" + PinnedVersion, officialRepo + "/release"} {
		response, err := request(context.Background(), client, address)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
	}
}

func TestProductionClientComposesSafeHTTPAndIgnoresEnvironmentProxy(t *testing.T) {
	for _, variable := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"} {
		t.Setenv(variable, "http://127.0.0.1:1/SECRET")
	}
	client := newDownloadClient(nil)
	defer client.CloseIdleConnections()
	wrapped, ok := client.Transport.(*releaseTransport)
	if !ok {
		t.Fatal("missing host guard")
	}
	// Do not replace the opaque safehttp guard to customize timeouts: it owns
	// DNS/Fake-IP/TLS protection and uses Proxy:nil (not DefaultTransport).
	if got := reflect.TypeOf(wrapped.base).String(); got != "*safehttp.guardedTransport" {
		t.Fatal("safehttp transport lost", got)
	}
	if client.Timeout <= 25*time.Second || client.Timeout > installTimeout {
		t.Fatal("download timeout not safely extended")
	}
	// No request is made: this remains a deterministic offline wiring test.
}

func TestTransportHostOverrideIsRejected(t *testing.T) {
	calls := 0
	client := newDownloadClient(roundTripper(func(req *http.Request) (*http.Response, error) { calls++; return testResponse(req, nil), nil }))
	req, _ := http.NewRequest(http.MethodGet, "https://github.com/", nil)
	req.Host = "evil.test"
	_, err := client.Do(req)
	if !errors.Is(err, errUntrustedURL) || calls != 0 {
		t.Fatal("Host override accepted", err)
	}
}

func TestHTTPAndTransportErrorsNeverLeakSecrets(t *testing.T) {
	for _, code := range []int{301, 401, 403, 404, 429, 500, 503, 999} {
		body := &trackedBody{reader: strings.NewReader("SECRET upstream body")}
		client := newDownloadClient(roundTripper(func(req *http.Request) (*http.Response, error) {
			response := testResponse(req, nil)
			response.StatusCode = code
			response.Body = body
			return response, nil
		}))
		_, err := request(context.Background(), client, "https://github.com/?SECRET")
		if err == nil || strings.Contains(err.Error(), "SECRET") || !body.closed {
			t.Fatal("unsafe HTTP error", err)
		}
		client.CloseIdleConnections()
	}
	for _, raw := range []error{
		&url.Error{Op: "Get", URL: "https://github.com/?SECRET", Err: errors.New("SECRET transport details")},
		&safehttp.FetchError{Kind: safehttp.KindDNS}, &safehttp.FetchError{Kind: safehttp.KindTLS},
		&safehttp.FetchError{Kind: safehttp.KindPrivateAddress}, &safehttp.FetchError{Kind: safehttp.KindFakeIPFallback},
		&safehttp.FetchError{Kind: safehttp.KindConnect}, &safehttp.FetchError{Kind: safehttp.KindTimeout},
		&os.PathError{Op: "open", Path: "SECRET-path", Err: os.ErrPermission}, context.Canceled, context.DeadlineExceeded,
	} {
		client := newDownloadClient(roundTripper(func(*http.Request) (*http.Response, error) { return nil, raw }))
		_, err := request(context.Background(), client, "https://github.com/?SECRET")
		client.CloseIdleConnections()
		if err == nil {
			t.Fatal("raw error ignored")
		}
		for e := err; e != nil; e = errors.Unwrap(e) {
			if strings.Contains(e.Error(), "SECRET") {
				t.Fatal("raw cause leaked", e)
			}
		}
	}
}

func TestDownloadIdentityBytesNotHTTPDecompressed(t *testing.T) {
	f := newFixture(t, "linux", "amd64")
	f.onResponse = func(req *http.Request, response *http.Response) {
		if req.URL.String() == f.release.Assets[0].URL && req.Header.Get("Accept-Encoding") != "identity" {
			t.Fatal("archive requested compressed HTTP representation")
		}
	}
	output := filepath.Join(t.TempDir(), "mihomo")
	_, err := install(context.Background(), Options{OS: "linux", Arch: "amd64", Output: output}, nil, f.deps())
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(output)
	if err != nil || !bytes.Equal(body, f.binary) {
		t.Fatal("wrong decoded output", err)
	}
}
