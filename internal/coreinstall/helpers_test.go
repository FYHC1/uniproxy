package coreinstall

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type trackedBody struct {
	reader   io.Reader
	closed   bool
	closeErr error
}

func (b *trackedBody) Read(p []byte) (int, error) { return b.reader.Read(p) }
func (b *trackedBody) Close() error               { b.closed = true; return b.closeErr }

func testResponse(req *http.Request, data []byte) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK, Header: make(http.Header),
		Body: io.NopCloser(bytes.NewReader(data)), ContentLength: int64(len(data)), Request: req,
	}
}

func gzipBytes(t *testing.T, body []byte, name string) []byte {
	t.Helper()
	var data bytes.Buffer
	writer := gzip.NewWriter(&data)
	writer.Name = name
	if _, err := writer.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
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

type installFixture struct {
	binary        []byte
	archive       []byte
	sum           [sha256.Size]byte
	platform      string
	release       release
	metadataCalls atomic.Int64
	downloadCalls atomic.Int64
	onResponse    func(*http.Request, *http.Response)
}

func newFixture(t *testing.T, goos, arch string) *installFixture {
	t.Helper()
	name, platform, err := assetName(goos, arch)
	if err != nil {
		t.Fatal(err)
	}
	f := &installFixture{binary: []byte("synthetic verified fixture; never execute"), platform: platform}
	if goos == "windows" {
		f.archive = zipBytes(t, zipEntry{name: "mihomo.exe", body: string(f.binary)})
	} else {
		f.archive = gzipBytes(t, f.binary, "mihomo")
	}
	f.sum = sha256.Sum256(f.archive)
	f.release = release{TagName: "v" + PinnedVersion, Assets: []asset{{
		Name: name, URL: officialRepo + "/releases/download/v" + PinnedVersion + "/" + name,
		Size: int64(len(f.archive)), Digest: "sha256:" + hex.EncodeToString(f.sum[:]),
	}}}
	return f
}

func (f *installFixture) deps() dependencies {
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
			response := testResponse(req, data)
			if f.onResponse != nil {
				f.onResponse(req, response)
			}
			return response, nil
		}),
	}
}

func assertFiles(t *testing.T, directory string, want ...string) {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	expected := make(map[string]bool, len(want))
	for _, name := range want {
		expected[name] = true
	}
	if len(entries) != len(want) {
		t.Fatalf("unexpected files in fixture directory: %v; want %v", entries, want)
	}
	for _, entry := range entries {
		if !expected[entry.Name()] {
			t.Fatalf("unexpected staging/file residue: %q", entry.Name())
		}
	}
}

func assertAbsent(t *testing.T, output string) {
	t.Helper()
	if _, err := os.Lstat(output); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed install created destination: %v", err)
	}
	assertFiles(t, filepath.Dir(output))
}
