package app

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/uniproxy/uniproxy/internal/model"
	"github.com/uniproxy/uniproxy/internal/sidecarinstall"
	"github.com/uniproxy/uniproxy/internal/store"
)

type installableFakeSidecar struct {
	*fakeSidecar
	installMu      sync.Mutex
	installation   model.CoreInstallation
	prepareErr     error
	runErr         error
	prepareCalls   int
	runCalls       int
	releaseInstall chan struct{}
}

func (f *installableFakeSidecar) InstallationStatus() model.CoreInstallation {
	f.installMu.Lock()
	defer f.installMu.Unlock()
	s := f.installation
	if s.State == "" {
		s.State = "idle"
	}
	if s.Version == "" {
		s.Version = sidecarinstall.PinnedVersion
	}
	s.OS, s.Arch = runtime.GOOS, runtime.GOARCH
	return s
}

func (f *installableFakeSidecar) PrepareInstall(context.Context) (model.CoreInstallation, error) {
	f.installMu.Lock()
	defer f.installMu.Unlock()
	f.prepareCalls++
	if f.prepareErr != nil {
		return f.installation, f.prepareErr
	}
	f.installation.State, f.installation.Stage = "installing", "metadata"
	return f.installation, nil
}

func (f *installableFakeSidecar) RunInstall(ctx context.Context) error {
	f.installMu.Lock()
	f.runCalls++
	release := f.releaseInstall
	f.installMu.Unlock()
	if release != nil {
		select {
		case <-release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	f.installMu.Lock()
	defer f.installMu.Unlock()
	if f.runErr != nil {
		f.installation.State = "failed"
		f.installation.Error = f.runErr.Error()
		return f.runErr
	}
	f.installation.State, f.installation.Stage = "succeeded", "complete"
	return nil
}

func newInstallableSidecarFixture(t *testing.T) (*App, *fakeCore, *installableFakeSidecar) {
	t.Helper()
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	core := &fakeCore{status: model.RuntimeStatus{Available: true}}
	fake := &installableFakeSidecar{fakeSidecar: &fakeSidecar{status: model.SidecarStatus{Engine: model.EngineXray, Version: sidecarinstall.PinnedVersion}, core: core}}
	a := New(s, core, Options{AdminPort: 9090, Sidecar: fake})
	t.Cleanup(func() { a.Close(); s.Close() })
	return a, core, fake
}

func TestSidecarInfoIncludesInstallation(t *testing.T) {
	a, _, _ := newInstallableSidecarFixture(t)
	code, out := request(t, a, "GET", "/api/sidecars", nil)
	if code != 200 {
		t.Fatalf("status = %d %#v", code, out)
	}
	installation, ok := out["installation"].(map[string]any)
	if !ok || installation["state"] != "idle" || installation["version"] != sidecarinstall.PinnedVersion {
		t.Fatalf("missing installation status: %#v", out)
	}
}

func TestSidecarInstallEndpointBlocksApplyUntilVerified(t *testing.T) {
	a, _, fake := newInstallableSidecarFixture(t)
	release := make(chan struct{})
	fake.installMu.Lock()
	fake.releaseInstall = release
	fake.installMu.Unlock()

	code, out := request(t, a, "POST", "/api/sidecars/install", map[string]any{})
	if code != 202 {
		t.Fatalf("install status = %d %#v", code, out)
	}
	if out["state"] != "installing" {
		t.Fatalf("install response = %#v", out)
	}
	fake.installMu.Lock()
	if fake.prepareCalls != 1 {
		fake.installMu.Unlock()
		t.Fatalf("prepare calls = %d", fake.prepareCalls)
	}
	fake.installMu.Unlock()

	code, body := request(t, a, "POST", "/api/runtime/apply", map[string]any{})
	if code != 409 {
		t.Fatalf("apply during install = %d %#v", code, body)
	}
	code, body = request(t, a, "GET", "/api/sidecars", nil)
	installation := body["installation"].(map[string]any)
	if code != 200 || installation["state"] != "installing" {
		t.Fatalf("status during install = %d %#v", code, body)
	}
	close(release)
	deadline := time.Now().Add(3 * time.Second)
	for {
		code, body = request(t, a, "GET", "/api/sidecars", nil)
		installation = body["installation"].(map[string]any)
		if code == 200 && installation["state"] == "succeeded" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("install did not finish: %#v", body)
		}
		time.Sleep(10 * time.Millisecond)
	}
	fake.installMu.Lock()
	defer fake.installMu.Unlock()
	if fake.runCalls != 1 {
		t.Fatalf("run calls = %d", fake.runCalls)
	}
}

func TestSidecarInstallFailureIsReported(t *testing.T) {
	a, _, fake := newInstallableSidecarFixture(t)
	fake.installMu.Lock()
	fake.runErr = errors.New("下载失败")
	fake.installMu.Unlock()
	code, _ := request(t, a, "POST", "/api/sidecars/install", map[string]any{})
	if code != 202 {
		t.Fatalf("install status = %d", code)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		a.mu.Lock()
		pending := a.sidecarInstallPending
		message := a.applyError
		a.mu.Unlock()
		if !pending {
			if !strings.Contains(message, "Xray 安装失败") {
				t.Fatalf("failure not surfaced: %q", message)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("install did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
	a.mu.Lock()
	awaiting := a.awaitingCoreApply
	a.mu.Unlock()
	if awaiting {
		t.Fatal("failed install left awaitingCoreApply set")
	}
	code, body := request(t, a, "POST", "/api/runtime/apply", map[string]any{})
	if code != 200 {
		t.Fatalf("apply after failed install = %d %#v", code, body)
	}
}
