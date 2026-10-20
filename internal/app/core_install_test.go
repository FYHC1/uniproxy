package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uniproxy/uniproxy/internal/coreinstall"
	"github.com/uniproxy/uniproxy/internal/model"
	"github.com/uniproxy/uniproxy/internal/store"
)

type installingTestCore struct {
	*fakeCore
	installMu    sync.Mutex
	installation model.CoreInstallation
	started      chan struct{}
	release      chan struct{}
	failure      error
	installs     atomic.Int32
}

func (c *installingTestCore) InstallationStatus() model.CoreInstallation {
	c.installMu.Lock()
	defer c.installMu.Unlock()
	return c.installation
}
func (c *installingTestCore) PrepareInstall(ctx context.Context) (model.CoreInstallation, error) {
	c.installMu.Lock()
	defer c.installMu.Unlock()
	if ctx.Err() != nil {
		return c.installation, ctx.Err()
	}
	if !c.installation.CanInstall || c.Status().Running {
		return c.installation, errors.New("已有内核或任务，不能覆盖")
	}
	c.installation.State = "installing"
	c.installation.Stage = "metadata"
	c.installation.CanInstall = false
	c.installation.Error = ""
	c.installation.StartedAt = model.Now()
	return c.installation, nil
}
func (c *installingTestCore) RunInstall(ctx context.Context) error {
	c.installs.Add(1)
	c.installMu.Lock()
	c.installation.Stage = "downloading"
	c.installation.BytesDownloaded = 12
	c.installation.TotalBytes = 30
	started, release, failure := c.started, c.release, c.failure
	c.installMu.Unlock()
	close(started)
	select {
	case <-ctx.Done():
		failure = ctx.Err()
	case <-release:
	}
	c.installMu.Lock()
	defer c.installMu.Unlock()
	c.installation.FinishedAt = model.Now()
	if failure != nil {
		c.installation.State = "failed"
		c.installation.Error = failure.Error()
		c.installation.CanInstall = true
		return failure
	}
	c.installation.State = "succeeded"
	c.installation.Stage = "complete"
	c.installation.BytesDownloaded = 30
	c.installation.Installed = true
	c.mu.Lock()
	c.status.Available = true
	c.status.Version = coreinstall.PinnedVersion
	c.status.Dirty = true
	c.mu.Unlock()
	return nil
}
func installFixture(t *testing.T) (*App, *installingTestCore) {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	c := &installingTestCore{fakeCore: &fakeCore{status: model.RuntimeStatus{Dirty: true}}, started: make(chan struct{}), release: make(chan struct{}), installation: model.CoreInstallation{State: "idle", Version: coreinstall.PinnedVersion, OS: "windows", Arch: "amd64", Path: filepath.Join(dir, "core", "mihomo.exe"), Supported: true, CanInstall: true}}
	a := New(s, c, Options{AdminPort: 9090})
	t.Cleanup(func() { a.Close(); s.Close() })
	return a, c
}
func waitInstall(t *testing.T, a *App, want string) model.CoreInstallation {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		s := a.installationStatus()
		if s.State == want {
			return s
		}
		select {
		case <-deadline.C:
			t.Fatalf("installation stayed %+v, wanted %s", s, want)
		case <-ticker.C:
		}
	}
}

func TestOnlineCoreInstallAsyncProgressAndNoAutomaticApply(t *testing.T) {
	a, c := installFixture(t)
	before := a.store.Snapshot()
	code, out := request(t, a, "POST", "/api/core/install", map[string]any{})
	if code != 202 || out["state"] != "installing" {
		t.Fatalf("start %d %+v", code, out)
	}
	<-c.started
	code, out = request(t, a, "GET", "/api/bootstrap", nil)
	if code != 200 {
		t.Fatal(out)
	}
	s := poolTestDecode[model.CoreInstallation](t, out["coreInstallation"])
	if s.Stage != "downloading" || s.BytesDownloaded != 12 || s.TotalBytes != 30 || s.CanInstall {
		t.Fatalf("bootstrap progress: %+v", s)
	}
	code, out = request(t, a, "GET", "/api/core/installation", nil)
	if code != 200 || out["stage"] != "downloading" {
		t.Fatalf("status %d %+v", code, out)
	}
	for _, path := range []string{"/api/core/install", "/api/runtime/apply", "/api/runtime/stop", "/api/core/inspect"} {
		code, out = request(t, a, "POST", path, map[string]any{})
		if code != 409 || out["code"] != "core_install_busy" {
			t.Fatalf("unguarded %s: %d %+v", path, code, out)
		}
	}
	// Ordinary saves still work while downloading, but cannot bring up traffic.
	code, out = request(t, a, "POST", "/api/groups", map[string]any{"name": "saved during install"})
	if code != 200 {
		t.Fatalf("save blocked: %+v", out)
	}
	c.mu.Lock()
	applies, validations := c.applyCalls, c.validateCalls
	c.mu.Unlock()
	if validations != 0 {
		t.Fatal("pending save waited for core lifecycle validation during install")
	}
	if applies != 0 {
		t.Fatal("configuration save started proxy during installation")
	}
	close(c.release)
	s = waitInstall(t, a, "succeeded")
	if !s.Installed || s.Error != "" || s.Stage != "complete" {
		t.Fatalf("success %+v", s)
	}
	r := a.Runtime()
	if !r.Available || r.Running || !r.Dirty || !strings.Contains(r.LastError, "重新应用") {
		t.Fatalf("installation falsely applied config: %+v", r)
	}
	if a.store.Snapshot().Revision != before.Revision+1 {
		t.Fatal("installation changed revision independently of group save")
	}
	code, out = request(t, a, "POST", "/api/groups", map[string]any{"name": "saved after install"})
	if code != 200 || a.Runtime().Running {
		t.Fatalf("first save unexpectedly started proxy %+v", out)
	}
	code, out = request(t, a, "POST", "/api/core/install", map[string]any{})
	if code != 409 || c.installs.Load() != 1 {
		t.Fatal("existing core reinstalled")
	}
	code, out = request(t, a, "POST", "/api/runtime/apply", map[string]any{})
	if code != 200 || out["running"] != true {
		t.Fatalf("explicit apply failed: %d %+v", code, out)
	}
	if a.manuallyStopped || a.awaitingCoreApply {
		t.Fatal("explicit apply did not clear pause")
	}
}

func TestOnlineCoreInstallFailureCanRetryAndPreservesRevision(t *testing.T) {
	a, c := installFixture(t)
	c.failure = errors.New("下载校验失败，未安装")
	before := a.store.Snapshot().Revision
	if code, out := request(t, a, "POST", "/api/core/install", map[string]any{}); code != 202 {
		t.Fatalf("%d %+v", code, out)
	}
	<-c.started
	close(c.release)
	s := waitInstall(t, a, "failed")
	if s.Error != c.failure.Error() || !s.CanInstall || s.Installed || a.Runtime().Running {
		t.Fatalf("failed status %+v", s)
	}
	c.installMu.Lock()
	c.failure = nil
	c.started = make(chan struct{})
	c.release = make(chan struct{})
	c.installMu.Unlock()
	if code, out := request(t, a, "POST", "/api/core/install", map[string]any{}); code != 202 {
		t.Fatalf("retry %d %+v", code, out)
	}
	<-c.started
	close(c.release)
	waitInstall(t, a, "succeeded")
	if c.installs.Load() != 2 || a.store.Snapshot().Revision != before {
		t.Fatal("retry changed configuration or did not run")
	}
}

func TestOnlineCoreInstallSurvivesRequestDisconnectAndStopsWithApp(t *testing.T) {
	a, c := installFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	if _, err := a.startCoreInstall(ctx); err != nil {
		t.Fatal(err)
	}
	cancel()
	<-c.started
	if s := a.installationStatus(); s.State != "installing" {
		t.Fatal("browser disconnect canceled worker")
	}
	done := make(chan struct{})
	go func() { a.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown failed to cancel/wait worker")
	}
	if s := a.installationStatus(); s.State != "failed" || s.CanInstall {
		t.Fatalf("shutdown state %+v", s)
	}
	if _, err := a.startCoreInstall(context.Background()); err == nil {
		t.Fatal("installation launched after Close")
	}
}

func TestOnlineCoreInstallConcurrentRequestsOnlyOneWorker(t *testing.T) {
	a, c := installFixture(t)
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := a.startCoreInstall(context.Background()); err == nil {
				accepted.Add(1)
			}
		}()
	}
	wg.Wait()
	<-c.started
	if accepted.Load() != 1 || c.installs.Load() != 1 {
		t.Fatalf("accepted=%d workers=%d", accepted.Load(), c.installs.Load())
	}
	close(c.release)
	waitInstall(t, a, "succeeded")
}

func TestOnlineCoreInstallSecurityAndStrictBody(t *testing.T) {
	a, c := installFixture(t)
	for _, tc := range []struct {
		name, path, method, host, origin, token, site, body, contentType string
		want                                                             int
	}{
		{name: "host", path: "/api/core/install", method: "POST", host: "evil.example", body: "{}", contentType: "application/json", want: 403},
		{name: "origin", path: "/api/core/install", method: "POST", origin: "https://evil.example", body: "{}", contentType: "application/json", want: 403},
		{name: "fetch-site", path: "/api/core/install", method: "POST", site: "cross-site", body: "{}", contentType: "application/json", want: 403},
		{name: "url", path: "/api/core/install", method: "POST", body: `{"url":"https://attacker.invalid/core"}`, contentType: "application/json", want: 400},
		{name: "output", path: "/api/core/install", method: "POST", body: `{"path":"../victim"}`, contentType: "application/json", want: 400},
		{name: "force", path: "/api/core/install", method: "POST", body: `{"force":true}`, contentType: "application/json", want: 400},
		{name: "version", path: "/api/core/install", method: "POST", body: `{"version":"latest"}`, contentType: "application/json", want: 400},
		{name: "null", path: "/api/core/install", method: "POST", body: "null", contentType: "application/json", want: 400},
		{name: "array", path: "/api/core/install", method: "POST", body: "[]", contentType: "application/json", want: 400},
		{name: "extra-object", path: "/api/core/install", method: "POST", body: "{}{}", contentType: "application/json", want: 400},
		{name: "content-type", path: "/api/core/install", method: "POST", body: "{}", want: 415},
		{name: "inspect-override", path: "/api/core/inspect", method: "POST", body: `{"path":"evil"}`, contentType: "application/json", want: 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			host := tc.host
			if host == "" {
				host = "127.0.0.1:9090"
			}
			r := httptest.NewRequest(tc.method, "http://"+host+tc.path, strings.NewReader(tc.body))
			r.Header.Set("Origin", tc.origin)
			r.Header.Set("Sec-Fetch-Site", tc.site)
			r.Header.Set("Content-Type", tc.contentType)
			w := httptest.NewRecorder()
			a.Handler().ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
		})
	}
	a.opts.Token = strings.Repeat("x", 32)
	for _, path := range []string{"/api/core/install", "/api/core/installation", "/api/core/inspect"} {
		method := "POST"
		if strings.HasSuffix(path, "installation") {
			method = "GET"
		}
		r := httptest.NewRequest(method, "http://127.0.0.1:9090"+path, strings.NewReader("{}"))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatalf("unauthenticated %s status=%d", path, w.Code)
		}
		if bytes.Contains(w.Body.Bytes(), []byte(c.installation.Path)) {
			t.Fatal("local path leaked to unauthenticated client")
		}
	}
	if c.installs.Load() != 0 || c.InstallationStatus().State != "idle" {
		t.Fatal("rejected request triggered download")
	}
}

func TestOnlineCoreInspectDoesNotApplyOrChangeConfig(t *testing.T) {
	a, c := installFixture(t)
	before := a.store.Snapshot().Revision
	code, out := request(t, a, "POST", "/api/core/inspect", map[string]any{})
	if code != 200 {
		t.Fatalf("inspect %d %+v", code, out)
	}
	var parsed struct {
		CoreInstallation model.CoreInstallation
		Runtime          model.RuntimeStatus
	}
	b, _ := json.Marshal(out)
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.CoreInstallation.Installed || parsed.Runtime.Available || parsed.Runtime.Running || !parsed.Runtime.Dirty {
		t.Fatalf("false ready %+v", parsed)
	}
	c.mu.Lock()
	c.status.Available = true
	c.status.Version = coreinstall.PinnedVersion
	c.mu.Unlock()
	c.installMu.Lock()
	c.installation.Installed = true
	c.installation.CanInstall = false
	c.installMu.Unlock()
	code, out = request(t, a, "POST", "/api/core/inspect", map[string]any{})
	if code != 200 || a.Runtime().Running || !a.Runtime().Available || a.store.Snapshot().Revision != before || c.installs.Load() != 0 {
		t.Fatalf("inspect had side effects: %d %+v", code, out)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.applyCalls != 0 {
		t.Fatal("inspect applied config")
	}
}

func TestOnlineCoreInstallNonInstallableAdapter(t *testing.T) {
	a, _ := fixture(t, false)
	code, out := request(t, a, "GET", "/api/core/installation", nil)
	if code != 200 || out["canInstall"] != false || out["state"] != "idle" {
		t.Fatalf("fallback %d %+v", code, out)
	}
	code, out = request(t, a, "POST", "/api/core/install", map[string]any{})
	if code != 409 || out["code"] != "core_install_unavailable" {
		t.Fatalf("fallback install %d %+v", code, out)
	}
}
