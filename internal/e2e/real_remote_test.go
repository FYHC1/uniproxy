package e2e

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/uniproxy/uniproxy/internal/model"
)

// TestRealRemoteSubscription exercises the full subscription path against a
// live remote Xray server: HTTP subscription fetch, importer classification,
// persisted Xray nodes, local sidecar startup, local mixed listeners, and a
// remote HTTP fetch through every imported protocol. Set UNIPROXY_E2E_REMOTE_NODES
// when the fixture exposes a single protocol; the default is three.
// It is opt-in because it needs a live remote fixture.
func TestRealRemoteSubscription(t *testing.T) {
	subURL := strings.TrimSpace(os.Getenv("UNIPROXY_E2E_REMOTE_SUB"))
	if subURL == "" {
		t.Skip("set UNIPROXY_E2E_REMOTE_SUB to a live remote subscription URL")
	}
	targetURL := strings.TrimSpace(os.Getenv("UNIPROXY_E2E_REMOTE_TARGET"))
	if targetURL == "" {
		targetURL = "http://210.109.205.7:20080/ok-remote.txt"
	}
	expectedNodes := 3
	if raw := strings.TrimSpace(os.Getenv("UNIPROXY_E2E_REMOTE_NODES")); raw != "" {
		n, parseErr := strconv.Atoi(raw)
		if parseErr != nil || n <= 0 {
			t.Fatalf("UNIPROXY_E2E_REMOTE_NODES = %q, want a positive integer", raw)
		}
		expectedNodes = n
	}
	var expectedNames []string
	if raw := strings.TrimSpace(os.Getenv("UNIPROXY_E2E_REMOTE_NAMES")); raw != "" {
		for _, name := range strings.Split(raw, ",") {
			if name = strings.TrimSpace(name); name != "" {
				expectedNames = append(expectedNames, name)
			}
		}
	}
	const wantBody = "remote-e2e-ok"

	xrayBinary := e2eRealBinary(t, "UNIPROXY_E2E_XRAY", filepath.Join("..", "uniproxy", ".local", "xray-test", "xray.exe"), ".local/real-test-linux/xray")
	mihomoBinary := e2eRealBinary(t, "UNIPROXY_E2E_MIHOMO", filepath.Join("..", "uniproxy", "bin", "mihomo.exe"), ".local/real-test-linux/mihomo")
	appBinary := e2eBuildApp(t)

	dataDir := t.TempDir()
	e2eSeedState(t, dataDir, nil, nil)
	app := e2eStartApp(t, appBinary, dataDir, mihomoBinary, xrayBinary)
	t.Cleanup(func() {
		logs, _ := os.ReadFile(app.logPath)
		if len(logs) > 12000 {
			logs = logs[len(logs)-12000:]
		}
		t.Logf("application log tail:\n%s", logs)
	})
	client := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: nil}}
	pre := e2eBootstrapState(t, app)
	t.Logf("pre-import nodes=%d subscriptions=%d listeners=%d", len(pre.Nodes), len(pre.Subscriptions), len(pre.Listeners))

	importData, err := e2eJSON(client, http.MethodPost, app.adminURL+"/api/import", map[string]any{
		"mode": "subscriptions", "content": subURL, "name": "kr-vps remote fixture",
	})
	if err != nil {
		t.Fatalf("import remote subscription: %v", err)
	}
	var imported model.ImportResult
	if err := json.Unmarshal(e2eUnwrapData(t, importData), &imported); err != nil {
		t.Fatalf("decode import result: %v", err)
	}
	if imported.Added != expectedNodes || imported.TotalErrors != 0 {
		t.Fatalf("import result = %+v, want %d added nodes and no errors", imported, expectedNodes)
	}
	if len(imported.SubscriptionIDs) != 1 {
		t.Fatalf("subscription ids = %v, want exactly one", imported.SubscriptionIDs)
	}

	info := e2eWaitSidecarRunning(t, app)
	if info.NodeCount != expectedNodes {
		t.Fatalf("sidecar nodeCount = %d, want %d", info.NodeCount, expectedNodes)
	}
	sidecarRaw, err := e2eJSON(client, http.MethodGet, app.adminURL+"/api/sidecars", nil)
	if err == nil {
		t.Logf("sidecar status: %s", e2eUnwrapData(t, sidecarRaw))
	}
	e2eWaitRuntimeRunning(t, app)

	state := e2eBootstrapState(t, app)
	if len(state.Nodes) != expectedNodes {
		t.Fatalf("bootstrap nodes = %d, want %d", len(state.Nodes), expectedNodes)
	}
	nodeIDs := make([]string, 0, len(state.Nodes))
	seen := map[string]bool{}
	for _, node := range state.Nodes {
		if node.EngineName() != model.EngineXray {
			t.Fatalf("node %q engine = %q, want xray", node.Name, node.EngineName())
		}
		if node.SidecarEndpoint == nil {
			t.Fatalf("node %q has no sidecar endpoint", node.Name)
		}
		t.Logf("node %s engine=%s endpoint=%s:%d status=%+v", node.Name, node.EngineName(), node.SidecarEndpoint.Host, node.SidecarEndpoint.Port, node.SidecarStatus)
		if seen[node.Name] {
			t.Fatalf("duplicate imported node name %q", node.Name)
		}
		seen[node.Name] = true
		nodeIDs = append(nodeIDs, node.ID)
	}
	for _, want := range expectedNames {
		if !seen[want] {
			t.Fatalf("missing imported node %q; got %v", want, seen)
		}
	}

	created := e2eCreateNodeListeners(t, client, app, nodeIDs)
	if len(created) != expectedNodes {
		t.Fatalf("created listeners = %d, want %d", len(created), expectedNodes)
	}
	e2eWaitRuntimeRunning(t, app)
	e2eAssertRemoteThroughListeners(t, created, targetURL, wantBody)

	// Force a real subscription refresh and ensure the refreshed nodes remain
	// usable instead of only proving the first import path.
	subID := imported.SubscriptionIDs[0]
	if _, err := e2eJSON(client, http.MethodPost, app.adminURL+"/api/subscriptions/"+subID+"/refresh", map[string]any{"force": true}); err != nil {
		t.Fatalf("refresh remote subscription: %v", err)
	}
	e2eWaitRuntimeRunning(t, app)
	refreshed := e2eBootstrapState(t, app)
	if len(refreshed.Nodes) != expectedNodes || len(refreshed.Subscriptions) != 1 {
		t.Fatalf("after refresh nodes=%d subscriptions=%d, want %d/1", len(refreshed.Nodes), len(refreshed.Subscriptions), expectedNodes)
	}
	e2eAssertRemoteThroughListeners(t, created, targetURL, wantBody)

	// Restart the application to prove the imported subscription, sidecar
	// endpoints and listeners survive an ordinary process restart.
	app.kill()
	restarted := e2eStartApp(t, appBinary, dataDir, mihomoBinary, xrayBinary)
	e2eWaitSidecarRunning(t, restarted)
	e2eWaitRuntimeRunning(t, restarted)
	restartedState := e2eBootstrapState(t, restarted)
	if len(restartedState.Nodes) != expectedNodes || len(restartedState.Listeners) != expectedNodes {
		t.Fatalf("after restart nodes=%d listeners=%d, want %d/%d", len(restartedState.Nodes), len(restartedState.Listeners), expectedNodes, expectedNodes)
	}
	e2eAssertRemoteThroughListeners(t, created, targetURL, wantBody)
}

func e2eBootstrapState(t *testing.T, app *e2eApp) model.State {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: nil}}
	data, err := e2eJSON(client, http.MethodGet, app.adminURL+"/api/bootstrap", nil)
	if err != nil {
		t.Fatalf("read bootstrap: %v", err)
	}
	var state model.State
	if err := json.Unmarshal(e2eUnwrapData(t, data), &state); err != nil {
		t.Fatalf("decode bootstrap: %v", err)
	}
	return state
}

func e2eUnwrapData(t *testing.T, raw []byte) []byte {
	t.Helper()
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("decode API envelope: %v", err)
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return raw
	}
	return envelope.Data
}

func e2eCreateNodeListeners(t *testing.T, client *http.Client, app *e2eApp, nodeIDs []string) []model.Listener {
	t.Helper()
	data, err := e2eJSON(client, http.MethodPost, app.adminURL+"/api/listeners/batch", map[string]any{
		"nodeIds": nodeIDs, "protocol": "mixed", "host": "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("create node listeners: %v", err)
	}
	var listeners []model.Listener
	if err := json.Unmarshal(e2eUnwrapData(t, data), &listeners); err != nil {
		t.Fatalf("decode listeners: %v", err)
	}
	return listeners
}

func e2eAssertRemoteThroughListeners(t *testing.T, listeners []model.Listener, targetURL, want string) {
	t.Helper()
	for _, listener := range listeners {
		deadline := time.Now().Add(25 * time.Second)
		var got string
		for {
			got = e2eFetchThroughProxy(t, listener.Port, targetURL)
			if got == want || time.Now().After(deadline) {
				break
			}
			time.Sleep(300 * time.Millisecond)
		}
		if got != want {
			t.Errorf("listener %d: got %q, want %q", listener.Port, got, want)
			continue
		}
		t.Logf("remote target through listener %d: %s", listener.Port, got)
	}
}
