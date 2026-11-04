package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/uniproxy/uniproxy/internal/model"
	"github.com/uniproxy/uniproxy/internal/store"
)

type fakeSidecar struct {
	mu                sync.Mutex
	status            model.SidecarStatus
	configureCalls    int
	preflightCalls    int
	configuredStart   int
	configuredEnd     int
	assignCalls       int
	applyCalls        int
	stopCalls         int
	configureErr      error
	preflightErr      error
	assignErr         error
	applyErr          error
	applied           []model.Node
	core              *fakeCore
	coreRunningAtStop bool
}

func (f *fakeSidecar) ConfigurePortRange(start, end int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.configureCalls++
	f.configuredStart = start
	f.configuredEnd = end
	return f.configureErr
}

func (f *fakeSidecar) Preflight([]model.Node) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.preflightCalls++
	return f.preflightErr
}

func (f *fakeSidecar) AssignEndpoints(nodes []model.Node) ([]model.Node, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.assignCalls++
	if f.assignErr != nil {
		return nil, f.assignErr
	}
	out := make([]model.Node, len(nodes))
	copy(out, nodes)
	for i := range out {
		if !out[i].Enabled || out[i].EngineName() != model.EngineXray {
			continue
		}
		if out[i].SidecarEndpoint == nil {
			out[i].SidecarEndpoint = &model.SidecarEndpoint{Host: "127.0.0.1", Port: 30100 + i}
		} else {
			endpoint := *out[i].SidecarEndpoint
			out[i].SidecarEndpoint = &endpoint
		}
		if out[i].SidecarEndpoint.Username == "" {
			out[i].SidecarEndpoint.Username = "user-" + out[i].ID
		}
		if out[i].SidecarEndpoint.Password == "" {
			out[i].SidecarEndpoint.Password = "pass-" + out[i].ID
		}
	}
	return out, nil
}

func (f *fakeSidecar) Apply(_ context.Context, nodes []model.Node) (map[string]model.SidecarEndpoint, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.applyCalls++
	f.applied = append([]model.Node(nil), nodes...)
	if f.applyErr != nil {
		return nil, f.applyErr
	}
	f.status.Running = true
	endpoints := map[string]model.SidecarEndpoint{}
	for _, node := range nodes {
		if node.Enabled && node.EngineName() == model.EngineXray && node.SidecarEndpoint != nil {
			endpoints[node.ID] = *node.SidecarEndpoint
		}
	}
	return endpoints, nil
}

func (f *fakeSidecar) Status() model.SidecarStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status
}

func (f *fakeSidecar) Stop(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopCalls++
	if f.core != nil {
		f.coreRunningAtStop = f.core.isRunning()
	}
	f.status.Running = false
	return nil
}

func (f *fakeCore) isRunning() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status.Running
}

func newSidecarFixture(t *testing.T) (*App, *fakeCore, *fakeSidecar) {
	t.Helper()
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	core := &fakeCore{status: model.RuntimeStatus{Available: true}}
	sidecar := &fakeSidecar{status: model.SidecarStatus{Engine: model.EngineXray, Version: "26.10.31"}, core: core}
	a := New(s, core, Options{AdminPort: 9090, Sidecar: sidecar})
	t.Cleanup(func() { a.Close(); s.Close() })
	return a, core, sidecar
}

func xrayAppNode(id string) model.Node {
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
	}
}

func addXrayNode(t *testing.T, a *App, id string) {
	t.Helper()
	if _, err := a.mutate(context.Background(), "加入 Xray 节点", func(s *model.State) (any, error) {
		s.Settings.EnableSidecar = true
		s.Nodes = append(s.Nodes, xrayAppNode(id))
		return nil, nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestStartAssignsSidecarEndpointsBeforeCoreApply(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	state := model.DefaultState()
	state.Settings.EnableSidecar = true
	state.Nodes = []model.Node{xrayAppNode("start-node")}
	if err := s.Save(state); err != nil {
		t.Fatal(err)
	}
	core := &fakeCore{status: model.RuntimeStatus{Available: true}}
	sidecar := &fakeSidecar{status: model.SidecarStatus{Engine: model.EngineXray}, core: core}
	a := New(s, core, Options{AdminPort: 9090, Sidecar: sidecar})
	a.Start()
	defer func() { a.Close(); s.Close() }()

	saved := a.store.Snapshot()
	if len(saved.Nodes) != 1 || saved.Nodes[0].SidecarEndpoint == nil {
		t.Fatalf("endpoint was not persisted on start: %#v", saved.Nodes)
	}
	if saved.Revision != 1 {
		t.Fatalf("revision = %d", saved.Revision)
	}
	if sidecar.applyCalls != 1 {
		t.Fatalf("sidecar apply calls = %d", sidecar.applyCalls)
	}
	if core.applied.Nodes[0].SidecarEndpoint == nil || core.applied.Nodes[0].SidecarEndpoint.Port != saved.Nodes[0].SidecarEndpoint.Port {
		t.Fatalf("core did not receive the sidecar endpoint: %#v", core.applied.Nodes)
	}
}

func TestSidecarLifecycleStatusAndBootstrapRedaction(t *testing.T) {
	a, core, sidecar := newSidecarFixture(t)
	addXrayNode(t, a, "xray-node")

	saved := a.store.Snapshot().Nodes[0]
	if saved.SidecarEndpoint == nil || saved.SidecarEndpoint.Port != 30100 {
		t.Fatalf("endpoint not saved: %#v", saved.SidecarEndpoint)
	}
	if core.applied.Nodes[0].SidecarEndpoint == nil || core.applied.Nodes[0].SidecarEndpoint.Username != "user-xray-node" {
		t.Fatalf("core did not receive credentials: %#v", core.applied.Nodes)
	}
	if sidecar.applyCalls != 1 || !sidecar.status.Running {
		t.Fatalf("sidecar not applied: %#v", sidecar)
	}

	code, out := request(t, a, "GET", "/api/sidecars", nil)
	if code != 200 || out["enabled"] != true || out["nodeCount"].(float64) != 1 {
		t.Fatalf("sidecars status = %d %#v", code, out)
	}
	if out["status"].(map[string]any)["running"] != true {
		t.Fatalf("sidecar runtime status missing: %#v", out)
	}
	if out["portRangeStart"].(float64) != 30000 || out["portRangeEnd"].(float64) != 30999 || out["maxNodes"].(float64) != 200 {
		t.Fatalf("sidecar range info missing: %#v", out)
	}

	raw, err := json.Marshal(a.bootstrap())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("pass-xray-node")) || bytes.Contains(raw, []byte("user-xray-node")) {
		t.Fatalf("bootstrap leaked sidecar credentials: %s", raw)
	}
	var boot struct {
		Nodes []model.Node `json:"nodes"`
	}
	if err := json.Unmarshal(raw, &boot); err != nil {
		t.Fatal(err)
	}
	if len(boot.Nodes) != 1 || boot.Nodes[0].SidecarEndpoint == nil || boot.Nodes[0].SidecarEndpoint.Port != 30100 {
		t.Fatalf("bootstrap lost endpoint address: %#v", boot.Nodes)
	}
	if boot.Nodes[0].SidecarEndpoint.Username != "" || boot.Nodes[0].SidecarEndpoint.Password != "" {
		t.Fatal("bootstrap retained credentials")
	}
	if boot.Nodes[0].SidecarStatus == nil || !boot.Nodes[0].SidecarStatus.Running {
		t.Fatalf("bootstrap missing sidecar status: %#v", boot.Nodes[0].SidecarStatus)
	}
}

func TestSidecarApplyFailureDoesNotTouchCore(t *testing.T) {
	a, core, sidecar := newSidecarFixture(t)
	sidecar.applyErr = errors.New("boom")
	_, err := a.mutate(context.Background(), "加入 Xray 节点", func(s *model.State) (any, error) {
		s.Settings.EnableSidecar = true
		s.Nodes = append(s.Nodes, xrayAppNode("fail-node"))
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if core.applyCalls != 0 {
		t.Fatalf("core applied despite sidecar failure: %d", core.applyCalls)
	}
	if !strings.Contains(a.Runtime().LastError, "Xray sidecar 启动失败") {
		t.Fatalf("runtime error = %q", a.Runtime().LastError)
	}
	if a.store.Snapshot().Nodes[0].SidecarEndpoint == nil {
		t.Fatal("allocated endpoint was not saved before apply")
	}
}

func TestSidecarFeatureOffRejectsXrayNode(t *testing.T) {
	a, _, _ := newSidecarFixture(t)
	_, err := a.mutate(context.Background(), "加入 Xray 节点", func(s *model.State) (any, error) {
		s.Nodes = append(s.Nodes, xrayAppNode("off-node"))
		return nil, nil
	})
	if err == nil || !strings.Contains(err.Error(), "Xray sidecar 功能未启用") {
		t.Fatalf("feature-off error = %v", err)
	}
	if len(a.store.Snapshot().Nodes) != 0 {
		t.Fatal("invalid node was persisted")
	}
}

func TestRuntimeStopStopsSidecarAfterCore(t *testing.T) {
	a, core, sidecar := newSidecarFixture(t)
	addXrayNode(t, a, "stop-node")
	if !core.isRunning() {
		t.Fatal("core should be running before stop")
	}
	code, out := request(t, a, "POST", "/api/runtime/stop", nil)
	if code != 200 {
		t.Fatalf("stop = %d %#v", code, out)
	}
	if sidecar.stopCalls != 1 || sidecar.coreRunningAtStop {
		t.Fatalf("sidecar stop order = calls %d coreRunning %v", sidecar.stopCalls, sidecar.coreRunningAtStop)
	}
	if !strings.Contains(a.Runtime().LastError, "代理内核已手动停止") {
		t.Fatalf("runtime error = %q", a.Runtime().LastError)
	}
}

func TestSettingsPatchPersistsSidecarPortRangeAndConfiguresManager(t *testing.T) {
	a, _, sidecar := newSidecarFixture(t)
	addXrayNode(t, a, "range-node")
	sidecar.mu.Lock()
	sidecar.configureCalls = 0
	sidecar.mu.Unlock()

	code, out := request(t, a, "PATCH", "/api/settings", map[string]any{"sidecarPortRangeStart": 30500, "sidecarPortRangeEnd": 30510})
	if code != 200 {
		t.Fatalf("patch = %d %#v", code, out)
	}
	saved := a.store.Snapshot().Settings
	if saved.SidecarPortRangeStart != 30500 || saved.SidecarPortRangeEnd != 30510 {
		t.Fatalf("sidecar range = %d-%d", saved.SidecarPortRangeStart, saved.SidecarPortRangeEnd)
	}
	sidecar.mu.Lock()
	start, end, calls := sidecar.configuredStart, sidecar.configuredEnd, sidecar.configureCalls
	sidecar.mu.Unlock()
	if calls == 0 || start != 30500 || end != 30510 {
		t.Fatalf("manager not configured: calls=%d range=%d-%d", calls, start, end)
	}
}

func TestSettingsPatchRejectsInvalidSidecarPortRange(t *testing.T) {
	a, _, _ := newSidecarFixture(t)
	if code, _ := request(t, a, "PATCH", "/api/settings", map[string]any{"enableSidecar": true}); code != 200 {
		t.Fatal("failed to enable sidecar")
	}
	cases := []struct {
		name  string
		patch map[string]any
		want  string
	}{
		{"overlap", map[string]any{"sidecarPortRangeStart": 20500, "sidecarPortRangeEnd": 30500}, "Sidecar 端口范围不能与独立监听端口范围重叠"},
		{"admin", map[string]any{"sidecarPortRangeStart": 9000, "sidecarPortRangeEnd": 9500}, "Sidecar 端口范围不能包含管理端口"},
		{"reversed", map[string]any{"sidecarPortRangeStart": 31000, "sidecarPortRangeEnd": 30000}, "Sidecar 端口分配范围无效"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, out := request(t, a, "PATCH", "/api/settings", tc.patch)
			if code == 200 || !strings.Contains(out["error"].(string), tc.want) {
				t.Fatalf("patch = %d %#v", code, out)
			}
		})
	}
	if saved := a.store.Snapshot().Settings; saved.SidecarPortRangeStart != model.DefaultSidecarPortRangeStart || saved.SidecarPortRangeEnd != model.DefaultSidecarPortRangeEnd {
		t.Fatalf("invalid range was persisted: %d-%d", saved.SidecarPortRangeStart, saved.SidecarPortRangeEnd)
	}
}

func TestValidateStateRejectsListenerInSidecarRange(t *testing.T) {
	state := model.DefaultState()
	state.Settings.EnableSidecar = true
	state.Listeners = []model.Listener{{ID: "listener", Name: "listener", Kind: "node", TargetID: "missing", Protocol: "socks", Host: "127.0.0.1", Port: 30500, Enabled: false}}
	err := ValidateState(state, 9090)
	if err == nil || !strings.Contains(err.Error(), "监听端口 30500 落在 Sidecar 端口范围内") {
		t.Fatalf("listener in range accepted: %v", err)
	}
}

func TestPrepareSidecarRejectsNodeLimit(t *testing.T) {
	a, _, sidecar := newSidecarFixture(t)
	state := model.DefaultState()
	state.Settings.EnableSidecar = true
	for i := 0; i <= 200; i++ {
		state.Nodes = append(state.Nodes, xrayAppNode(fmt.Sprintf("limit-%d", i)))
	}
	err := a.prepareSidecar(&state)
	if err == nil || !strings.Contains(err.Error(), "最多支持 200 个节点") {
		t.Fatalf("node limit error = %v", err)
	}
	sidecar.mu.Lock()
	calls := sidecar.configureCalls
	sidecar.mu.Unlock()
	if calls != 0 {
		t.Fatalf("manager configured before node limit: %d", calls)
	}
}

func TestSidecarPreflightFailureBlocksCore(t *testing.T) {
	a, core, sidecar := newSidecarFixture(t)
	sidecar.preflightErr = errors.New("occupied")
	_, err := a.mutate(context.Background(), "加入 Xray 节点", func(s *model.State) (any, error) {
		s.Settings.EnableSidecar = true
		s.Nodes = append(s.Nodes, xrayAppNode("preflight-node"))
		return nil, nil
	})
	if err == nil || !strings.Contains(err.Error(), "sidecar 端口预检失败") {
		t.Fatalf("preflight error = %v", err)
	}
	if core.applyCalls != 0 {
		t.Fatalf("core applied despite preflight failure: %d", core.applyCalls)
	}
	if len(a.store.Snapshot().Nodes) != 0 {
		t.Fatal("invalid node was persisted")
	}
}

func TestSettingsPatchEnablesSidecar(t *testing.T) {
	a, _, _ := newSidecarFixture(t)
	code, out := request(t, a, "PATCH", "/api/settings", map[string]any{"enableSidecar": true})
	if code != 200 {
		t.Fatalf("patch = %d %#v", code, out)
	}
	if !a.store.Snapshot().Settings.EnableSidecar {
		t.Fatal("enableSidecar was not persisted")
	}
}
