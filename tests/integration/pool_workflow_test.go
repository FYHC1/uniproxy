package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uniproxy/uniproxy/internal/app"
	"github.com/uniproxy/uniproxy/internal/core"
	"github.com/uniproxy/uniproxy/internal/model"
	"github.com/uniproxy/uniproxy/internal/store"
)

// This test uses only a temporary database and loopback upstream/target. No user subscriptions or remote probes.
func TestRealPoolAtomicListenerRemoveReinclude(t *testing.T) {
	binary := os.Getenv("UNIPROXY_TEST_CORE_PATH")
	if binary == "" {
		t.Skip("set UNIPROXY_TEST_CORE_PATH for real pool workflow")
	}
	var forwards atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Millisecond) // native latency endpoint rejects a rounded 0ms
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwards.Add(1)
		if r.Method == http.MethodConnect {
			dest, err := net.DialTimeout("tcp", r.Host, 3*time.Second)
			if err != nil {
				http.Error(w, "fixture connect", 502)
				return
			}
			client, buffer, err := w.(http.Hijacker).Hijack()
			if err != nil {
				dest.Close()
				return
			}
			_, _ = buffer.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
			_ = buffer.Flush()
			go func() {
				defer client.Close()
				defer dest.Close()
				go func() { _, _ = io.Copy(dest, buffer) }()
				_, _ = io.Copy(client, dest)
			}()
			return
		}
		clone := r.Clone(r.Context())
		clone.RequestURI = ""
		clone.Header.Del("Proxy-Authorization")
		transport := &http.Transport{Proxy: nil}
		defer transport.CloseIdleConnections()
		result, err := transport.RoundTrip(clone)
		if err != nil {
			http.Error(w, "fixture forward", 502)
			return
		}
		defer result.Body.Close()
		w.WriteHeader(result.StatusCode)
		_, _ = io.Copy(w, result.Body)
	}))
	defer upstream.Close()
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	manager := core.New(core.Options{BinaryPath: binary, DataDir: t.TempDir()})
	application := app.New(s, manager, app.Options{Host: "127.0.0.1", AdminPort: 19091})
	application.Start()
	defer application.Close()
	call := func(method, path string, body any) map[string]any {
		t.Helper()
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, "http://127.0.0.1:19091"+path, bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		application.Handler().ServeHTTP(w, req)
		if w.Code < 200 || w.Code >= 300 {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		var out map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	call("PATCH", "/api/settings", map[string]any{"testUrl": target.URL, "healthIntervalSeconds": 86400})
	call("POST", "/api/import", map[string]any{"mode": "text", "content": upstream.URL + "#Pool-Local-Fixture", "groupIds": []string{}, "groupNames": []string{"Pool-Local-Group"}})
	snapshot := s.Snapshot()
	nodeID := snapshot.Nodes[0].ID
	groupID := snapshot.Groups[0].ID
	// Fix the auto-allocation range to an OS-chosen free TCP port, avoiding user configured ranges.
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := probe.Addr().(*net.TCPAddr).Port
	probe.Close()
	call("PATCH", "/api/settings", map[string]any{"portRangeStart": port, "portRangeEnd": port})
	revision := s.Snapshot().Revision
	created := call("POST", "/api/pools/with-listener", map[string]any{
		"pool":     map[string]any{"name": "Local atomic pool", "strategy": "round-robin", "groupIds": []string{groupID}, "testUrl": target.URL, "intervalSeconds": 86400},
		"listener": map[string]any{"users": []map[string]string{{"username": "fixture", "password": "fixture-only-secret"}}},
	})
	result := created["data"].(map[string]any)
	poolID := result["pool"].(map[string]any)["id"].(string)
	listener := result["listener"].(map[string]any)
	listenerID := listener["id"].(string)
	if s.Snapshot().Revision != revision+1 || len(s.Snapshot().Pools) != 1 || len(s.Snapshot().Listeners) != 1 {
		t.Fatal("combined create not one complete revision")
	}
	if listener["targetId"] != poolID || int(listener["port"].(float64)) != port {
		t.Fatal("wrong binding/auto port")
	}
	if created["runtime"].(map[string]any)["dirty"] != false {
		t.Fatal("combined runtime pending", created["runtime"])
	}
	public, _ := json.Marshal(created)
	if bytes.Contains(public, []byte("fixture-only-secret")) {
		t.Fatal("credential leaked")
	}
	proxyURL, _ := url.Parse("http://127.0.0.1:" + strconv.Itoa(port))
	proxyURL.User = url.UserPassword("fixture", "fixture-only-secret")
	transport := &http.Transport{Proxy: http.ProxyURL(proxyURL), DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	succeeds := func() bool {
		t.Helper()
		response, err := client.Get(target.URL)
		if err != nil {
			return false
		}
		defer response.Body.Close()
		return response.StatusCode == http.StatusNoContent
	}
	if !succeeds() || forwards.Load() == 0 {
		t.Fatal("atomic pool listener did not proxy actual traffic")
	}
	started := call("POST", "/api/tests", map[string]any{"nodeIds": []string{nodeID}, "kind": "latency"})
	jobID := started["id"].(string)
	deadline := time.Now().Add(8 * time.Second)
	for {
		status := call("GET", "/api/jobs/"+jobID, nil)
		if status["status"] == "completed" {
			results := status["results"].([]any)
			if len(results) != 1 || results[0].(map[string]any)["ok"] != true || results[0].(map[string]any)["latencyMs"] == nil {
				t.Fatal("real latency failed", status)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("latency job did not finish")
		}
		time.Sleep(15 * time.Millisecond)
	}
	removed := call("DELETE", "/api/pools/"+poolID+"/members/"+nodeID, nil)
	if removed["runtime"].(map[string]any)["dirty"] != false {
		t.Fatal("remove not applied")
	}
	after := s.Snapshot()
	if len(after.Nodes) != 1 || len(model.PoolMembers(after, after.Pools[0])) != 0 || after.Listeners[0].ID != listenerID || !after.Listeners[0].Enabled {
		t.Fatal("pool removal damaged global node/listener or left member")
	}
	before := forwards.Load()
	if succeeds() {
		t.Fatal("empty pool leaked a new connection")
	}
	if forwards.Load() != before {
		t.Fatal("empty pool still forwarded")
	}
	pool := after.Pools[0]
	pool.ExcludeNodeIDs = []string{}
	call("PUT", "/api/pools/"+poolID, pool)
	if !succeeds() {
		t.Fatal("reinclude failed to restore the same pool listener")
	}
	t.Log("one-revision pool+listener creation, authenticated local proxy, real delay task, empty-pool refusal and same-port reinclude verified")
}
