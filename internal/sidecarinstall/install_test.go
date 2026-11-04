package sidecarinstall

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func testResponse(req *http.Request, data []byte) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK, Header: make(http.Header),
		Body: io.NopCloser(bytes.NewReader(data)), ContentLength: int64(len(data)), Request: req,
	}
}

type zipEntry struct {
	name string
	body string
	mode os.FileMode
}

func zipBytes(t *testing.T, entries ...zipEntry) []byte {
	t.Helper()
	var data bytes.Buffer
	writer := zip.NewWriter(&data)
	for _, spec := range entries {
		header := &zip.FileHeader{Name: spec.name, Method: zip.Deflate}
		if spec.mode != 0 {
			header.SetMode(spec.mode)
		}
		entry, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(spec.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

type fixture struct {
	files         map[string]string
	archive       []byte
	sum           [sha256.Size]byte
	platform      string
	release       release
	metadataCalls atomic.Int64
	downloadCalls atomic.Int64
}

func newFixture(t *testing.T, goos, arch string) *fixture {
	t.Helper()
	name, platform, err := assetName(goos, arch)
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{files: map[string]string{"xray": "synthetic verified fixture; never execute", "xray.exe": "synthetic verified fixture; never execute", "geoip.dat": "geo fixture", "geosite.dat": "geosite fixture", "wintun.dll": "wintun fixture", "LICENSE": "license fixture"}, platform: platform}
	entries := []zipEntry{{name: "README.md", body: "ignored readme"}, {name: "LICENSE", body: f.files["LICENSE"]}}
	if goos == "windows" {
		entries = append(entries, zipEntry{name: "xray.exe", body: f.files["xray.exe"], mode: 0755}, zipEntry{name: "geoip.dat", body: f.files["geoip.dat"]}, zipEntry{name: "geosite.dat", body: f.files["geosite.dat"]}, zipEntry{name: "wintun.dll", body: f.files["wintun.dll"]})
	} else {
		entries = append(entries, zipEntry{name: "xray", body: f.files["xray"], mode: 0755}, zipEntry{name: "geoip.dat", body: f.files["geoip.dat"]}, zipEntry{name: "geosite.dat", body: f.files["geosite.dat"]})
	}
	f.archive = zipBytes(t, entries...)
	f.sum = sha256.Sum256(f.archive)
	f.release = release{TagName: "v" + PinnedVersion, Assets: []asset{{
		Name: name, URL: officialRepo + "/releases/download/v" + PinnedVersion + "/" + name,
		Size: int64(len(f.archive)), Digest: "sha256:" + hex.EncodeToString(f.sum[:]),
	}}}
	return f
}

func (f *fixture) deps() dependencies {
	return dependencies{
		pin: func(platform string) ([sha256.Size]byte, bool) { return f.sum, platform == f.platform },
		transport: roundTripper(func(req *http.Request) (*http.Response, error) {
			var data []byte
			if req.URL.String() == releaseAPI+"v"+PinnedVersion {
				f.metadataCalls.Add(1)
				data, _ = json.Marshal(f.release)
			} else if len(f.release.Assets) > 0 && req.URL.String() == f.release.Assets[0].URL {
				f.downloadCalls.Add(1)
				data = f.archive
			} else {
				return nil, errors.New("unexpected request in offline fixture")
			}
			return testResponse(req, data), nil
		}),
	}
}

func assertEntries(t *testing.T, directory string, want ...string) {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, entry := range entries {
		got = append(got, entry.Name())
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected entries: %v; want %v", got, want)
	}
}

func assertAbsent(t *testing.T, dir string) {
	t.Helper()
	if _, err := os.Lstat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed install created destination: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(dir))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("failed install left staging residue: %v", entries)
	}
}

func TestSupportedAndBuiltInPins(t *testing.T) {
	want := map[string]string{
		"linux-amd64":   "e4bd9119f50ae7c41658b1abd0679ae06f98bd864a2907ff33f2f2cf7081b214",
		"linux-arm64":   "1811c8f1da0c3b6dc707ebaf1c8390e987ba39859a8f0a7aadd8526157f2ec8d",
		"windows-amd64": "2e04a9995a6f2ad1b28c0bb25c708ecb2695e43a1c9c7889c4b6ac2b48cf7a6a",
		"windows-arm64": "db062e8a177facacc8ae53d4c9c0df4132b13fc3a89acda6f21d92d36633077e",
	}
	if PinnedVersion != "26.10.31" || maxArchiveBytes != 128<<20 || maxFileBytes != 128<<20 || maxTotalBytes != 256<<20 {
		t.Fatal("installation policy changed without an explicit compatibility review")
	}
	wantName := map[string]string{
		"windows-amd64": "Xray-windows-64.zip", "windows-arm64": "Xray-windows-arm64-v8a.zip",
		"linux-amd64": "Xray-linux-64.zip", "linux-arm64": "Xray-linux-arm64-v8a.zip",
	}
	for _, goos := range []string{"windows", "linux"} {
		for _, arch := range []string{"amd64", "arm64"} {
			name, platform, err := assetName(goos, arch)
			if err != nil || !Supported(goos, arch) || name != wantName[platform] {
				t.Fatal("unexpected asset mapping", goos, arch, name, platform, err)
			}
			sum, ok := builtinDigest(platform)
			if !ok || hex.EncodeToString(sum[:]) != want[platform] {
				t.Fatal("missing or changed fixed pin", platform)
			}
		}
	}
	for _, pair := range [][2]string{{"", ""}, {"Windows", "amd64"}, {"linux", "AMD64"}, {"darwin", "arm64"}, {"windows", "386"}, {"linux ", "amd64"}} {
		if Supported(pair[0], pair[1]) {
			t.Fatal("unexpectedly supported target", pair)
		}
		if _, _, err := assetName(pair[0], pair[1]); err == nil {
			t.Fatal("unsupported asset accepted", pair)
		}
	}
	if _, ok := builtinDigest("other"); ok {
		t.Fatal("unknown platform has a pin")
	}
}

func TestInstallAllTargetsReportsProgressAndPublishesDirectory(t *testing.T) {
	for _, goos := range []string{"windows", "linux"} {
		for _, arch := range []string{"amd64", "arm64"} {
			t.Run(goos+"-"+arch, func(t *testing.T) {
				f := newFixture(t, goos, arch)
				dir := filepath.Join(t.TempDir(), "xray-"+PinnedVersion)
				var events []Progress
				got, err := install(context.Background(), Options{OS: goos, Arch: arch, Dir: dir}, func(p Progress) { events = append(events, p) }, f.deps())
				if err != nil || !filepath.IsAbs(got.Dir) || got.Dir != dir {
					t.Fatalf("install failed: %+v %v", got, err)
				}
				wantFiles := []string{"LICENSE", "geoip.dat", "geosite.dat", "xray"}
				if goos == "windows" {
					wantFiles = []string{"LICENSE", "geoip.dat", "geosite.dat", "wintun.dll", "xray.exe"}
				}
				assertEntries(t, dir, wantFiles...)
				for _, name := range wantFiles {
					body, err := os.ReadFile(filepath.Join(dir, name))
					if err != nil || string(body) != f.files[name] {
						t.Fatalf("wrong installed bytes for %s: %v", name, err)
					}
				}
				binaryName := "xray"
				if goos == "windows" {
					binaryName = "xray.exe"
				}
				if got.Binary != filepath.Join(dir, binaryName) {
					t.Fatalf("wrong binary path: %s", got.Binary)
				}
				if runtime.GOOS != "windows" {
					info, err := os.Stat(got.Binary)
					if err != nil || info.Mode().Perm() != 0700 {
						t.Fatalf("unsafe executable permissions: %v %v", info, err)
					}
				}
				var stages []string
				var previous int64
				for _, p := range events {
					if len(stages) == 0 || stages[len(stages)-1] != p.Stage {
						stages = append(stages, p.Stage)
					}
					if p.BytesDownloaded < previous || p.BytesDownloaded < 0 || p.BytesDownloaded > p.TotalBytes {
						t.Fatalf("unreal progress: %+v", p)
					}
					if p.Stage != "metadata" && p.TotalBytes != int64(len(f.archive)) {
						t.Fatal(p)
					}
					previous = p.BytesDownloaded
				}
				if !reflect.DeepEqual(stages, []string{"metadata", "downloading", "verifying", "extracting", "installing"}) || previous != int64(len(f.archive)) {
					t.Fatalf("wrong stage sequence/bytes: %v %d", stages, previous)
				}
				if f.metadataCalls.Load() != 1 || f.downloadCalls.Load() != 1 {
					t.Fatal("unexpected request counts")
				}
				entries, err := os.ReadDir(filepath.Dir(dir))
				if err != nil || len(entries) != 1 || entries[0].Name() != filepath.Base(dir) {
					t.Fatalf("staging residue: %v %v", entries, err)
				}
			})
		}
	}
}

func TestInstallRejectsExistingDestinationWithoutNetwork(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "xray")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	f := newFixture(t, "linux", "amd64")
	_, err := install(context.Background(), Options{OS: "linux", Arch: "amd64", Dir: dir}, nil, f.deps())
	if !errors.Is(err, os.ErrExist) || f.metadataCalls.Load() != 0 || f.downloadCalls.Load() != 0 {
		t.Fatalf("existing destination accepted: %v", err)
	}
}

func TestInstallVerifiesBeforeExtraction(t *testing.T) {
	f := newFixture(t, "linux", "amd64")
	f.archive = bytes.Repeat([]byte("!"), len(f.archive))
	dir := filepath.Join(t.TempDir(), "xray")
	var stages []string
	_, err := install(context.Background(), Options{OS: "linux", Arch: "amd64", Dir: dir}, func(p Progress) { stages = append(stages, p.Stage) }, f.deps())
	if !errors.Is(err, errChecksum) {
		t.Fatalf("expected checksum failure: %v", err)
	}
	for _, stage := range stages {
		if stage == "extracting" || stage == "installing" {
			t.Fatal("unverified bytes used", stages)
		}
	}
	assertAbsent(t, dir)
}

func TestInstallRejectsUnsafeOrIncompleteArchive(t *testing.T) {
	cases := []struct {
		name    string
		entries []zipEntry
	}{
		{name: "traversal", entries: []zipEntry{{name: "../xray", body: "evil"}, {name: "geoip.dat", body: "geo"}, {name: "geosite.dat", body: "geosite"}}},
		{name: "backslash", entries: []zipEntry{{name: "sub\\xray", body: "evil"}, {name: "geoip.dat", body: "geo"}, {name: "geosite.dat", body: "geosite"}}},
		{name: "missing binary", entries: []zipEntry{{name: "geoip.dat", body: "geo"}, {name: "geosite.dat", body: "geosite"}}},
		{name: "missing wintun", entries: []zipEntry{{name: "xray.exe", body: "binary"}, {name: "geoip.dat", body: "geo"}, {name: "geosite.dat", body: "geosite"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			goos := "linux"
			if tc.name == "missing wintun" {
				goos = "windows"
			}
			f := newFixture(t, goos, "amd64")
			f.archive = zipBytes(t, tc.entries...)
			f.release.Assets[0].Size = int64(len(f.archive))
			f.sum = sha256.Sum256(f.archive)
			f.release.Assets[0].Digest = "sha256:" + hex.EncodeToString(f.sum[:])
			dir := filepath.Join(t.TempDir(), "xray")
			_, err := install(context.Background(), Options{OS: goos, Arch: "amd64", Dir: dir}, nil, f.deps())
			if !errors.Is(err, errArchive) {
				t.Fatalf("unsafe archive accepted: %v", err)
			}
			assertAbsent(t, dir)
		})
	}
}

func TestInstallPublicationFailureLeavesNoDestination(t *testing.T) {
	f := newFixture(t, "linux", "amd64")
	dir := filepath.Join(t.TempDir(), "xray")
	deps := f.deps()
	deps.link = func(string, string) error { return os.ErrExist }
	_, err := install(context.Background(), Options{OS: "linux", Arch: "amd64", Dir: dir}, nil, deps)
	if !errors.Is(err, os.ErrExist) {
		t.Fatalf("expected publication failure: %v", err)
	}
	assertAbsent(t, dir)
}

func TestInstallRejectsInvalidOptions(t *testing.T) {
	for _, dir := range []string{"", " ", "\t\n", "bad\x00dir"} {
		_, err := Install(context.Background(), Options{OS: "linux", Arch: "amd64", Dir: dir}, nil)
		if err == nil || !strings.Contains(err.Error(), "目录") {
			t.Fatalf("bad dir accepted: %q %v", dir, err)
		}
	}
	if _, err := Install(nil, Options{Dir: "irrelevant"}, nil); err == nil {
		t.Fatal("nil context accepted")
	}
	for _, pair := range [][2]string{{"darwin", "arm64"}, {"linux", "386"}} {
		if _, err := Install(context.Background(), Options{OS: pair[0], Arch: pair[1], Dir: "irrelevant"}, nil); err == nil {
			t.Fatal("unsupported target accepted")
		}
	}
}
