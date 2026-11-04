package sidecar

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/uniproxy/uniproxy/internal/model"
)

func testXrayNode(id string) model.Node {
	return model.Node{
		ID: id,
		ParsedNode: model.ParsedNode{
			Name: id, Protocol: "vless", Server: "example.com", Port: 443, UDP: true,
			Config: map[string]any{
				"xray": map[string]any{
					"protocol": "vless",
					"settings": map[string]any{
						"encryption": "none",
						"vnext": []any{map[string]any{
							"address": "template.invalid", "port": 1,
							"users": []any{map[string]any{"id": "11111111-1111-4111-8111-111111111111", "encryption": "none", "level": 0}},
						}},
					},
					"streamSettings": map[string]any{"security": "tls", "network": "tcp", "tlsSettings": map[string]any{"serverName": "example.com"}},
				},
			},
		},
		Enabled: true, Engine: model.EngineXray,
		SidecarEndpoint: &model.SidecarEndpoint{Host: "127.0.0.1", Port: 31001, Username: "u-test", Password: "password"},
	}
}

func decodeXrayConfig(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("config is not JSON: %v", err)
	}
	return document
}

func TestBuildXrayConfigUsesLocalInboundAndRemoteOutbound(t *testing.T) {
	node := testXrayNode("a")
	data, err := BuildXrayConfig([]model.Node{node})
	if err != nil {
		t.Fatal(err)
	}
	document := decodeXrayConfig(t, data)
	inbounds := document["inbounds"].([]any)
	if len(inbounds) != 1 {
		t.Fatalf("inbounds = %d", len(inbounds))
	}
	inbound := inbounds[0].(map[string]any)
	if inbound["listen"] != "127.0.0.1" || inbound["protocol"] != "socks" {
		t.Fatalf("unexpected inbound: %#v", inbound)
	}
	if inbound["port"].(float64) != 31001 {
		t.Fatalf("inbound port = %v", inbound["port"])
	}
	settings := inbound["settings"].(map[string]any)
	accounts := settings["accounts"].([]any)
	if len(accounts) != 1 || accounts[0].(map[string]any)["user"] != "u-test" {
		t.Fatalf("unexpected accounts: %#v", accounts)
	}
	if settings["auth"] != "password" {
		t.Fatalf("inbound auth = %v", settings["auth"])
	}
	outbounds := document["outbounds"].([]any)
	if len(outbounds) != 2 {
		t.Fatalf("outbounds = %d", len(outbounds))
	}
	outbound := outbounds[0].(map[string]any)
	if outbound["protocol"] != "vless" || outbound["tag"] != "out-a" {
		t.Fatalf("unexpected outbound: %#v", outbound)
	}
	outSettings := outbound["settings"].(map[string]any)
	vnext := outSettings["vnext"].([]any)[0].(map[string]any)
	if vnext["address"] != "example.com" || vnext["port"].(float64) != 443 {
		t.Fatalf("remote destination was not authoritative: %#v", vnext)
	}
	rules := document["routing"].(map[string]any)["rules"].([]any)
	first := rules[0].(map[string]any)
	if first["outboundTag"] != "out-a" {
		t.Fatalf("routing rule = %#v", first)
	}
}

func TestBuildXrayConfigRejectsInvalidInputs(t *testing.T) {
	node := testXrayNode("a")
	node.Engine = model.EngineMihomo
	if _, err := BuildXrayConfig([]model.Node{node}); err == nil {
		t.Fatal("accepted non-Xray node")
	}

	node = testXrayNode("a")
	node.SidecarEndpoint.Port = 31002
	second := testXrayNode("b")
	second.SidecarEndpoint.Port = 31002
	if _, err := BuildXrayConfig([]model.Node{node, second}); err == nil {
		t.Fatal("accepted duplicate endpoint ports")
	}

	node = testXrayNode("a")
	node.Config = map[string]any{"xray": map[string]any{"protocol": "vless", "settings": map[string]any{"vnext": []any{map[string]any{"address": "x", "port": 1}}, "certificateFile": "secret.pem"}}}
	if _, err := BuildXrayConfig([]model.Node{node}); err == nil {
		t.Fatal("accepted local certificate file reference")
	}

	node = testXrayNode("a")
	xrayTemplate := node.Config["xray"].(map[string]any)
	xraySettings := xrayTemplate["settings"].(map[string]any)
	xraySettings["vnext"] = append(xraySettings["vnext"].([]any), map[string]any{"address": "second.invalid", "port": 2})
	if _, err := BuildXrayConfig([]model.Node{node}); err == nil {
		t.Fatal("accepted multiple vnext entries")
	}
}

func TestPortAllocatorAllocatesDistinctLoopbackPorts(t *testing.T) {
	alloc, err := NewPortAllocator(31500, 31520)
	if err != nil {
		t.Fatal(err)
	}
	first, err := alloc.Allocate("127.0.0.1", true)
	if err != nil {
		t.Fatal(err)
	}
	second, err := alloc.Allocate("127.0.0.1", false)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("duplicate ports %d", first)
	}
	if err := alloc.Reserve(first); err == nil {
		t.Fatal("Reserve accepted an already used port")
	}
}

func findXrayBinary(t *testing.T) string {
	t.Helper()
	if value := os.Getenv("UNIPROXY_XRAY_BIN"); value != "" {
		return value
	}
	candidates := []string{
		filepath.Join("..", "..", "..", "uniproxy", ".local", "xray-test", "xray.exe"),
		filepath.Join("..", "..", ".local", "xray-test", "xray.exe"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			absolute, _ := filepath.Abs(candidate)
			return absolute
		}
	}
	if path, err := exec.LookPath("xray"); err == nil {
		return path
	}
	t.Skip("set UNIPROXY_XRAY_BIN to run real Xray configuration tests")
	return ""
}

func TestBuildXrayConfigAcceptedByXray(t *testing.T) {
	binary := findXrayBinary(t)
	node := testXrayNode("real")
	data, err := BuildXrayConfig([]model.Node{node})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := runXrayTest(ctx, binary, path, []model.Node{node}); err != nil {
		t.Fatalf("real xray rejected generated config: %v", err)
	}
}
