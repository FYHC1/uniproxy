package e2e

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/uniproxy/uniproxy/internal/model"
	"github.com/uniproxy/uniproxy/internal/store"
)

var (
	e2eDecryptionLine  = regexp.MustCompile(`"decryption": "([^"]+)"`)
	e2eEncryptionLine  = regexp.MustCompile(`"encryption": "([^"]+)"`)
	e2ePrivateKeyLine  = regexp.MustCompile(`(?m)^PrivateKey: (.+)$`)
	e2ePublicKeyLine   = regexp.MustCompile(`(?m)^Password \(PublicKey\): (.+)$`)
	e2eMldsaSeedLine   = regexp.MustCompile(`(?m)^Seed: (.+)$`)
	e2eMldsaVerifyLine = regexp.MustCompile(`(?m)^Verify: (.+)$`)
)

func e2eRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func e2eRealBinary(t *testing.T, envName string, candidates ...string) string {
	t.Helper()
	if value := strings.TrimSpace(os.Getenv(envName)); value != "" {
		info, err := os.Stat(value)
		if err != nil || info.IsDir() {
			t.Fatalf("%s points to an inaccessible file: %s", envName, value)
		}
		return value
	}
	for _, candidate := range candidates {
		if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(e2eRepoRoot(t), candidate)
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	if path, err := exec.LookPath(strings.TrimSuffix(filepath.Base(candidates[0]), filepath.Ext(candidates[0]))); err == nil {
		return path
	}
	t.Skipf("set %s to a real binary (candidate: %s)", envName, candidates[0])
	return ""
}

func e2eBuildApp(t *testing.T) string {
	t.Helper()
	root := e2eRepoRoot(t)
	out := filepath.Join(t.TempDir(), "uniproxy")
	if runtime.GOOS == "windows" {
		out += ".exe"
	}
	cmd := exec.Command("go", "build", "-trimpath", "-o", out, "./cmd/uniproxy")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build uniproxy: %v\n%s", err, output)
	}
	return out
}

func e2eReservePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	return port
}

func e2eRunXray(t *testing.T, binary string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("xray %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}

func e2eLastMatch(re *regexp.Regexp, output string) string {
	matches := re.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		return ""
	}
	return strings.TrimSpace(matches[len(matches)-1][1])
}

func e2eWriteXrayServerConfig(t *testing.T, binary string, config map[string]any) string {
	t.Helper()
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "server.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func e2eStartXrayServer(t *testing.T, binary, configPath string) string {
	t.Helper()
	logPath := filepath.Join(t.TempDir(), "server.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "run", "-c", configPath)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = logFile.Close()
	})
	return logPath
}

func e2eWaitTCP(t *testing.T, port int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 250*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("port %d did not become ready", port)
}

func e2eWaitPortClosed(t *testing.T, port int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 250*time.Millisecond)
		if err != nil {
			return
		}
		_ = conn.Close()
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("port %d is still accepting connections", port)
}

func e2eStartLargeCertificateTLSServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "example.com"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"example.com"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		ExtraExtensions: []pkix.Extension{{
			Id:    []int{1, 3, 6, 1, 4, 1, 99999, 1},
			Value: make([]byte, 8192),
		}},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
	server.StartTLS()
	t.Cleanup(server.Close)
	return server
}

type e2eApp struct {
	adminURL string
	cmd      *exec.Cmd
	logFile  *os.File
	logPath  string
	done     bool
}

func e2eStartApp(t *testing.T, binary, dataDir, corePath, xrayPath string) *e2eApp {
	t.Helper()
	port := e2eReservePort(t)
	logPath := filepath.Join(t.TempDir(), "app.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "-host", "127.0.0.1", "-port", strconv.Itoa(port), "-data", dataDir, "-core", corePath, "-xray", xrayPath)
	cmd.Dir = e2eRepoRoot(t)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		t.Fatal(err)
	}
	app := &e2eApp{adminURL: "http://127.0.0.1:" + strconv.Itoa(port), cmd: cmd, logFile: logFile, logPath: logPath}
	t.Cleanup(func() { app.kill() })
	app.waitHealth(t, 30*time.Second)
	return app
}

func (a *e2eApp) kill() {
	if a == nil || a.done {
		return
	}
	a.done = true
	_ = a.cmd.Process.Kill()
	_ = a.cmd.Wait()
	if a.logFile != nil {
		_ = a.logFile.Close()
		a.logFile = nil
	}
}

func (a *e2eApp) waitHealth(t *testing.T, timeout time.Duration) {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{Proxy: nil}}
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		resp, err := client.Get(a.adminURL + "/api/health")
		if err == nil {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
			last = fmt.Errorf("health status %d: %s", resp.StatusCode, body)
		} else {
			last = err
		}
		time.Sleep(150 * time.Millisecond)
	}
	logs, _ := os.ReadFile(a.logPath)
	t.Fatalf("application did not become healthy: %v\n%s", last, logs)
}

func e2eJSON(client *http.Client, method, target string, body any) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, target, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s %s: status %d: %s", method, target, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return data, nil
}

func e2eWaitFor(t *testing.T, timeout time.Duration, description string, fn func() error) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		if err := fn(); err == nil {
			return
		} else {
			last = err
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatalf("%s: %v", description, last)
}

type e2eSidecarInfo struct {
	Enabled        bool                `json:"enabled"`
	Status         model.SidecarStatus `json:"status"`
	NodeCount      int                 `json:"nodeCount"`
	PortRangeStart int                 `json:"portRangeStart"`
	PortRangeEnd   int                 `json:"portRangeEnd"`
	MaxNodes       int                 `json:"maxNodes"`
}

type e2eRuntime struct {
	Running   bool   `json:"running"`
	Applying  bool   `json:"applying"`
	Dirty     bool   `json:"dirty"`
	LastError string `json:"lastError"`
}

func e2eWaitSidecarRunning(t *testing.T, app *e2eApp) e2eSidecarInfo {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{Proxy: nil}}
	var info e2eSidecarInfo
	e2eWaitFor(t, 30*time.Second, "sidecar running", func() error {
		data, err := e2eJSON(client, http.MethodGet, app.adminURL+"/api/sidecars", nil)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(data, &info); err != nil {
			return err
		}
		if !info.Enabled {
			return fmt.Errorf("sidecar feature is disabled")
		}
		if !info.Status.Running {
			return fmt.Errorf("sidecar status: %+v", info.Status)
		}
		if info.Status.Error != "" {
			return fmt.Errorf("sidecar error: %s", info.Status.Error)
		}
		return nil
	})
	return info
}

func e2eWaitRuntimeRunning(t *testing.T, app *e2eApp) e2eRuntime {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{Proxy: nil}}
	var runtimeStatus e2eRuntime
	e2eWaitFor(t, 30*time.Second, "core runtime running", func() error {
		data, err := e2eJSON(client, http.MethodGet, app.adminURL+"/api/runtime", nil)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(data, &runtimeStatus); err != nil {
			return err
		}
		if !runtimeStatus.Running || runtimeStatus.Applying {
			return fmt.Errorf("runtime: %+v", runtimeStatus)
		}
		if runtimeStatus.LastError != "" {
			return fmt.Errorf("runtime error: %s", runtimeStatus.LastError)
		}
		return nil
	})
	return runtimeStatus
}

func e2eSeedState(t *testing.T, dataDir string, nodes []model.Node, listeners []model.Listener) {
	t.Helper()
	s, err := store.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	state := s.Snapshot()
	state.Nodes = nodes
	state.Listeners = listeners
	state.Settings.EnableSidecar = true
	state.Settings.SidecarPortRangeStart = 32400
	state.Settings.SidecarPortRangeEnd = 32499
	state.Revision++
	if err := s.Save(state); err != nil {
		t.Fatal(err)
	}
}

func e2eFetchThroughProxy(t *testing.T, proxyPort int, targetURL string) string {
	t.Helper()
	proxyURL := &url.URL{Scheme: "http", Host: net.JoinHostPort("127.0.0.1", strconv.Itoa(proxyPort))}
	client := &http.Client{
		Timeout: 20 * time.Second,
		Transport: &http.Transport{
			Proxy:                 http.ProxyURL(proxyURL),
			DisableKeepAlives:     true,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
		},
	}
	resp, err := client.Get(targetURL)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	return strings.TrimSpace(string(body))
}

func TestRealAppProcessThreeProtocols(t *testing.T) {
	if os.Getenv("UNIPROXY_E2E_REAL") != "1" {
		t.Skip("set UNIPROXY_E2E_REAL=1 to run the real Xray/Mihomo application-process test")
	}
	xrayBinary := e2eRealBinary(t, "UNIPROXY_E2E_XRAY", filepath.Join("..", "uniproxy", ".local", "xray-test", "xray.exe"), ".local/real-test-linux/xray")
	mihomoBinary := e2eRealBinary(t, "UNIPROXY_E2E_MIHOMO", filepath.Join("..", "uniproxy", "bin", "mihomo.exe"), ".local/real-test-linux/mihomo")
	appBinary := e2eBuildApp(t)

	uuid := "11111111-1111-4111-8111-111111111111"
	shortID := "0123456789abcdef"
	vlessOutput := e2eRunXray(t, xrayBinary, "vlessenc")
	decryption := e2eLastMatch(e2eDecryptionLine, vlessOutput)
	encryption := e2eLastMatch(e2eEncryptionLine, vlessOutput)
	x25519Output := e2eRunXray(t, xrayBinary, "x25519")
	privateKey := e2eLastMatch(e2ePrivateKeyLine, x25519Output)
	publicKey := e2eLastMatch(e2ePublicKeyLine, x25519Output)
	mldsaOutput := e2eRunXray(t, xrayBinary, "mldsa65")
	mldsaSeed := e2eLastMatch(e2eMldsaSeedLine, mldsaOutput)
	mldsaVerify := e2eLastMatch(e2eMldsaVerifyLine, mldsaOutput)
	if decryption == "" || encryption == "" || privateKey == "" || publicKey == "" || mldsaSeed == "" || mldsaVerify == "" {
		t.Fatal("could not generate VLESS Encryption or Reality keys")
	}

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "real-e2e-ok")
	}))
	defer origin.Close()
	originURL, _ := url.Parse(origin.URL)
	originPort, _ := strconv.Atoi(originURL.Port())

	realityTarget := e2eStartLargeCertificateTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "reality-target")
	}))
	realityURL, _ := url.Parse(realityTarget.URL)
	realityPort, _ := strconv.Atoi(realityURL.Port())

	vlessPort := e2eReservePort(t)
	vmessPort := e2eReservePort(t)
	realityInPort := e2eReservePort(t)
	serverConfig := map[string]any{
		"log": map[string]any{"loglevel": "debug"},
		"inbounds": []any{
			map[string]any{
				"tag": "vless-enc-in", "listen": "127.0.0.1", "port": vlessPort, "protocol": "vless",
				"settings":       map[string]any{"decryption": decryption, "clients": []any{map[string]any{"id": uuid}}},
				"streamSettings": map[string]any{"network": "tcp", "security": "none"},
			},
			map[string]any{
				"tag": "vmess-xhttp-in", "listen": "127.0.0.1", "port": vmessPort, "protocol": "vmess",
				"settings":       map[string]any{"clients": []any{map[string]any{"id": uuid}}},
				"streamSettings": map[string]any{"network": "xhttp", "security": "none", "xhttpSettings": map[string]any{"path": "/xhttp"}},
			},
			map[string]any{
				"tag": "reality-in", "listen": "127.0.0.1", "port": realityInPort, "protocol": "vless",
				"settings": map[string]any{"decryption": "none", "clients": []any{map[string]any{"id": uuid, "flow": "xtls-rprx-vision"}}},
				"streamSettings": map[string]any{
					"network": "tcp", "security": "reality",
					"realitySettings": map[string]any{
						"dest": fmt.Sprintf("127.0.0.1:%d", realityPort), "xver": 0,
						"serverNames": []string{"example.com"}, "privateKey": privateKey,
						"shortIds": []string{shortID}, "mldsa65Seed": mldsaSeed,
					},
				},
			},
		},
		"outbounds": []any{map[string]any{"protocol": "freedom", "tag": "direct", "settings": map[string]any{"finalRules": []any{map[string]any{"action": "allow", "ip": []string{"127.0.0.1/32"}}}}}},
	}
	serverPath := e2eWriteXrayServerConfig(t, xrayBinary, serverConfig)
	serverLog := e2eStartXrayServer(t, xrayBinary, serverPath)
	for _, port := range []int{vlessPort, vmessPort, realityInPort} {
		e2eWaitTCP(t, port, 15*time.Second)
	}

	nodes := []model.Node{
		{ID: "vless-enc", Enabled: true, Engine: model.EngineXray, ParsedNode: model.ParsedNode{Name: "vless-enc", Protocol: "vless", Server: "127.0.0.1", Port: vlessPort, UDP: true, Config: map[string]any{"xray": map[string]any{
			"protocol":       "vless",
			"settings":       map[string]any{"vnext": []any{map[string]any{"address": "127.0.0.1", "port": vlessPort, "users": []any{map[string]any{"id": uuid, "encryption": encryption}}}}},
			"streamSettings": map[string]any{"network": "tcp", "security": "none"},
		}}}},
		{ID: "vmess-xhttp", Enabled: true, Engine: model.EngineXray, ParsedNode: model.ParsedNode{Name: "vmess-xhttp", Protocol: "vmess", Server: "127.0.0.1", Port: vmessPort, UDP: true, Config: map[string]any{"xray": map[string]any{
			"protocol":       "vmess",
			"settings":       map[string]any{"vnext": []any{map[string]any{"address": "127.0.0.1", "port": vmessPort, "users": []any{map[string]any{"id": uuid, "security": "auto"}}}}},
			"streamSettings": map[string]any{"network": "xhttp", "security": "none", "xhttpSettings": map[string]any{"path": "/xhttp"}},
		}}}},
		{ID: "reality", Enabled: true, Engine: model.EngineXray, ParsedNode: model.ParsedNode{Name: "reality", Protocol: "vless", Server: "127.0.0.1", Port: realityInPort, UDP: false, Config: map[string]any{"xray": map[string]any{
			"protocol":       "vless",
			"settings":       map[string]any{"vnext": []any{map[string]any{"address": "127.0.0.1", "port": realityInPort, "users": []any{map[string]any{"id": uuid, "encryption": "none", "flow": "xtls-rprx-vision"}}}}},
			"streamSettings": map[string]any{"network": "tcp", "security": "reality", "realitySettings": map[string]any{"serverName": "example.com", "fingerprint": "chrome", "publicKey": publicKey, "shortId": shortID, "spiderX": "/", "mldsa65Verify": mldsaVerify}},
		}}}},
	}
	listenerPorts := []int{e2eReservePort(t), e2eReservePort(t), e2eReservePort(t)}
	listeners := make([]model.Listener, 0, len(nodes))
	for i, node := range nodes {
		listeners = append(listeners, model.Listener{ID: "listener-" + node.ID, Name: "listener-" + node.ID, Kind: "node", TargetID: node.ID, Protocol: "mixed", Host: "127.0.0.1", Port: listenerPorts[i], Enabled: true})
	}
	dataDir := t.TempDir()
	e2eSeedState(t, dataDir, nodes, listeners)

	app := e2eStartApp(t, appBinary, dataDir, mihomoBinary, xrayBinary)
	info := e2eWaitSidecarRunning(t, app)
	if info.NodeCount != len(nodes) {
		t.Fatalf("sidecar nodeCount = %d, want %d", info.NodeCount, len(nodes))
	}
	if info.PortRangeStart != 32400 || info.PortRangeEnd != 32499 || info.MaxNodes != 200 {
		t.Fatalf("sidecar info = %+v", info)
	}
	runtimeStatus := e2eWaitRuntimeRunning(t, app)
	if runtimeStatus.Dirty {
		t.Fatalf("runtime should be clean after startup: %+v", runtimeStatus)
	}
	for i, node := range nodes {
		e2eWaitTCP(t, listenerPorts[i], 15*time.Second)
		target := fmt.Sprintf("http://127.0.0.1:%d/", originPort)
		var got string
		e2eWaitFor(t, 20*time.Second, node.ID+" through Mihomo", func() error {
			got = e2eFetchThroughProxy(t, listenerPorts[i], target)
			if got != "real-e2e-ok" {
				return fmt.Errorf("got %q", got)
			}
			return nil
		})
		t.Logf("%s through application process: %s", node.ID, got)
	}

	client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{Proxy: nil}}
	if _, err := e2eJSON(client, http.MethodPost, app.adminURL+"/api/runtime/stop", map[string]any{}); err != nil {
		t.Fatalf("stop runtime: %v", err)
	}
	e2eWaitFor(t, 15*time.Second, "sidecar stopped", func() error {
		data, err := e2eJSON(client, http.MethodGet, app.adminURL+"/api/sidecars", nil)
		if err != nil {
			return err
		}
		var stopped e2eSidecarInfo
		if err := json.Unmarshal(data, &stopped); err != nil {
			return err
		}
		if stopped.Status.Running {
			return fmt.Errorf("still running: %+v", stopped.Status)
		}
		return nil
	})
	app.kill()

	restarted := e2eStartApp(t, appBinary, dataDir, mihomoBinary, xrayBinary)
	e2eWaitSidecarRunning(t, restarted)
	e2eWaitRuntimeRunning(t, restarted)
	e2eWaitTCP(t, listenerPorts[0], 15*time.Second)
	got := ""
	e2eWaitFor(t, 20*time.Second, "restart through Mihomo", func() error {
		got = e2eFetchThroughProxy(t, listenerPorts[0], fmt.Sprintf("http://127.0.0.1:%d/", originPort))
		if got != "real-e2e-ok" {
			return fmt.Errorf("got %q", got)
		}
		return nil
	})
	t.Logf("restart through application process: %s", got)

	data, err := e2eJSON(client, http.MethodGet, restarted.adminURL+"/api/bootstrap", nil)
	if err != nil {
		t.Fatalf("read bootstrap before abrupt kill: %v", err)
	}
	var bootstrapped struct {
		Nodes []model.Node `json:"nodes"`
	}
	if err := json.Unmarshal(data, &bootstrapped); err != nil {
		t.Fatalf("decode bootstrap: %v", err)
	}
	ownedPorts := []int{listenerPorts[0]}
	for _, node := range bootstrapped.Nodes {
		if node.SidecarEndpoint != nil {
			ownedPorts = append(ownedPorts, node.SidecarEndpoint.Port)
		}
	}
	restarted.kill()
	for _, port := range ownedPorts {
		e2eWaitPortClosed(t, port, 20*time.Second)
	}
	t.Logf("abrupt application exit released %d owned ports", len(ownedPorts))
	_ = serverLog
}
