package coreinstall

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOverlongDownloadNeverReportsMoreThanTotal(t *testing.T) {
	f := newFixture(t, "linux", "amd64")
	f.onResponse = func(req *http.Request, response *http.Response) {
		if req.URL.String() == f.release.Assets[0].URL {
			response.ContentLength = -1
			response.Body = io.NopCloser(io.MultiReader(bytes.NewReader(f.archive), strings.NewReader("oversize")))
		}
	}
	output := filepath.Join(t.TempDir(), "mihomo")
	_, err := install(context.Background(), Options{OS: "linux", Arch: "amd64", Output: output}, func(p Progress) {
		if p.BytesDownloaded > p.TotalBytes {
			t.Errorf("unbounded progress: %+v", p)
		}
		if p.Stage == "verifying" || p.Stage == "extracting" || p.Stage == "installing" {
			t.Errorf("oversize body reached %s", p.Stage)
		}
	}, f.deps())
	if !errors.Is(err, errDownloadSize) {
		t.Fatal("wrong oversize error", err)
	}
	assertAbsent(t, output)
}

type cancelReaderAt struct {
	reader io.ReaderAt
	cancel context.CancelFunc
	calls  int
}

func (r *cancelReaderAt) ReadAt(p []byte, offset int64) (int, error) {
	r.calls++
	n, err := r.reader.ReadAt(p, offset)
	r.cancel()
	return n, err
}

func TestZIPDirectoryReadsHonorCancellation(t *testing.T) {
	data := zipBytes(t, zipEntry{name: "mihomo.exe", body: "fixture"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader := &cancelReaderAt{reader: bytes.NewReader(data), cancel: cancel}
	err := extractZIP(ctx, reader, int64(len(data)), io.Discard, maxBinaryBytes)
	if !errors.Is(err, context.Canceled) || reader.calls != 1 {
		t.Fatalf("ZIP directory reads ignored context: %d %v", reader.calls, err)
	}
	guarded := &contextReaderAt{ctx: ctx, reader: reader}
	if n, err := guarded.ReadAt(make([]byte, 1), 0); n != 0 || !errors.Is(err, context.Canceled) || reader.calls != 1 {
		t.Fatal("canceled ReaderAt reached disk", n, err)
	}
}

type cancelReader struct {
	reader io.Reader
	cancel context.CancelFunc
}

func (r *cancelReader) Read(p []byte) (int, error) {
	if len(p) > 8 {
		p = p[:8]
	}
	n, err := r.reader.Read(p)
	r.cancel()
	return n, err
}

func TestNetworkCancellationInTransportAndBody(t *testing.T) {
	for _, stage := range []string{"transport", "body"} {
		t.Run(stage, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f := newFixture(t, "linux", "amd64")
			deps := f.deps()
			var body *trackedBody
			if stage == "transport" {
				deps.transport = roundTripper(func(req *http.Request) (*http.Response, error) {
					cancel()
					<-req.Context().Done()
					return nil, &url.Error{Op: "Get", URL: "https://github.com/?SECRET", Err: req.Context().Err()}
				})
			} else {
				f.onResponse = func(req *http.Request, response *http.Response) {
					if req.URL.String() != f.release.Assets[0].URL {
						return
					}
					body = &trackedBody{reader: &cancelReader{reader: bytes.NewReader(f.archive), cancel: cancel}}
					response.Body = body
				}
			}
			output := filepath.Join(t.TempDir(), "mihomo")
			_, err := install(ctx, Options{OS: "linux", Arch: "amd64", Output: output}, nil, deps)
			if !errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "SECRET") {
				t.Fatal("network cancellation lost/leaked", err)
			}
			if stage == "body" && (body == nil || !body.closed) {
				t.Fatal("canceled response body not closed")
			}
			assertAbsent(t, output)
		})
	}
}

func TestSuccessfulPublicationIsTheCommitPoint(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := newFixture(t, "linux", "amd64")
	deps := f.deps()
	deps.link = func(staged, destination string) error {
		err := os.Link(staged, destination)
		cancel() // Cancellation racing AFTER commit must not delete the binary.
		return err
	}
	output := filepath.Join(t.TempDir(), "mihomo")
	got, err := install(ctx, Options{OS: "linux", Arch: "amd64", Output: output}, nil, deps)
	if err != nil || got != output {
		t.Fatal("successful commit reported as canceled", err)
	}
	body, err := os.ReadFile(got)
	if err != nil || !bytes.Equal(body, f.binary) {
		t.Fatal("commit undone", err)
	}
	assertFiles(t, filepath.Dir(output), "mihomo")
}
