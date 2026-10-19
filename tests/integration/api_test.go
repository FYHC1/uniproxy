package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/uniproxy/uniproxy/internal/app"
	"github.com/uniproxy/uniproxy/internal/core"
	"github.com/uniproxy/uniproxy/internal/store"
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
)

func TestRealAPIImportApplyProxyProbeAndDisable(t *testing.T) {
	binary := os.Getenv("UNIPROXY_TEST_CORE_PATH")
	if binary == "" {
		t.Skip("set UNIPROXY_TEST_CORE_PATH for real end-to-end test")
	}
	var forwarded atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			// Native delay API treats a rounded 0ms loopback result as HTTP503.
			// Real fixture delay keeps the measurement nonzero, not a stubbed metric.
			time.Sleep(5 * time.Millisecond)
			w.WriteHeader(204)
			return
		}
		w.Write(bytes.Repeat([]byte("x"), 128*1024))
	}))
	defer target.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwarded.Add(1)
		if r.Method == http.MethodConnect {
			dest, e := net.DialTimeout("tcp", r.Host, 3*time.Second)
			if e != nil {
				http.Error(w, "connect", 502)
				return
			}
			client, buffer, e := w.(http.Hijacker).Hijack()
			if e != nil {
				dest.Close()
				return
			}
			_, _ = buffer.WriteString("HTTP/1.1 200 Connection Established" + string([]byte{13, 10, 13, 10}))
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
		res, e := transport.RoundTrip(clone)
		if e != nil {
			http.Error(w, "forward", 502)
			return
		}
		defer res.Body.Close()
		for k, v := range res.Header {
			w.Header()[k] = v
		}
		w.WriteHeader(res.StatusCode)
		_, _ = io.Copy(w, res.Body)
	}))
	defer upstream.Close()
	s, e := store.Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	manager := core.New(core.Options{BinaryPath: binary, DataDir: t.TempDir()})
	application := app.New(s, manager, app.Options{Host: "127.0.0.1", AdminPort: 9090})
	application.Start()
	defer application.Close()
	call := func(method, path string, body any) map[string]any {
		t.Helper()
		data, _ := json.Marshal(body)
		req := httptest.NewRequest(method, "http://127.0.0.1:9090"+path, bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		application.Handler().ServeHTTP(w, req)
		if w.Code < 200 || w.Code >= 300 {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		var result map[string]any
		if e := json.Unmarshal(w.Body.Bytes(), &result); e != nil {
			t.Fatal(e)
		}
		return result
	}
	imported := call("POST", "/api/import", map[string]any{"mode": "text", "content": upstream.URL + "#Local-Fixture", "groupIds": []string{}})
	if imported["runtime"].(map[string]any)["dirty"] != false {
		t.Fatalf("not automatically applied: %#v", imported["runtime"])
	}
	nodeID := s.Snapshot().Nodes[0].ID
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	applied := call("POST", "/api/listeners", map[string]any{"name": "integration", "kind": "node", "targetId": nodeID, "protocol": "mixed", "host": "127.0.0.1", "port": port, "udp": false, "enabled": true, "users": []any{}})
	if applied["runtime"].(map[string]any)["dirty"] != false {
		t.Fatalf("listener pending: %#v", applied["runtime"])
	}
	proxyURL, _ := url.Parse("http://127.0.0.1:" + strconv.Itoa(port))
	transport := &http.Transport{Proxy: http.ProxyURL(proxyURL), DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	res, e := client.Get(target.URL + "/health")
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if res.StatusCode != 204 || forwarded.Load() == 0 {
		t.Fatal("no real upstream forwarding")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	check, e := manager.Check(ctx, nodeID, target.URL+"/health", 5*time.Second)
	if e != nil || !check.OK || check.LatencyMS == nil {
		t.Fatalf("real URL test: %+v %v", check, e)
	}
	speed, e := manager.Speed(ctx, nodeID, target.URL+"/download", 5*time.Second, 64*1024)
	if e != nil || !speed.OK || speed.Bytes != 64*1024 || speed.SpeedMbps == nil || *speed.SpeedMbps <= 0 {
		t.Fatalf("real speed: %+v %v", speed, e)
	}
	before := forwarded.Load()
	disabled := call("POST", "/api/nodes/batch", map[string]any{"ids": []string{nodeID}, "action": "disable"})
	if disabled["runtime"].(map[string]any)["dirty"] != false {
		t.Fatal(disabled)
	}
	res, e = client.Get(target.URL + "/health")
	if e == nil {
		res.Body.Close()
		if res.StatusCode == 204 {
			t.Fatal("disabled target leaked traffic")
		}
	}
	if forwarded.Load() != before {
		t.Fatal("disabled node was still forwarded")
	}
	t.Log(fmt.Sprintf("real HTTP/mixed chain, URLTest, %d-byte speed measurement and fail-closed disable verified", speed.Bytes))
}
