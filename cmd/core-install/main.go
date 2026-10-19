// core-install downloads a release asset directly from the official repository,
// verifies its SHA256 digest BEFORE extraction, and atomically installs one binary.
// No downloaded script or executable is run by this command.
package main

import (
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/uniproxy/uniproxy/internal/core"
)

const officialRepo = "https://github.com/MetaCubeX/mihomo"
const releaseAPI = "https://api.github.com/repos/MetaCubeX/mihomo/releases/tags/"
const maxArchiveBytes int64 = 128 << 20
const maxBinaryBytes int64 = 256 << 20

// Recorded from the official v1.19.32 GitHub release asset SHA256 digests.
// Pinning these additionally detects replacement of a same-named release asset.
var pinnedDigests = map[string]string{
	"linux-amd64-compatible":   "ba3ce607747a07f948fc35780e108a4a7c7f552a38b9bd4d115f313ebcb89c20",
	"linux-arm64":              "9dd862e28b46ff7d775f169cceebc28deccaa0a9e804237d421cd2571e0caba0",
	"windows-amd64-compatible": "974a4d7ad69aed27aa2e8f91d61113573c14dadb14562c63e58effabf59816f0",
	"windows-arm64":            "7a4f6e58af9a120920935f2150cf642d945085fdb3e753314505b334234e1d1e",
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
type installOptions struct {
	Version, OS, Arch, Output string
	Force                     bool
}

func main() {
	flags := flag.NewFlagSet("core-install", flag.ExitOnError)
	version := flags.String("version", core.PinnedVersion, "official stable release version (runtime requires the pinned version)")
	goos := flags.String("os", runtime.GOOS, "target OS: windows or linux")
	arch := flags.String("arch", runtime.GOARCH, "target architecture: amd64 or arm64")
	output := flags.String("output", "", "binary destination (default data/core/mihomo[.exe])")
	force := flags.Bool("force", false, "replace an existing binary; stop UniProxy's core first")
	_ = flags.Parse(os.Args[1:])
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	client := &http.Client{Timeout: 4 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 5 {
			return errors.New("too many release download redirects")
		}
		return trustedURL(req.URL)
	}}
	dest, err := install(ctx, client, installOptions{*version, *goos, *arch, *output, *force})
	if err != nil {
		fmt.Fprintln(os.Stderr, "core-install:", err)
		os.Exit(1)
	}
	fmt.Printf("Verified Mihomo v%s installed at %s\n", strings.TrimPrefix(*version, "v"), dest)
	if strings.TrimPrefix(*version, "v") != core.PinnedVersion {
		fmt.Fprintf(os.Stderr, "Warning: UniProxy runtime currently requires v%s; version overrides are for explicit testing only.\n", core.PinnedVersion)
	}
}

var versionRE = regexp.MustCompile(`^v?([0-9]+[.][0-9]+[.][0-9]+)$`)

func assetName(version, goos, arch string) (string, string, error) {
	match := versionRE.FindStringSubmatch(version)
	if len(match) != 2 || len(version) > 32 {
		return "", "", errors.New("version must be a stable numeric release such as 1.19.32")
	}
	if goos != "windows" && goos != "linux" {
		return "", "", errors.New("supported target operating systems are windows and linux")
	}
	if arch != "amd64" && arch != "arm64" {
		return "", "", errors.New("supported target architectures are amd64 and arm64")
	}
	platform := goos + "-" + arch
	if arch == "amd64" {
		platform += "-compatible"
	}
	suffix := ".gz"
	if goos == "windows" {
		suffix = ".zip"
	}
	return "mihomo-" + platform + "-v" + match[1] + suffix, platform, nil
}
func trustedURL(u *url.URL) error {
	if u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") {
		return errors.New("release requests must use trusted HTTPS endpoints")
	}
	switch u.Hostname() {
	case "api.github.com", "github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com", "github-releases.githubusercontent.com":
		return nil
	}
	return errors.New("release redirect points to an untrusted host")
}
func request(ctx context.Context, client *http.Client, address string) (*http.Response, error) {
	u, err := url.Parse(address)
	if err != nil {
		return nil, errors.New("invalid release URL")
	}
	if err = trustedURL(u); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, errors.New("cannot create release request")
	}
	req.Header.Set("User-Agent", "UniProxy-core-install/0.1")
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	response, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("official release download failed")
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, fmt.Errorf("official release endpoint returned HTTP %d", response.StatusCode)
	}
	return response, nil
}
func findAsset(ctx context.Context, client *http.Client, version, name, platform string) (asset, error) {
	response, err := request(ctx, client, releaseAPI+"v"+version)
	if err != nil {
		return asset{}, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 4<<20+1))
	if err != nil || len(data) > 4<<20 {
		return asset{}, errors.New("release metadata is invalid or too large")
	}
	var rel release
	if err = json.Unmarshal(data, &rel); err != nil {
		return asset{}, errors.New("release metadata is not valid JSON")
	}
	if rel.TagName != "v"+version || rel.Draft || rel.Prerelease {
		return asset{}, errors.New("release metadata does not match the requested stable version")
	}
	for _, a := range rel.Assets {
		if a.Name != name {
			continue
		}
		if a.URL != officialRepo+"/releases/download/v"+version+"/"+name {
			return asset{}, errors.New("release asset URL does not match the official release")
		}
		if a.Size <= 0 || a.Size > maxArchiveBytes {
			return asset{}, errors.New("release asset size is outside the safety limit")
		}
		digest, err := parseDigest(a.Digest)
		if err != nil {
			return asset{}, err
		}
		if version == core.PinnedVersion && hex.EncodeToString(digest) != pinnedDigests[platform] {
			return asset{}, errors.New("release asset digest differs from the built-in pinned SHA256")
		}
		return a, nil
	}
	return asset{}, errors.New("official release has no asset for this platform")
}
func parseDigest(raw string) ([]byte, error) {
	if !strings.HasPrefix(raw, "sha256:") || len(raw) != len("sha256:")+64 {
		return nil, errors.New("official asset has no usable SHA256 digest; refusing unverified download")
	}
	sum, err := hex.DecodeString(strings.TrimPrefix(raw, "sha256:"))
	if err != nil || len(sum) != sha256.Size {
		return nil, errors.New("official asset SHA256 digest is invalid")
	}
	return sum, nil
}
func install(ctx context.Context, client *http.Client, opts installOptions) (string, error) {
	name, platform, err := assetName(opts.Version, opts.OS, opts.Arch)
	if err != nil {
		return "", err
	}
	version := strings.TrimPrefix(opts.Version, "v")
	output := opts.Output
	if output == "" {
		output = filepath.Join("data", "core", "mihomo")
		if opts.OS == "windows" {
			output += ".exe"
		}
	}
	destination, err := filepath.Abs(output)
	if err != nil {
		return "", errors.New("invalid output path")
	}
	if info, err := os.Lstat(destination); err == nil {
		if !info.Mode().IsRegular() {
			return "", errors.New("destination exists and is not a regular file")
		}
		if !opts.Force {
			return "", errors.New("destination already exists; use -force only after stopping the core")
		}
	} else if !os.IsNotExist(err) {
		return "", errors.New("cannot inspect destination")
	}
	a, err := findAsset(ctx, client, version, name, platform)
	if err != nil {
		return "", err
	}
	if err = os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		return "", errors.New("cannot create installation directory")
	}
	archive, err := os.CreateTemp(filepath.Dir(destination), ".mihomo-archive-*")
	if err != nil {
		return "", errors.New("cannot create temporary download")
	}
	defer os.Remove(archive.Name())
	defer archive.Close()
	response, err := request(ctx, client, a.URL)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(archive, hash), io.LimitReader(response.Body, maxArchiveBytes+1))
	response.Body.Close()
	if copyErr != nil || n != a.Size || n > maxArchiveBytes {
		return "", errors.New("release download was truncated or exceeded its advertised size")
	}
	expected, _ := parseDigest(a.Digest)
	if !equalBytes(hash.Sum(nil), expected) {
		return "", errors.New("release SHA256 mismatch; downloaded file was not installed")
	}
	if err = archive.Sync(); err != nil {
		return "", errors.New("cannot flush verified release archive")
	}
	binary, err := os.CreateTemp(filepath.Dir(destination), ".mihomo-install-*")
	if err != nil {
		return "", errors.New("cannot create verified binary staging file")
	}
	defer os.Remove(binary.Name())
	defer binary.Close()
	if err = extractBinary(archive, n, name, binary); err != nil {
		return "", err
	}
	if err = binary.Sync(); err != nil {
		return "", errors.New("cannot flush extracted binary")
	}
	if err = binary.Chmod(0700); err != nil {
		return "", errors.New("cannot set binary permissions")
	}
	if err = binary.Close(); err != nil {
		return "", errors.New("cannot close staged binary")
	}
	// Refuse an unexpected destination created during download. Rename replacement
	// is intentional only with -force; a running Windows executable fails safely.
	if !opts.Force {
		if _, err = os.Lstat(destination); !os.IsNotExist(err) {
			return "", errors.New("destination appeared during installation; refusing replacement")
		}
	}
	if err = os.Rename(binary.Name(), destination); err != nil {
		return "", errors.New("cannot atomically install verified binary; ensure the core is stopped")
	}
	return destination, nil
}
func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := range a {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}

func extractBinary(archive *os.File, size int64, name string, output io.Writer) error {
	var input io.ReadCloser
	if strings.HasSuffix(name, ".zip") {
		reader, err := zip.NewReader(archive, size)
		if err != nil {
			return errors.New("verified release ZIP is invalid")
		}
		var entry *zip.File
		for _, f := range reader.File {
			// Never extract archive paths: only copy the sole expected executable into
			// our already-open destination. Reject symlinks and ambiguous executables.
			if strings.HasSuffix(strings.ToLower(f.Name), ".exe") {
				if filepath.Base(f.Name) != f.Name || strings.ContainsAny(f.Name, "/\\") || !strings.HasPrefix(f.Name, "mihomo") || f.Mode()&os.ModeSymlink != 0 || f.UncompressedSize64 > uint64(maxBinaryBytes) {
					return errors.New("release ZIP contains an unsafe executable entry")
				}
				if entry != nil {
					return errors.New("release ZIP contains multiple executables")
				}
				entry = f
			}
		}
		if entry == nil {
			return errors.New("release ZIP has no Mihomo executable")
		}
		input, err = entry.Open()
		if err != nil {
			return errors.New("cannot open verified executable entry")
		}
	} else if strings.HasSuffix(name, ".gz") {
		if _, err := archive.Seek(0, io.SeekStart); err != nil {
			return errors.New("cannot read verified archive")
		}
		reader, err := gzip.NewReader(archive)
		if err != nil {
			return errors.New("verified release gzip is invalid")
		}
		input = reader
	} else {
		return errors.New("unsupported release archive format")
	}
	defer input.Close()
	n, err := io.Copy(output, io.LimitReader(input, maxBinaryBytes+1))
	if err != nil || n == 0 || n > maxBinaryBytes {
		return errors.New("verified binary extraction failed or exceeded its safety limit")
	}
	return nil
}
