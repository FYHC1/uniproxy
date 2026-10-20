package integration

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/uniproxy/uniproxy/internal/app"
	"github.com/uniproxy/uniproxy/internal/core"
	"github.com/uniproxy/uniproxy/internal/model"
	"github.com/uniproxy/uniproxy/internal/store"
)

// This is deliberately opt-in: normal Go/browser tests must not fetch a release.
// The only remote traffic is the official pinned binary download. No user data,
// subscriptions or proxy nodes are used; the installed core only starts empty
// local management/probe listeners after an explicit apply request.
func TestOnlineCoreDownloadThroughManagementAPI(t *testing.T) {
	if os.Getenv("UNIPROXY_TEST_CORE_DOWNLOAD") != "1" {
		t.Skip("set UNIPROXY_TEST_CORE_DOWNLOAD=1 to download a verified core into a temporary directory")
	}
	dir := t.TempDir()
	name := "mihomo"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(dir, "core", name)
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	m := core.New(core.Options{BinaryPath: path, DataDir: filepath.Dir(path)})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	a := app.New(s, m, app.Options{Host: "127.0.0.1", AdminPort: port, Token: strings.Repeat("integration-token-", 2)})
	a.Start()
	defer a.Close()
	server := httptest.NewUnstartedServer(a.Handler())
	server.Listener.Close()
	server.Listener = listener
	server.Start()
	defer server.Close()
	client := &http.Client{Timeout: 30 * time.Second}
	base := "http://127.0.0.1:" + strconv.Itoa(port)
	call := func(method, path string, want int, target any) {
		t.Helper()
		var body io.Reader
		if method == "POST" {
			body = strings.NewReader("{}")
		}
		r, err := http.NewRequestWithContext(context.Background(), method, base+path, body)
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Authorization", "Bearer "+strings.Repeat("integration-token-", 2))
		r.Header.Set("Content-Type", "application/json")
		res, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		if res.StatusCode != want {
			var problem map[string]any
			_ = json.NewDecoder(res.Body).Decode(&problem)
			t.Fatalf("%s %s status=%d want=%d: %+v", method, path, res.StatusCode, want, problem)
		}
		if target != nil {
			if err = json.NewDecoder(res.Body).Decode(target); err != nil {
				t.Fatal(err)
			}
		}
	}
	var status model.CoreInstallation
	call("GET", "/api/core/installation", 200, &status)
	if status.Installed || !status.CanInstall {
		t.Fatalf("not isolated missing state: %+v", status)
	}
	before := s.Snapshot().Revision
	call("POST", "/api/core/install", 202, &status)
	call("POST", "/api/core/install", 409, nil)
	deadline := time.NewTimer(5*time.Minute + 30*time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for status.State == "installing" {
		select {
		case <-deadline.C:
			t.Fatalf("installation deadline; last state %+v", status)
		case <-ticker.C:
			call("GET", "/api/core/installation", 200, &status)
		}
	}
	if status.State != "succeeded" || !status.Installed || status.CanInstall || status.BytesDownloaded == 0 || status.BytesDownloaded != status.TotalBytes {
		t.Fatalf("download did not finish cleanly: %+v", status)
	}
	var r model.RuntimeStatus
	call("GET", "/api/runtime", 200, &r)
	if !r.Available || r.Running || !r.Dirty || r.Version != core.PinnedVersion {
		t.Fatalf("install not distinct from runtime: %+v", r)
	}
	if s.Snapshot().Revision != before {
		t.Fatal("install mutated desired config")
	}
	if info, err := os.Stat(path); err != nil || info.Size() == 0 {
		t.Fatalf("installed binary missing %v", err)
	}
	call("POST", "/api/runtime/apply", 200, &r)
	if !r.Running || r.Dirty {
		t.Fatalf("explicit apply failed: %+v", r)
	}
	call("POST", "/api/core/install", 409, nil)
	call("POST", "/api/runtime/stop", 200, &r)
	if r.Running {
		t.Fatal("owned temporary core failed to stop")
	}
	t.Logf("Official Mihomo v%s installed through authenticated API (%d archive bytes), version verified, explicit empty-config start/stop passed", status.Version, status.BytesDownloaded)
}
