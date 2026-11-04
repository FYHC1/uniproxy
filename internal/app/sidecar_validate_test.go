package app

import (
	"strings"
	"testing"

	"github.com/uniproxy/uniproxy/internal/model"
)

func sidecarValidationNode(id string) model.Node {
	return model.Node{
		ID: id, Enabled: true, Engine: model.EngineXray,
		ParsedNode: model.ParsedNode{
			Name: id, Protocol: "vless", Server: "example.com", Port: 443,
			Config: map[string]any{"xray": map[string]any{
				"protocol": "vless",
				"settings": map[string]any{"vnext": []any{map[string]any{
					"address": "template.invalid", "port": 1,
					"users": []any{map[string]any{"id": "11111111-1111-4111-8111-111111111111", "encryption": "none"}},
				}}},
				"streamSettings": map[string]any{"network": "tcp"},
			}},
		},
		SidecarEndpoint: &model.SidecarEndpoint{Host: "127.0.0.1", Port: 30100, Username: "u-" + id, Password: "p-" + id},
	}
}

func TestValidateStateSidecarEndpoint(t *testing.T) {
	base := model.DefaultState()
	base.Settings.EnableSidecar = true
	sidecar := sidecarValidationNode("sidecar")
	native := model.Node{ID: "native", Enabled: true, ParsedNode: model.ParsedNode{Name: "native", Protocol: "vless", Server: "example.com", Port: 443, Config: map[string]any{"type": "vless"}}}
	base.Nodes = []model.Node{sidecar, native}
	if err := ValidateState(base, 9090); err != nil {
		t.Fatalf("valid sidecar state rejected: %v", err)
	}

	bad := sidecar
	bad.SidecarEndpoint = &model.SidecarEndpoint{Host: "127.0.0.2", Port: 30100, Username: "u", Password: "p"}
	s := base
	s.Nodes = []model.Node{bad}
	if err := ValidateState(s, 9090); err == nil || !strings.Contains(err.Error(), "sidecar 端点") {
		t.Fatalf("non-loopback endpoint accepted: %v", err)
	}

	bad = sidecar
	bad.Engine = model.EngineMihomo
	s.Nodes = []model.Node{bad}
	if err := ValidateState(s, 9090); err == nil || !strings.Contains(err.Error(), "原生节点不能设置") {
		t.Fatalf("native endpoint accepted: %v", err)
	}
}

func TestValidateStateSidecarChainPlacement(t *testing.T) {
	base := model.DefaultState()
	base.Settings.EnableSidecar = true
	base.Nodes = []model.Node{sidecarValidationNode("sidecar"), {ID: "native", Enabled: true, ParsedNode: model.ParsedNode{Name: "native", Protocol: "vless", Server: "example.com", Port: 443, Config: map[string]any{"type": "vless"}}}}
	base.Chains = []model.Chain{{ID: "route", Name: "route", NodeIDs: []string{"native", "sidecar"}, Enabled: true}}
	if err := ValidateState(base, 9090); err == nil || !strings.Contains(err.Error(), "sidecar 节点只允许位于链首") {
		t.Fatalf("sidecar chain tail accepted: %v", err)
	}
	base.Chains[0].NodeIDs = []string{"sidecar", "native"}
	if err := ValidateState(base, 9090); err != nil {
		t.Fatalf("sidecar chain head rejected: %v", err)
	}
}
