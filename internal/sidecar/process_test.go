package sidecar

import (
	"context"
	"testing"
	"time"

	"github.com/uniproxy/uniproxy/internal/model"
)

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
