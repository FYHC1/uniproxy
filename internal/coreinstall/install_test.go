package coreinstall

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestSupportedAndBuiltInPins(t *testing.T) {
	want := map[string]string{
		"linux-amd64-compatible":   "ba3ce607747a07f948fc35780e108a4a7c7f552a38b9bd4d115f313ebcb89c20",
		"linux-arm64":              "9dd862e28b46ff7d775f169cceebc28deccaa0a9e804237d421cd2571e0caba0",
		"windows-amd64-compatible": "974a4d7ad69aed27aa2e8f91d61113573c14dadb14562c63e58effabf59816f0",
		"windows-arm64":            "7a4f6e58af9a120920935f2150cf642d945085fdb3e753314505b334234e1d1e",
	}
	if PinnedVersion != "1.19.32" || maxArchiveBytes != 128<<20 || maxBinaryBytes != 256<<20 || installTimeout != 5*time.Minute {
		t.Fatal("installation policy changed without an explicit compatibility review")
	}
	for _, goos := range []string{"windows", "linux"} {
		for _, arch := range []string{"amd64", "arm64"} {
			name, platform, err := assetName(goos, arch)
			if err != nil || !Supported(goos, arch) {
				t.Fatal("supported platform rejected", goos, arch, err)
			}
			suffix := ".gz"
			if goos == "windows" {
				suffix = ".zip"
			}
			if name != "mihomo-"+platform+"-v1.19.32"+suffix {
				t.Fatal("unexpected asset name", name)
			}
			sum, ok := builtinDigest(platform)
			if !ok || hex.EncodeToString(sum[:]) != want[platform] {
				t.Fatal("missing or changed fixed pin", platform)
			}
		}
	}
	for _, pair := range [][2]string{{"", ""}, {"Windows", "amd64"}, {"linux", "AMD64"}, {"darwin", "arm64"}, {"windows", "386"}, {"linux", "arm"}, {"linux ", "amd64"}} {
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

func TestInstallAllTargetsReportsRealProgressAndCleansStaging(t *testing.T) {
	for _, goos := range []string{"windows", "linux"} {
		for _, arch := range []string{"amd64", "arm64"} {
			t.Run(goos+"-"+arch, func(t *testing.T) {
				f := newFixture(t, goos, arch)
				output := filepath.Join(t.TempDir(), "mihomo")
				var events []Progress
				got, err := install(context.Background(), Options{OS: goos, Arch: arch, Output: output}, func(p Progress) { events = append(events, p) }, f.deps())
				if err != nil || !filepath.IsAbs(got) || got != output {
					t.Fatalf("install failed: %q %v", got, err)
				}
				body, err := os.ReadFile(got)
				if err != nil || !bytes.Equal(body, f.binary) {
					t.Fatal("wrong installed bytes", err)
				}
				info, err := os.Stat(got)
				if err != nil || !info.Mode().IsRegular() {
					t.Fatal("not a regular file", err)
				}
				if runtime.GOOS != "windows" && info.Mode().Perm() != 0700 {
					t.Fatal("unsafe executable permissions", info.Mode())
				}
				assertFiles(t, filepath.Dir(output), "mihomo")
				var stages []string
				var previous int64
				for _, p := range events {
					if len(stages) == 0 || stages[len(stages)-1] != p.Stage {
						stages = append(stages, p.Stage)
					}
					if p.BytesDownloaded < previous || p.BytesDownloaded < 0 || p.BytesDownloaded > p.TotalBytes {
						t.Fatalf("unreal progress: %+v", p)
					}
					if p.Stage == "metadata" {
						if p.TotalBytes != 0 || p.BytesDownloaded != 0 {
							t.Fatal(p)
						}
					} else if p.TotalBytes != int64(len(f.archive)) {
						t.Fatal(p)
					}
					previous = p.BytesDownloaded
				}
				if !reflect.DeepEqual(stages, []string{"metadata", "downloading", "verifying", "extracting", "installing"}) || previous != int64(len(f.archive)) {
					t.Fatalf("wrong stage sequence/bytes: %v %d", stages, previous)
				}
				if f.metadataCalls.Load() != 1 || f.downloadCalls.Load() != 1 {
					t.Fatal("unexpected requests")
				}
			})
		}
	}
}

func TestInstallDefaultsAndAbsoluteOutput(t *testing.T) {
	t.Chdir(t.TempDir())
	if !Supported(runtime.GOOS, runtime.GOARCH) {
		_, err := Install(context.Background(), Options{Output: "mihomo"}, nil)
		if err == nil {
			t.Fatal("unsupported host installed")
		}
		return
	}
	f := newFixture(t, runtime.GOOS, runtime.GOARCH)
	want, err := filepath.Abs("nested/mihomo")
	if err != nil {
		t.Fatal(err)
	}
	got, err := install(context.Background(), Options{Output: "nested/mihomo"}, nil, f.deps())
	if err != nil || got != want {
		t.Fatalf("runtime defaults / absolute output: %q %v", got, err)
	}
	assertFiles(t, filepath.Dir(want), "mihomo")
}

func TestInstallRejectsInvalidOptionsWithoutNetwork(t *testing.T) {
	for _, output := range []string{"", " ", "\t\n", "bad\x00path"} {
		_, err := Install(context.Background(), Options{OS: "linux", Arch: "amd64", Output: output}, nil)
		if err == nil || !strings.Contains(err.Error(), "路径") {
			t.Fatalf("bad output accepted: %q %v", output, err)
		}
	}
	if _, err := Install(nil, Options{Output: "irrelevant"}, nil); err == nil {
		t.Fatal("nil context accepted")
	}
	for _, pair := range [][2]string{{"darwin", "arm64"}, {"linux", "386"}} {
		if _, err := Install(context.Background(), Options{OS: pair[0], Arch: pair[1], Output: "irrelevant"}, nil); err == nil {
			t.Fatal("unsupported target accepted")
		}
	}
}

func TestInstallNeverOverridesBuiltInPin(t *testing.T) {
	f := newFixture(t, "linux", "amd64")
	deps := f.deps()
	deps.pin = nil // production policy, despite matching synthetic metadata/body
	output := filepath.Join(t.TempDir(), "mihomo")
	_, err := install(context.Background(), Options{OS: "linux", Arch: "amd64", Output: output}, nil, deps)
	if !errors.Is(err, errMetadataPin) || f.downloadCalls.Load() != 0 {
		t.Fatalf("untrusted pin accepted: %v", err)
	}
	assertAbsent(t, output)
}

func TestInstallVerifiesBeforeExtraction(t *testing.T) {
	f := newFixture(t, "linux", "amd64")
	f.archive = bytes.Repeat([]byte("!"), len(f.archive)) // same size, invalid archive and checksum
	output := filepath.Join(t.TempDir(), "mihomo")
	var stages []string
	_, err := install(context.Background(), Options{OS: "linux", Arch: "amd64", Output: output}, func(p Progress) { stages = append(stages, p.Stage) }, f.deps())
	if !errors.Is(err, errChecksum) {
		t.Fatalf("expected checksum failure before parsing: %v", err)
	}
	for _, stage := range stages {
		if stage == "extracting" || stage == "installing" {
			t.Fatal("unverified bytes used", stages)
		}
	}
	assertAbsent(t, output)
}

func TestInstallBadVerifiedArchiveIsCleaned(t *testing.T) {
	f := newFixture(t, "linux", "amd64")
	f.archive = []byte("not a gzip archive")
	f.sum = sha256.Sum256(f.archive)
	f.release.Assets[0].Digest = "sha256:" + hex.EncodeToString(f.sum[:])
	f.release.Assets[0].Size = int64(len(f.archive))
	output := filepath.Join(t.TempDir(), "mihomo")
	_, err := install(context.Background(), Options{OS: "linux", Arch: "amd64", Output: output}, nil, f.deps())
	if !errors.Is(err, errArchive) {
		t.Fatal("invalid archive accepted", err)
	}
	assertAbsent(t, output) // Windows also proves handles closed before removal.
}

func TestInstallRejectsExistingFilesDirectoriesAndSymlinks(t *testing.T) {
	for _, kind := range []string{"file", "directory", "symlink", "dangling-symlink"} {
		t.Run(kind, func(t *testing.T) {
			directory := t.TempDir()
			output := filepath.Join(directory, "mihomo")
			wantFiles := []string{"mihomo"}
			switch kind {
			case "file":
				if err := os.WriteFile(output, []byte("existing"), 0600); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(output, 0700); err != nil {
					t.Fatal(err)
				}
			default:
				target := filepath.Join(directory, "target")
				if kind == "symlink" {
					if err := os.WriteFile(target, []byte("existing"), 0600); err != nil {
						t.Fatal(err)
					}
					wantFiles = append(wantFiles, "target")
				}
				if err := os.Symlink(target, output); err != nil {
					t.Skipf("host cannot create test symlink: %v", err)
				}
			}
			f := newFixture(t, "linux", "amd64")
			_, err := install(context.Background(), Options{OS: "linux", Arch: "amd64", Output: output}, nil, f.deps())
			if !errors.Is(err, os.ErrExist) || f.metadataCalls.Load() != 0 {
				t.Fatalf("existing path accepted: %v", err)
			}
			assertFiles(t, directory, wantFiles...)
			if kind == "file" || kind == "symlink" {
				body, err := os.ReadFile(output)
				if err != nil || string(body) != "existing" {
					t.Fatal("existing binary changed", err)
				}
			}
		})
	}
}

func TestInstallDoesNotReplaceDestinationCreatedDuringDownload(t *testing.T) {
	f := newFixture(t, "linux", "amd64")
	output := filepath.Join(t.TempDir(), "mihomo")
	f.onResponse = func(req *http.Request, _ *http.Response) {
		if req.URL.String() == f.release.Assets[0].URL {
			if err := os.WriteFile(output, []byte("competitor"), 0700); err != nil {
				t.Fatal(err)
			}
		}
	}
	_, err := install(context.Background(), Options{OS: "linux", Arch: "amd64", Output: output}, nil, f.deps())
	if !errors.Is(err, os.ErrExist) {
		t.Fatal("racing destination not rejected", err)
	}
	body, err := os.ReadFile(output)
	if err != nil || string(body) != "competitor" {
		t.Fatal("competing file overwritten", err)
	}
	assertFiles(t, filepath.Dir(output), "mihomo")
}

func TestInstallNoReplaceAtPublicationAndClosedHandles(t *testing.T) {
	f := newFixture(t, "linux", "amd64")
	output := filepath.Join(t.TempDir(), "mihomo")
	deps := f.deps()
	called := false
	deps.link = func(staging, dest string) error {
		called = true
		if filepath.Dir(staging) != filepath.Dir(dest) {
			return errors.New("not same-directory staging")
		}
		// Rename is only a test of closed staging handles on Windows. Production
		// publication uses os.Link, never os.Rename.
		if err := os.Rename(staging, staging+".closed"); err != nil {
			return err
		}
		if err := os.Rename(staging+".closed", staging); err != nil {
			return err
		}
		entries, err := os.ReadDir(filepath.Dir(dest))
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".mihomo-archive-") {
				if err := os.Remove(filepath.Join(filepath.Dir(dest), entry.Name())); err != nil {
					return err
				}
			}
		}
		if err := os.WriteFile(dest, []byte("created at commit"), 0700); err != nil {
			return err
		}
		return os.Link(staging, dest)
	}
	_, err := install(context.Background(), Options{OS: "linux", Arch: "amd64", Output: output}, nil, deps)
	if !called || !errors.Is(err, os.ErrExist) {
		t.Fatalf("no-replace commit / handles incorrect: %v", err)
	}
	body, err := os.ReadFile(output)
	if err != nil || string(body) != "created at commit" {
		t.Fatal("publication overwrote competitor", err)
	}
	assertFiles(t, filepath.Dir(output), "mihomo")
}

func TestConcurrentInstallersOnlyOnePublishes(t *testing.T) {
	output := filepath.Join(t.TempDir(), "mihomo")
	f := newFixture(t, "linux", "amd64")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ready := make(chan struct{}, 2)
	commit := make(chan struct{})
	type result struct {
		path string
		err  error
	}
	results := make(chan result, 2)
	for i := 0; i < 2; i++ {
		go func() {
			path, err := install(ctx, Options{OS: "linux", Arch: "amd64", Output: output}, func(p Progress) {
				if p.Stage == "installing" {
					ready <- struct{}{}
					select {
					case <-commit:
					case <-ctx.Done():
					}
				}
			}, f.deps())
			results <- result{path, err}
		}()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-ready:
		case got := <-results:
			cancel()
			<-results
			t.Fatalf("installer failed before commit barrier: %v", got.err)
		case <-ctx.Done():
			<-results
			<-results
			t.Fatal("installer never reached commit barrier")
		}
	}
	close(commit)
	var successes, conflicts int
	for i := 0; i < 2; i++ {
		got := <-results
		if got.err == nil && got.path == output {
			successes++
		} else if errors.Is(got.err, os.ErrExist) {
			conflicts++
		} else {
			t.Errorf("unexpected install result: %q %v", got.path, got.err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("publication not exclusive: %d success, %d conflict", successes, conflicts)
	}
	body, err := os.ReadFile(output)
	if err != nil || !bytes.Equal(body, f.binary) {
		t.Fatal("incomplete published binary", err)
	}
	assertFiles(t, filepath.Dir(output), "mihomo")
}

func TestInstallFailsClosedWhenHardLinksUnsupported(t *testing.T) {
	for _, raw := range []error{errors.New("unsupported filesystem SECRET-PATH"), &os.LinkError{Op: "link", Old: "SECRET-OLD", New: "SECRET-NEW", Err: os.ErrPermission}} {
		f := newFixture(t, "linux", "amd64")
		output := filepath.Join(t.TempDir(), "mihomo")
		deps := f.deps()
		deps.link = func(string, string) error { return raw }
		_, err := install(context.Background(), Options{OS: "linux", Arch: "amd64", Output: output}, nil, deps)
		if err == nil || strings.Contains(err.Error(), "SECRET") {
			t.Fatal("unsafe publication fallback/error", err)
		}
		if !strings.Contains(err.Error(), "权限") {
			t.Fatal("not actionable", err)
		}
		assertAbsent(t, output)
	}
}

func TestInstallCancellationAtEveryStage(t *testing.T) {
	for _, target := range []string{"metadata", "downloading", "download-bytes", "verifying", "extracting", "installing"} {
		t.Run(target, func(t *testing.T) {
			f := newFixture(t, "linux", "amd64")
			output := filepath.Join(t.TempDir(), "mihomo")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			_, err := install(ctx, Options{OS: "linux", Arch: "amd64", Output: output}, func(p Progress) {
				if p.Stage == target || (target == "download-bytes" && p.Stage == "downloading" && p.BytesDownloaded > 0) {
					cancel()
				}
			}, f.deps())
			if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "取消") {
				t.Fatal("cancellation not honored", err)
			}
			assertAbsent(t, output)
		})
	}
}

func TestInstallCanceledAndExpiredContextsNeverRequest(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	expired, stop := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer stop()
	for _, tc := range []struct {
		ctx  context.Context
		want error
	}{{canceled, context.Canceled}, {expired, context.DeadlineExceeded}} {
		f := newFixture(t, "linux", "amd64")
		output := filepath.Join(t.TempDir(), "mihomo")
		_, err := install(tc.ctx, Options{OS: "linux", Arch: "amd64", Output: output}, nil, f.deps())
		if !errors.Is(err, tc.want) || f.metadataCalls.Load() != 0 {
			t.Fatal("context ignored", err)
		}
		assertAbsent(t, output)
	}
}

func TestInstallRequestTimeoutAndEarlierDeadline(t *testing.T) {
	for _, duration := range []time.Duration{time.Hour, time.Minute} {
		f := newFixture(t, "linux", "amd64")
		ctx, cancel := context.WithTimeout(context.Background(), duration)
		output := filepath.Join(t.TempDir(), "mihomo")
		f.onResponse = func(req *http.Request, _ *http.Response) {
			deadline, ok := req.Context().Deadline()
			remaining := time.Until(deadline)
			if !ok || remaining <= 0 || remaining > downloadTimeout || remaining > duration {
				t.Errorf("request missing bounded deadline: %v", remaining)
			}
		}
		_, err := install(ctx, Options{OS: "linux", Arch: "amd64", Output: output}, nil, f.deps())
		cancel()
		if err != nil {
			t.Fatal(err)
		}
	}
}

type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

func TestInstallDownloadFailuresCloseBodiesAndLeaveNoFiles(t *testing.T) {
	for _, failure := range []string{"http", "content-length", "short", "long", "encoding", "read", "close"} {
		t.Run(failure, func(t *testing.T) {
			f := newFixture(t, "linux", "amd64")
			output := filepath.Join(t.TempDir(), "mihomo")
			var body *trackedBody
			f.onResponse = func(req *http.Request, response *http.Response) {
				if req.URL.String() != f.release.Assets[0].URL {
					return
				}
				body = &trackedBody{reader: bytes.NewReader(f.archive)}
				response.Body = body
				switch failure {
				case "http":
					response.StatusCode = 403
					body.reader = strings.NewReader("SECRET response body")
				case "content-length":
					response.ContentLength++
				case "short":
					response.ContentLength = -1
					body.reader = bytes.NewReader(f.archive[:len(f.archive)-1])
				case "long":
					response.ContentLength = -1
					body.reader = io.MultiReader(bytes.NewReader(f.archive), strings.NewReader("extra"))
				case "encoding":
					response.Header.Set("Content-Encoding", "gzip")
				case "read":
					body.reader = failingReader{errors.New("SECRET signed URL read error")}
				case "close":
					body.closeErr = errors.New("SECRET close error")
				}
			}
			_, err := install(context.Background(), Options{OS: "linux", Arch: "amd64", Output: output}, nil, f.deps())
			if err == nil || strings.Contains(fmt.Sprintf("%+v", err), "SECRET") {
				t.Fatal("download failure unsafely handled", err)
			}
			if body == nil || !body.closed {
				t.Fatal("download body not closed")
			}
			assertAbsent(t, output)
		})
	}
}

func TestInstallInvalidDirectoryIsSanitized(t *testing.T) {
	directory := t.TempDir()
	file := filepath.Join(directory, "SECRET-not-a-directory")
	if err := os.WriteFile(file, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	f := newFixture(t, "linux", "amd64")
	_, err := install(context.Background(), Options{OS: "linux", Arch: "amd64", Output: filepath.Join(file, "mihomo")}, nil, f.deps())
	if err == nil || strings.Contains(err.Error(), "SECRET") || !strings.Contains(err.Error(), "权限") {
		t.Fatal("unsafe filesystem error", err)
	}
	assertFiles(t, directory, "SECRET-not-a-directory")
}
