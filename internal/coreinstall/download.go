package coreinstall

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/uniproxy/uniproxy/internal/safehttp"
)

const (
	officialRepo     = "https://github.com/MetaCubeX/mihomo"
	releaseAPI       = "https://api.github.com/repos/MetaCubeX/mihomo/releases/tags/"
	maxMetadataBytes = 4 << 20
	downloadTimeout  = 4 * time.Minute
)

// builtinDigest returns the SHA256 recorded for the original official v1.19.32
// release asset. A switch avoids an externally or concurrently mutable pin map.
func builtinDigest(platform string) ([sha256.Size]byte, bool) {
	var raw string
	switch platform {
	case "linux-amd64-compatible":
		raw = "ba3ce607747a07f948fc35780e108a4a7c7f552a38b9bd4d115f313ebcb89c20"
	case "linux-arm64":
		raw = "9dd862e28b46ff7d775f169cceebc28deccaa0a9e804237d421cd2571e0caba0"
	case "windows-amd64-compatible":
		raw = "974a4d7ad69aed27aa2e8f91d61113573c14dadb14562c63e58effabf59816f0"
	case "windows-arm64":
		raw = "7a4f6e58af9a120920935f2150cf642d945085fdb3e753314505b334234e1d1e"
	default:
		return [sha256.Size]byte{}, false
	}
	decoded, err := hex.DecodeString(raw)
	if err != nil || len(decoded) != sha256.Size {
		return [sha256.Size]byte{}, false
	}
	var sum [sha256.Size]byte
	copy(sum[:], decoded)
	return sum, true
}

func assetName(goos, arch string) (string, string, error) {
	if !Supported(goos, arch) {
		return "", "", errors.New("当前系统或架构不支持在线安装；仅支持 Windows/Linux 的 amd64/arm64，请使用受支持的平台")
	}
	platform := goos + "-" + arch
	if arch == "amd64" {
		platform += "-compatible"
	}
	suffix := ".gz"
	if goos == "windows" {
		suffix = ".zip"
	}
	return "mihomo-" + platform + "-v" + PinnedVersion + suffix, platform, nil
}

type release struct {
	TagName    string  `json:"tag_name"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []asset `json:"assets"`
}

type asset struct {
	Name   string `json:"name"`
	Digest string `json:"digest"`
	URL    string `json:"browser_download_url"`
	Size   int64  `json:"size"`
}

func trustedURL(u *url.URL) error {
	if u == nil || u.Scheme != "https" || u.Opaque != "" || u.User != nil || u.Fragment != "" || u.RawFragment != "" {
		return errUntrustedURL
	}
	// Exact hosts only: no wildcard suffixes, IP literals, empty ports, userinfo,
	// trailing dots, or alternate port spellings. Signed CDN queries are allowed.
	host := u.Hostname()
	switch host {
	case "api.github.com", "github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com", "github-releases.githubusercontent.com":
	default:
		return errUntrustedURL
	}
	if u.Host != host && u.Host != host+":443" {
		return errUntrustedURL
	}
	if _, err := safehttp.ValidateURL(u.String()); err != nil {
		return errUntrustedURL
	}
	return nil
}

func newDownloadClient(transport http.RoundTripper) *http.Client {
	client := safehttp.NewClient(false)
	// Keep its DNS/public-IP checks, Fake-IP DoH verification, numeric-pinned
	// dialing, TLS policy and bounded dial/header timeouts. Only extend the
	// overall request/body timeout; the outer installation still has 5 minutes.
	client.Timeout = downloadTimeout
	safeRedirect := client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		req.Header.Del("Referer")
		if err := trustedURL(req.URL); err != nil {
			return errRedirect
		}
		if err := safeRedirect(req, via); err != nil {
			return errRedirect
		}
		return nil
	}
	if transport != nil {
		// Only deterministic tests in this package can reach this seam. They
		// still use the real initial-URL/redirect/header guard below.
		client.Transport = transport
	}
	client.Transport = &releaseTransport{base: client.Transport}
	return client
}

// Validate each outgoing request AND the Location value before net/http parses
// it, so neither a malformed redirect nor url.Error can leak a signed CDN URL.
type releaseTransport struct{ base http.RoundTripper }

func (t *releaseTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := trustedURL(req.URL); err != nil {
		return nil, err
	}
	if req.Host != "" && req.Host != req.URL.Host {
		return nil, errUntrustedURL
	}
	r := req.Clone(req.Context())
	r.Header.Del("Referer")
	r.Header.Del("Authorization")
	r.Header.Del("Proxy-Authorization")
	r.Header.Del("Cookie")
	response, err := t.base.RoundTrip(r)
	if err != nil {
		return nil, err
	}
	if isRedirect(response.StatusCode) && response.Header.Get("Location") != "" {
		next, parseErr := r.URL.Parse(response.Header.Get("Location"))
		if parseErr != nil || trustedURL(next) != nil {
			response.Body.Close()
			return nil, errRedirect
		}
	}
	return response, nil
}

func (t *releaseTransport) CloseIdleConnections() {
	if closer, ok := t.base.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

func isRedirect(code int) bool {
	return code == http.StatusMovedPermanently || code == http.StatusFound ||
		code == http.StatusSeeOther || code == http.StatusTemporaryRedirect || code == http.StatusPermanentRedirect
}

func request(ctx context.Context, client *http.Client, address string) (*http.Response, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	u, err := url.Parse(address)
	if err != nil || trustedURL(u) != nil {
		return nil, errUntrustedURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, errUntrustedURL
	}
	req.Header.Set("User-Agent", "UniProxy-coreinstall/"+PinnedVersion)
	req.Header.Set("Accept", "application/octet-stream")
	if u.Hostname() == "api.github.com" {
		req.Header.Set("Accept", "application/vnd.github+json")
	}
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Accept-Encoding", "identity")
	response, err := client.Do(req)
	if err != nil {
		return nil, networkFailure(ctx, err)
	}
	if err := checkContext(ctx); err != nil {
		response.Body.Close()
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, httpFailure(response.StatusCode)
	}
	return response, nil
}

func findAsset(ctx context.Context, client *http.Client, name, platform string, pin func(string) ([sha256.Size]byte, bool)) (asset, [sha256.Size]byte, error) {
	var zero [sha256.Size]byte
	expected, ok := pin(platform)
	if !ok {
		return asset{}, zero, errMetadataPin
	}
	response, err := request(ctx, client, releaseAPI+"v"+PinnedVersion)
	if err != nil {
		return asset{}, zero, err
	}
	defer response.Body.Close()
	if response.ContentLength > maxMetadataBytes {
		return asset{}, zero, errMetadata
	}
	data, err := io.ReadAll(&contextReader{ctx: ctx, reader: io.LimitReader(response.Body, maxMetadataBytes+1)})
	if err != nil {
		return asset{}, zero, networkFailure(ctx, err)
	}
	if err := checkContext(ctx); err != nil {
		return asset{}, zero, err
	}
	if len(data) > maxMetadataBytes {
		return asset{}, zero, errMetadata
	}
	var rel release
	if err := json.Unmarshal(data, &rel); err != nil || rel.TagName != "v"+PinnedVersion || rel.Draft || rel.Prerelease {
		return asset{}, zero, errMetadata
	}
	var selected *asset
	for i := range rel.Assets {
		a := &rel.Assets[i]
		if a.Name != name {
			continue
		}
		if selected != nil || a.URL != officialRepo+"/releases/download/v"+PinnedVersion+"/"+name || a.Size <= 0 || a.Size > maxArchiveBytes {
			return asset{}, zero, errMetadata
		}
		declared, err := parseDigest(a.Digest)
		if err != nil || subtle.ConstantTimeCompare(declared[:], expected[:]) != 1 {
			return asset{}, zero, errMetadataPin
		}
		selected = a
	}
	if selected == nil {
		return asset{}, zero, errMetadata
	}
	return *selected, expected, nil
}

func parseDigest(raw string) ([sha256.Size]byte, error) {
	var sum [sha256.Size]byte
	if !strings.HasPrefix(raw, "sha256:") || len(raw) != len("sha256:")+2*sha256.Size {
		return sum, errMetadataPin
	}
	decoded, err := hex.DecodeString(strings.TrimPrefix(raw, "sha256:"))
	if err != nil || len(decoded) != sha256.Size {
		return sum, errMetadataPin
	}
	copy(sum[:], decoded)
	return sum, nil
}
