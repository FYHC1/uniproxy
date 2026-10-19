package main

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uniproxy/uniproxy/internal/core"
)

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func gzipBytes(t *testing.T, body []byte) []byte {
	t.Helper()
	var data bytes.Buffer
	writer := gzip.NewWriter(&data)
	if _, err := writer.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}
func zipBytes(t *testing.T, names ...string) []byte {
	t.Helper()
	var data bytes.Buffer
	writer := zip.NewWriter(&data)
	for _, name := range names {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		entry.Write([]byte("test binary"))
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}
func TestAssetNameAndStableVersionValidation(t *testing.T) {
	for _, goos := range []string{"windows", "linux"} {
		for _, arch := range []string{"amd64", "arm64"} {
			name, platform, err := assetName("v1.19.32", goos, arch)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(name, "mihomo-"+platform+"-v1.19.32.") {
				t.Fatal(name)
			}
			if len(pinnedDigests[platform]) != 64 {
				t.Fatal("missing pinned platform digest")
			}
		}
	}
	for _, version := range []string{"latest", "1.19", "../../evil", "1.19.32-dev", "1.19.32?token=secret", ""} {
		if _, _, err := assetName(version, "linux", "amd64"); err == nil {
			t.Fatalf("unsafe version accepted %q", version)
		}
	}
	for _, pair := range [][2]string{{"darwin", "amd64"}, {"windows", "386"}, {"linux", "arm"}} {
		if _, _, err := assetName(core.PinnedVersion, pair[0], pair[1]); err == nil {
			t.Fatal("unsupported platform accepted")
		}
	}
}
func TestInstallerRequiresDigestAndTrustedHTTPS(t *testing.T) {
	for _, raw := range []string{"", "sha512:" + strings.Repeat("a", 64), "sha256:abc", "sha256:" + strings.Repeat("z", 64)} {
		if _, err := parseDigest(raw); err == nil {
			t.Fatal("invalid digest accepted")
		}
	}
	for _, raw := range []string{"http://github.com/release", "https://evil.test/release", "https://github.com@evil.test/release", "https://github.com:444/release", "https://user:secret@github.com/release"} {
		u, _ := url.Parse(raw)
		if err := trustedURL(u); err == nil {
			t.Fatal("untrusted download URL accepted", raw)
		}
	}
	for _, raw := range []string{"https://api.github.com/", "https://github.com/MetaCubeX/mihomo", "https://release-assets.githubusercontent.com/path"} {
		u, _ := url.Parse(raw)
		if err := trustedURL(u); err != nil {
			t.Fatal(err)
		}
	}
}
func TestInstallVerifiesBeforeExtractionAndSupportsOverrides(t *testing.T) {
	for _, bad := range []string{"", "digest", "size", "missing-digest", "wrong-tag", "wrong-url", "metadata-override-pin"} {
		t.Run(bad, func(t *testing.T) {
			version := "1.20.0"
			if bad == "metadata-override-pin" {
				version = core.PinnedVersion
			}
			binary := []byte("synthetic verified binary, never executed")
			archive := gzipBytes(t, binary)
			sum := sha256.Sum256(archive)
			name, _, _ := assetName(version, "linux", "amd64")
			assetURL := officialRepo + "/releases/download/v" + version + "/" + name
			a := asset{Name: name, Digest: "sha256:" + hex.EncodeToString(sum[:]), URL: assetURL, Size: int64(len(archive))}
			rel := release{TagName: "v" + version, Assets: []asset{a}}
			switch bad {
			case "digest":
				rel.Assets[0].Digest = "sha256:" + strings.Repeat("0", 64)
			case "size":
				rel.Assets[0].Size++
			case "missing-digest":
				rel.Assets[0].Digest = ""
			case "wrong-tag":
				rel.TagName = "v9.9.9"
			case "wrong-url":
				rel.Assets[0].URL = "https://github.com/attacker/release"
			}
			metadata, _ := json.Marshal(rel)
			client := &http.Client{Transport: roundTripper(func(r *http.Request) (*http.Response, error) {
				var body []byte
				if strings.HasPrefix(r.URL.String(), releaseAPI) {
					body = metadata
				} else if r.URL.String() == assetURL {
					body = archive
				} else {
					t.Fatalf("unexpected network request %s", r.URL)
					return nil, nil
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(body)), Request: r}, nil
			})}
			output := filepath.Join(t.TempDir(), "mihomo")
			destination, err := install(context.Background(), client, installOptions{Version: version, OS: "linux", Arch: "amd64", Output: output})
			if bad != "" {
				if err == nil {
					t.Fatal("unsafe archive installed")
				}
				if _, statErr := os.Stat(output); !os.IsNotExist(statErr) {
					t.Fatal("failed install wrote destination")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(destination)
			if err != nil || !bytes.Equal(data, binary) {
				t.Fatalf("wrong installed bytes %q %v", data, err)
			}
			if _, err = install(context.Background(), client, installOptions{Version: version, OS: "linux", Arch: "amd64", Output: output}); err == nil {
				t.Fatal("existing binary replaced without -force")
			}
			if _, err = install(context.Background(), client, installOptions{Version: version, OS: "linux", Arch: "amd64", Output: output, Force: true}); err != nil {
				t.Fatal("explicit replacement failed", err)
			}
		})
	}
}
func TestZIPExtractionRejectsUnsafePathsAndAmbiguity(t *testing.T) {
	for _, names := range [][]string{{"mihomo.exe"}, {"../mihomo.exe"}, {"nested/mihomo.exe"}, {"mihomo.exe", "mihomo-extra.exe"}, {"attacker.exe"}, {"README.txt"}} {
		data := zipBytes(t, names...)
		path := filepath.Join(t.TempDir(), "download.zip")
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		archive, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		err = extractBinary(archive, int64(len(data)), "mihomo-windows.zip", &out)
		archive.Close()
		if len(names) == 1 && names[0] == "mihomo.exe" {
			if err != nil || out.String() != "test binary" {
				t.Fatalf("valid ZIP failed %v", err)
			}
		} else if err == nil {
			t.Fatalf("unsafe ZIP accepted %v", names)
		}
	}
}
func TestFailedInstallLeavesExistingBinaryUntouched(t *testing.T) {
	output := filepath.Join(t.TempDir(), "mihomo")
	if err := os.WriteFile(output, []byte("previous binary"), 0700); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: roundTripper(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 403, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("forbidden")), Request: r}, nil
	})}
	if _, err := install(context.Background(), client, installOptions{Version: core.PinnedVersion, OS: "linux", Arch: "amd64", Output: output, Force: true}); err == nil {
		t.Fatal("unexpected successful install")
	}
	data, err := os.ReadFile(output)
	if err != nil || string(data) != "previous binary" {
		t.Fatal("failed install changed existing binary")
	}
}
