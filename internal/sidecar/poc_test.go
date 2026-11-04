package sidecar

import (
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
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/uniproxy/uniproxy/internal/model"
)

var (
	decryptionLine  = regexp.MustCompile("\"decryption\": \"([^\"]+)\"")
	encryptionLine  = regexp.MustCompile("\"encryption\": \"([^\"]+)\"")
	privateKeyLine  = regexp.MustCompile("(?m)^PrivateKey: (.+)$")
	publicKeyLine   = regexp.MustCompile("(?m)^Password \\(PublicKey\\): (.+)$")
	mldsaSeedLine   = regexp.MustCompile("(?m)^Seed: (.+)$")
	mldsaVerifyLine = regexp.MustCompile("(?m)^Verify: (.+)$")
)

func runXrayHelper(t *testing.T, binary string, args ...string) string {
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

func lastMatch(re *regexp.Regexp, output string) string {
	matches := re.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		return ""
	}
	return strings.TrimSpace(matches[len(matches)-1][1])
}

func reserveLoopbackPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	return port
}

func writeXrayServerConfig(t *testing.T, binary string, config map[string]any) string {
	t.Helper()
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "server.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	testCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := runXrayTest(testCtx, binary, path, nil); err != nil {
		t.Fatalf("server config rejected: %v", err)
	}
	return path
}

func startXrayServer(t *testing.T, binary, configPath string) string {
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

func waitTCP(t *testing.T, port int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 300*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("port %d did not become ready", port)
}

func curlThroughSOCKS(t *testing.T, endpoint model.SidecarEndpoint, targetURL string) (string, error) {
	t.Helper()
	proxyURL := &url.URL{Scheme: "socks5h", User: url.UserPassword(endpoint.Username, endpoint.Password), Host: net.JoinHostPort(endpoint.Host, strconv.Itoa(endpoint.Port))}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	curl := "curl"
	if path, err := exec.LookPath("curl"); err == nil {
		curl = path
	} else if path, err := exec.LookPath("curl.exe"); err == nil {
		curl = path
	}
	cmd := exec.CommandContext(ctx, curl, "--fail", "--silent", "--show-error", "--max-time", "15", "--proxy", proxyURL.String(), targetURL)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("curl through %s failed: %w: %s", endpoint.Username, err, output)
	}
	return strings.TrimSpace(string(output)), nil
}

func startLargeCertificateTLSServer(t *testing.T, handler http.Handler) *httptest.Server {
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

func TestPhase0XrayPOC(t *testing.T) {
	binary := findXrayBinary(t)
	uuid := "11111111-1111-4111-8111-111111111111"
	shortID := "0123456789abcdef"

	vlessOutput := runXrayHelper(t, binary, "vlessenc")
	decryption := lastMatch(decryptionLine, vlessOutput)
	encryption := lastMatch(encryptionLine, vlessOutput)
	if decryption == "" || encryption == "" {
		t.Fatalf("could not parse vlessenc output: %s", vlessOutput)
	}
	x25519Output := runXrayHelper(t, binary, "x25519")
	privateKey := lastMatch(privateKeyLine, x25519Output)
	publicKey := lastMatch(publicKeyLine, x25519Output)
	mldsaOutput := runXrayHelper(t, binary, "mldsa65")
	mldsaSeed := lastMatch(mldsaSeedLine, mldsaOutput)
	mldsaVerify := lastMatch(mldsaVerifyLine, mldsaOutput)
	if privateKey == "" || publicKey == "" || mldsaSeed == "" || mldsaVerify == "" {
		t.Fatal("could not generate Reality keys")
	}

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "phase0-ok")
	}))
	defer origin.Close()
	originURL, _ := url.Parse(origin.URL)
	originPort, _ := strconv.Atoi(originURL.Port())

	realityTarget := startLargeCertificateTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "reality-target")
	}))
	realityURL, _ := url.Parse(realityTarget.URL)
	realityPort, _ := strconv.Atoi(realityURL.Port())

	vlessPort := reserveLoopbackPort(t)
	vmessPort := reserveLoopbackPort(t)
	realityInPort := reserveLoopbackPort(t)

	serverConfig := map[string]any{
		"log": map[string]any{"loglevel": "debug"},
		"inbounds": []any{
			map[string]any{
				"tag": "vless-enc-in", "listen": "127.0.0.1", "port": vlessPort, "protocol": "vless",
				"settings": map[string]any{
					"decryption": decryption,
					"clients":    []any{map[string]any{"id": uuid}},
				},
				"streamSettings": map[string]any{"network": "tcp", "security": "none"},
			},
			map[string]any{
				"tag": "vmess-xhttp-in", "listen": "127.0.0.1", "port": vmessPort, "protocol": "vmess",
				"settings":       map[string]any{"clients": []any{map[string]any{"id": uuid}}},
				"streamSettings": map[string]any{"network": "xhttp", "security": "none", "xhttpSettings": map[string]any{"path": "/xhttp"}},
			},
			map[string]any{
				"tag": "reality-in", "listen": "127.0.0.1", "port": realityInPort, "protocol": "vless",
				"settings": map[string]any{
					"decryption": "none",
					"clients":    []any{map[string]any{"id": uuid, "flow": "xtls-rprx-vision"}},
				},
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
	serverPath := writeXrayServerConfig(t, binary, serverConfig)
	serverLog := startXrayServer(t, binary, serverPath)
	for _, port := range []int{vlessPort, vmessPort, realityInPort} {
		waitTCP(t, port, 10*time.Second)
	}

	clientNodes := []model.Node{
		{
			ID: "vless-enc", Enabled: true, Engine: model.EngineXray,
			ParsedNode: model.ParsedNode{Name: "vless-enc", Protocol: "vless", Server: "127.0.0.1", Port: vlessPort, UDP: true, Config: map[string]any{"xray": map[string]any{
				"protocol": "vless",
				"settings": map[string]any{"vnext": []any{map[string]any{
					"address": "127.0.0.1", "port": vlessPort,
					"users": []any{map[string]any{"id": uuid, "encryption": encryption}},
				}}},
				"streamSettings": map[string]any{"network": "tcp", "security": "none"},
			}}},
		},
		{
			ID: "vmess-xhttp", Enabled: true, Engine: model.EngineXray,
			ParsedNode: model.ParsedNode{Name: "vmess-xhttp", Protocol: "vmess", Server: "127.0.0.1", Port: vmessPort, UDP: true, Config: map[string]any{"xray": map[string]any{
				"protocol": "vmess",
				"settings": map[string]any{"vnext": []any{map[string]any{
					"address": "127.0.0.1", "port": vmessPort,
					"users": []any{map[string]any{"id": uuid, "security": "auto"}},
				}}},
				"streamSettings": map[string]any{"network": "xhttp", "security": "none", "xhttpSettings": map[string]any{"path": "/xhttp"}},
			}}},
		},
		{
			ID: "reality", Enabled: true, Engine: model.EngineXray,
			ParsedNode: model.ParsedNode{Name: "reality", Protocol: "vless", Server: "127.0.0.1", Port: realityInPort, UDP: false, Config: map[string]any{"xray": map[string]any{
				"protocol": "vless",
				"settings": map[string]any{"vnext": []any{map[string]any{
					"address": "127.0.0.1", "port": realityInPort,
					"users": []any{map[string]any{"id": uuid, "encryption": "none", "flow": "xtls-rprx-vision"}},
				}}},
				"streamSettings": map[string]any{"network": "tcp", "security": "reality", "realitySettings": map[string]any{
					"serverName": "example.com", "fingerprint": "chrome", "publicKey": publicKey,
					"shortId": shortID, "spiderX": "/", "mldsa65Verify": mldsaVerify,
				}},
			}}},
		},
	}
	manager, err := NewXrayManager(XrayOptions{BinaryPath: binary, PortRangeStart: 32300, PortRangeEnd: 32399, TestTimeout: 20 * time.Second, StartupTimeout: 20 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	endpoints, err := manager.Start(ctx, clientNodes)
	if err != nil {
		t.Fatalf("client start: %v", err)
	}
	defer func() { _ = manager.Stop(context.Background()) }()
	for _, node := range clientNodes {
		endpoint, ok := endpoints[node.ID]
		if !ok {
			t.Fatalf("missing endpoint for %s", node.ID)
		}
		got, err := curlThroughSOCKS(t, endpoint, fmt.Sprintf("http://127.0.0.1:%d/", originPort))
		if err != nil {
			serverOutput, _ := os.ReadFile(serverLog)
			t.Fatalf("%s: %v\nclient logs:\n%s\nserver logs:\n%s", node.ID, err, manager.Logs(), serverOutput)
		}
		if got != "phase0-ok" {
			t.Fatalf("%s returned %q\nclient logs:\n%s", node.ID, got, manager.Logs())
		}
		t.Logf("%s forwarded successfully through %d", node.ID, endpoint.Port)
	}
}
