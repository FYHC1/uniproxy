package sidecarinstall

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
	officialRepo     = "https://github.com/XTLS/Xray-core"
	releaseAPI       = "https://api.github.com/repos/XTLS/Xray-core/releases/tags/"
	maxMetadataBytes = 4 << 20
	downloadTimeout  = 4 * time.Minute
)

// builtinDigest returns the SHA256 recorded for the original official v26.10.31
// release asset. A switch avoids an externally or concurrently mutable pin map.
func builtinDigest(platform string) ([sha256.Size]byte, bool) {
	var raw string
	switch platform {
	case "linux-amd64":
		raw = "e4bd9119f50ae7c41658b1abd0679ae06f98bd864a2907ff33f2f2cf7081b214"
	case "linux-arm64":
		raw = "1811c8f1da0c3b6dc707ebaf1c8390e987ba39859a8f0a7aadd8526157f2ec8d"
	case "windows-amd64":
		raw = "2e04a9995a6f2ad1b28c0bb25c708ecb2695e43a1c9c7889c4b6ac2b48cf7a6a"
	case "windows-arm64":
		raw = "db062e8a177facacc8ae53d4c9c0df4132b13fc3a89acda6f21d92d36633077e"
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
		return "", "", errors.New("当前系统或架构不支持 Xray 在线安装；仅支持 Windows/Linux 的 amd64/arm64，请使用受支持的平台")
	}
	if goos == "windows" {
		if arch == "amd64" {
			return "Xray-windows-64.zip", "windows-amd64", nil
		}
		return "Xray-windows-arm64-v8a.zip", "windows-arm64", nil
	}
	if arch == "amd64" {
		return "Xray-linux-64.zip", "linux-amd64", nil
	}
	return "Xray-linux-arm64-v8a.zip", "linux-arm64", nil
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
		client.Transport = transport
	}
	client.Transport = &releaseTransport{base: client.Transport}
	return client
}

// releaseTransport validates each outgoing request and the Location value
// before net/http parses it, so neither a malformed redirect nor url.Error can
// leak a signed CDN URL.
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
	req.Header.Set("User-Agent", "UniProxy-sidecarinstall/"+PinnedVersion)
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
