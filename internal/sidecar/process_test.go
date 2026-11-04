package sidecar

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/uniproxy/uniproxy/internal/model"
)

func TestXrayManagerRejectsUnpinnedVersionBeforeStarting(t *testing.T) {
	binary := filepath.Join(t.TempDir(), xrayBinaryName())
	if err := os.WriteFile(binary, []byte("stub"), 0o700); err != nil {
		t.Fatal(err)
	}
	manager, err := NewXrayManager(XrayOptions{BinaryPath: binary, WorkDir: t.TempDir(), PortRangeStart: 31650, PortRangeEnd: 31660})
	if err != nil {
		t.Fatal(err)
	}
	manager.probeVersion = func(context.Context, string) (string, error) { return "1.2.3", nil }
	_, err = manager.Start(context.Background(), []model.Node{testXrayNode("version-check")})
	if err == nil || !strings.Contains(err.Error(), "Xray 版本不匹配") {
		t.Fatalf("unpinned binary was accepted: %v", err)
	}
	if manager.Status().Running {
		t.Fatal("manager reported running after version rejection")
	}
}

func TestXrayManagerRestartsAfterUnexpectedExit(t *testing.T) {
	binary := findXrayBinary(t)
	manager, err := NewXrayManager(XrayOptions{BinaryPath: binary, PortRangeStart: 31660, PortRangeEnd: 31690, TestTimeout: 15 * time.Second, StartupTimeout: 20 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	node := testXrayNode("crash-restart")
	node.SidecarEndpoint = nil
	ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
	defer cancel()
	if _, err := manager.Start(ctx, []model.Node{node}); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer manager.Stop(context.Background())

	manager.mu.Lock()
	if manager.cmd == nil || manager.cmd.Process == nil {
		manager.mu.Unlock()
		t.Fatal("missing xray process handle")
	}
	oldPID := manager.cmd.Process.Pid
	manager.mu.Unlock()
	process, err := os.FindProcess(oldPID)
	if err != nil {
		t.Fatal(err)
	}
	if err := process.Kill(); err != nil {
		t.Fatalf("kill xray: %v", err)
	}

	deadline := time.Now().Add(30 * time.Second)
	for {
		manager.mu.Lock()
		restarted := manager.status.Running && manager.cmd != nil && manager.cmd.Process != nil && manager.cmd.Process.Pid != oldPID && manager.status.Error == ""
		lastError := manager.status.Error
		manager.mu.Unlock()
		if restarted {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("xray did not restart after crash; last error = %q", lastError)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := manager.Health(ctx); err != nil {
		t.Fatalf("health after restart: %v", err)
	}
}

func TestXrayManagerStartsAndStopsSingleProcess(t *testing.T) {
	binary := findXrayBinary(t)
	manager, err := NewXrayManager(XrayOptions{BinaryPath: binary, PortRangeStart: 31600, PortRangeEnd: 31640, TestTimeout: 15 * time.Second, StartupTimeout: 20 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	node := testXrayNode("mgr")
	node.SidecarEndpoint = nil
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	endpoints, err := manager.Start(ctx, []model.Node{node})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if len(endpoints) != 1 {
		t.Fatalf("endpoints = %d", len(endpoints))
	}
	endpoint := endpoints["mgr"]
	if endpoint.Host != "127.0.0.1" || endpoint.Port < 31600 || endpoint.Port > 31640 || endpoint.Username == "" || endpoint.Password == "" {
		t.Fatalf("unexpected generated endpoint: %#v", endpoint)
	}
	status := manager.Status()
	if !status.Running || status.Engine != "xray" || status.Version != XrayVersion {
		t.Fatalf("unexpected status: %#v", status)
	}
	if err := manager.Health(ctx); err != nil {
		t.Fatalf("health: %v", err)
	}
	if err := manager.Stop(ctx); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if manager.Status().Running {
		t.Fatal("manager still reports running after stop")
	}
}
