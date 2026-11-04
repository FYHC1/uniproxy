package sidecar

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/uniproxy/uniproxy/internal/model"
)

func endpointTestManager(t *testing.T) *XrayManager {
	t.Helper()
	manager, err := NewXrayManager(XrayOptions{
		WorkDir:        t.TempDir(),
		PortRangeStart: 32100,
		PortRangeEnd:   32110,
		Rand:           bytes.NewReader(bytes.Repeat([]byte{7}, 512)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func TestNewXrayManagerCreatesWorkDir(t *testing.T) {
	workDir := filepath.Join(t.TempDir(), "nested", "sidecar")
	if _, err := NewXrayManager(XrayOptions{WorkDir: workDir}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(workDir)
	if err != nil || !info.IsDir() {
		t.Fatalf("work directory was not created: %v", err)
	}
}

func endpointTestNode(id string, enabled bool, engine model.NodeEngine, endpoint *model.SidecarEndpoint) model.Node {
	return model.Node{
		ID: id, Enabled: enabled, Engine: engine,
		ParsedNode:      model.ParsedNode{Name: id, Protocol: "vless", Server: "example.com", Port: 443},
		SidecarEndpoint: endpoint,
	}
}

func TestAssignEndpointsPreservesExistingAndAllocatesMissing(t *testing.T) {
	manager := endpointTestManager(t)
	existing := &model.SidecarEndpoint{Host: "127.0.0.1", Port: 32102, Username: "existing-user", Password: "existing-pass"}
	original := []model.Node{
		endpointTestNode("existing", true, model.EngineXray, existing),
		endpointTestNode("missing", true, model.EngineXray, nil),
		endpointTestNode("disabled", false, model.EngineXray, nil),
		endpointTestNode("native", true, model.EngineMihomo, nil),
	}
	assigned, err := manager.AssignEndpoints(original)
	if err != nil {
		t.Fatal(err)
	}
	if original[0].SidecarEndpoint != existing || original[1].SidecarEndpoint != nil {
		t.Fatal("input nodes were modified")
	}
	if assigned[0].SidecarEndpoint != existing && (assigned[0].SidecarEndpoint.Host != "127.0.0.1" || assigned[0].SidecarEndpoint.Port != 32102 || assigned[0].SidecarEndpoint.Username != "existing-user") {
		t.Fatalf("existing endpoint changed: %#v", assigned[0].SidecarEndpoint)
	}
	missing := assigned[1].SidecarEndpoint
	if missing == nil || missing.Host != "127.0.0.1" || missing.Port < 32100 || missing.Port > 32110 {
		t.Fatalf("missing endpoint not allocated: %#v", missing)
	}
	if missing.Username == "" || missing.Password == "" {
		t.Fatal("missing credentials not generated")
	}
	if assigned[2].SidecarEndpoint != nil || assigned[3].SidecarEndpoint != nil {
		t.Fatal("disabled or native node received a sidecar endpoint")
	}
	second, err := manager.AssignEndpoints(assigned)
	if err != nil {
		t.Fatal(err)
	}
	if second[1].SidecarEndpoint.Port != missing.Port || second[1].SidecarEndpoint.Username != missing.Username {
		t.Fatal("existing generated endpoint was not preserved")
	}
}

func TestConfigurePortRangeAppliesToNewAssignments(t *testing.T) {
	manager := endpointTestManager(t)
	if err := manager.ConfigurePortRange(32200, 32202); err != nil {
		t.Fatal(err)
	}
	start, end := manager.PortRange()
	if start != 32200 || end != 32202 {
		t.Fatalf("range = %d-%d", start, end)
	}
	assigned, err := manager.AssignEndpoints([]model.Node{endpointTestNode("new", true, model.EngineXray, nil)})
	if err != nil {
		t.Fatal(err)
	}
	if port := assigned[0].SidecarEndpoint.Port; port < 32200 || port > 32202 {
		t.Fatalf("allocated port outside configured range: %d", port)
	}
}

func TestPreflightRejectsOccupiedPort(t *testing.T) {
	manager := endpointTestManager(t)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	node := endpointTestNode("occupied", true, model.EngineXray, &model.SidecarEndpoint{Host: "127.0.0.1", Port: port, Username: "u", Password: "p"})
	if err := manager.Preflight([]model.Node{node}); err == nil || !strings.Contains(err.Error(), "sidecar 端口 "+strconv.Itoa(port)+" 已被占用") {
		t.Fatalf("occupied port accepted: %v", err)
	}
}

func TestRedactSecretsRemovesUsernameAndPassword(t *testing.T) {
	node := endpointTestNode("redact", true, model.EngineXray, &model.SidecarEndpoint{Host: "127.0.0.1", Port: 32100, Username: "visible-user", Password: "visible-pass"})
	output := redactSecrets("user=visible-user pass=visible-pass done", []model.Node{node})
	if strings.Contains(output, "visible-user") || strings.Contains(output, "visible-pass") || !strings.Contains(output, "[redacted]") {
		t.Fatalf("secrets were not redacted: %q", output)
	}
}

func TestAssignEndpointsRejectsUnsupportedEngineAndDuplicatePort(t *testing.T) {
	manager := endpointTestManager(t)
	unsupported := []model.Node{endpointTestNode("box", true, model.EngineSingBox, nil)}
	if _, err := manager.AssignEndpoints(unsupported); err == nil || !strings.Contains(err.Error(), "unsupported sidecar engine") {
		t.Fatalf("unsupported engine accepted: %v", err)
	}
	duplicate := []model.Node{
		endpointTestNode("a", true, model.EngineXray, &model.SidecarEndpoint{Host: "127.0.0.1", Port: 32105, Username: "a", Password: "a"}),
		endpointTestNode("b", true, model.EngineXray, &model.SidecarEndpoint{Host: "127.0.0.1", Port: 32105, Username: "b", Password: "b"}),
	}
	if _, err := manager.AssignEndpoints(duplicate); err == nil {
		t.Fatal("duplicate endpoint port accepted")
	}
}
