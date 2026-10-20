package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/uniproxy/uniproxy/internal/model"
)

// Verify the staged executable, not just the source package. The explicit absent
// core and temporary data/cache make preview/commit non-networking operations.
func TestBuiltSubscriptionCompatibility(t *testing.T) {
	binary := os.Getenv("UNIPROXY_TEST_APP_PATH")
	if binary == "" {
		t.Skip("set UNIPROXY_TEST_APP_PATH for compiled parser validation")
	}
	root := t.TempDir()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
	_ = ln.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "-host", "127.0.0.1", "-port", port, "-data", filepath.Join(root, "data"), "-core", filepath.Join(root, "intentionally-absent-core"))
	cmd.Dir = root
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	cmd.WaitDelay = time.Second
	for _, value := range os.Environ() {
		u := strings.ToUpper(value)
		if strings.HasPrefix(u, "UNIPROXY_") || strings.HasPrefix(u, "LOCALAPPDATA=") || strings.HasPrefix(u, "XDG_CACHE_HOME=") {
			continue
		}
		cmd.Env = append(cmd.Env, value)
	}
	cmd.Env = append(cmd.Env, "LOCALAPPDATA="+root, "XDG_CACHE_HOME="+root)
	if err := cmd.Start(); err != nil {
		t.Fatal("cannot start isolated compiled service")
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	defer func() { cancel(); <-done }()
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	base := "http://127.0.0.1:" + port
	deadline := time.NewTimer(12 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	ready := false
	for !ready {
		select {
		case <-deadline.C:
			t.Fatal("compiled service not ready")
		case <-ctx.Done():
			t.Fatal("compiled service startup expired")
		case <-ticker.C:
			res, e := client.Get(base + "/api/health")
			if e == nil {
				_ = res.Body.Close()
				ready = res.StatusCode == 200
			}
		}
	}
	request := func(method, path string, payload any) (int, []byte) {
		t.Helper()
		var raw []byte
		if payload != nil {
			raw, _ = json.Marshal(payload)
		}
		req, e := http.NewRequestWithContext(ctx, method, base+path, bytes.NewReader(raw))
		if e != nil {
			t.Fatal(e)
		}
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		res, e := client.Do(req)
		if e != nil {
			t.Fatal("compiled service API unavailable")
		}
		defer res.Body.Close()
		body, e := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		if e != nil {
			t.Fatal("compiled service API body unavailable")
		}
		return res.StatusCode, body
	}
	status, before := request("GET", "/api/bootstrap", nil)
	if status != 200 {
		t.Fatal("compiled bootstrap unavailable")
	}
	const uuid = "11111111-1111-4111-8111-111111111111"
	inputs := []string{
		"proxies:\n - {name: plain, type: vmess, server: fixture.example.invalid, port: 443, uuid: " + uuid + ", tls: false, skip-cert-verify: false}",
		"[server_local]\ntrojan=fixture.example.invalid:443,password=synthetic-binary-canary,tls-host=front.example.invalid,tls-verification=true,tag=fixture",
		"[Proxy]\nfixture = trojan, fixture.example.invalid, 443, password=synthetic-binary-canary,sni=front.example.invalid,skip-cert-verify=false\n[Rule]\nFINAL,DIRECT",
		"[Proxy]\nfixture = trojan, fixture.example.invalid, 443, \"synthetic-binary-canary\",tls-name=front.example.invalid,skip-cert-verify=false",
		"STATUS=Remaining: 10 GB\ntrojan://synthetic-binary-canary@fixture.example.invalid:443?sni=front.example.invalid&peer=front.example.invalid",
		"? proxies\n:\n - {name: 'n=trojan,example.invalid,443,password=p', type: http, server: fixture.example.invalid, port: 80}\n",
	}
	for _, input := range inputs {
		code, raw := request("POST", "/api/import", map[string]any{"mode": "text", "content": input, "preview": true})
		var preview model.ParseResult
		if code != 200 || json.Unmarshal(raw, &preview) != nil || preview.TotalErrors != 0 || len(preview.Nodes) != 1 {
			t.Fatal("compiled parser did not preview a supported source")
		}
		if preview.Nodes[0].Config != nil || bytes.Contains(raw, []byte("synthetic-binary-canary")) || bytes.Contains(raw, []byte(uuid)) {
			t.Fatal("compiled preview leaked credentials")
		}
	}
	_, after := request("GET", "/api/bootstrap", nil)
	var beforeState, afterState struct {
		Revision      int64
		Nodes         []model.Node
		Subscriptions []model.Subscription
	}
	if json.Unmarshal(before, &beforeState) != nil || json.Unmarshal(after, &afterState) != nil || beforeState.Revision != afterState.Revision || len(afterState.Nodes) != 0 || len(afterState.Subscriptions) != 0 {
		t.Fatal("compiled previews saved state")
	}
	code, raw := request("POST", "/api/import", map[string]any{"mode": "text", "content": inputs[0], "preview": false})
	if code != 200 {
		t.Fatal("compiled TLS-default import failed")
	}
	if bytes.Contains(raw, []byte(uuid)) {
		t.Fatal("compiled commit leaked credentials")
	}
	_, raw = request("GET", "/api/bootstrap", nil)
	var saved struct {
		Nodes   []model.Node
		Runtime model.RuntimeStatus
	}
	if json.Unmarshal(raw, &saved) != nil || len(saved.Nodes) != 1 || saved.Runtime.Running || saved.Nodes[0].Config != nil {
		t.Fatal("compiled import state or absent-core semantics changed")
	}
	for _, input := range []string{
		"proxies:\n - {name: unsafe, type: vmess, server: fixture.example.invalid, port: 443, uuid: " + uuid + ", tls: false, skip-cert-verify: true}",
		inputs[2] + "\n[Proxy Group]\nexternal = select, policy-path=https://provider.example.invalid/nodes",
		"STATUS=note: |\n  trojan://synthetic-binary-canary@fixture.example.invalid:443\n",
		"fixture = vmess,fixture.example.invalid,443,username=" + uuid + ",vmess-aead=true,tls=true,ws=true,ws-headers=Host:other.example.invalid",
	} {
		code, raw := request("POST", "/api/import", map[string]any{"mode": "text", "content": input, "preview": true})
		var preview model.ParseResult
		if code != 200 || json.Unmarshal(raw, &preview) != nil || preview.TotalErrors == 0 {
			t.Fatal("compiled parser accepted a meaningful security/provider conflict")
		}
	}
}
