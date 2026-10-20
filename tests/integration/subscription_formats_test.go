package integration

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/uniproxy/uniproxy/internal/core"
	"github.com/uniproxy/uniproxy/internal/importer"
	"github.com/uniproxy/uniproxy/internal/model"
)

// validationState never adopts source globals, listeners, tests or schedules.
// A graph-aware in-memory state is sufficient for a core -t subprocess.
func subscriptionValidationState(t *testing.T, result model.ParseResult) model.State {
	t.Helper()
	state := model.DefaultState()
	ids := map[string]string{}
	for i, node := range result.Nodes {
		fp := importer.Fingerprint(node)
		if fp == "" {
			t.Fatal("invalid imported identity")
		}
		if ids[fp] == "" {
			ids[fp] = fmt.Sprintf("fixture-%d", i)
		}
	}
	seen := map[string]bool{}
	for _, node := range result.Nodes {
		fp := importer.Fingerprint(node)
		if seen[fp] {
			continue
		}
		seen[fp] = true
		dialerID := ""
		if node.DialerFingerprint != "" {
			dialerID = ids[node.DialerFingerprint]
			if dialerID == "" {
				t.Fatal("missing normalized relay")
			}
		}
		node.DialerProxy = ""
		node.DialerFingerprint = ""
		state.Nodes = append(state.Nodes, model.Node{ID: ids[fp], ParsedNode: node, Fingerprint: fp, DialerID: dialerID, Enabled: true})
	}
	if err := model.ValidateRelayGraph(state.Nodes); err != nil {
		t.Fatal("invalid normalized relay graph")
	}
	return state
}

func validateSubscriptionOffline(t *testing.T, result model.ParseResult, binary string) {
	t.Helper()
	state := subscriptionValidationState(t, result)
	manager := core.New(core.Options{BinaryPath: binary, DataDir: t.TempDir()})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := manager.Validate(ctx, state); err != nil {
		t.Fatal("offline generated core configuration was rejected; raw output withheld")
	}
	if manager.Status().Running {
		t.Fatal("offline validation started a persistent core")
	}
}

func TestStaticSubscriptionFormatsPassPinnedCoreValidation(t *testing.T) {
	binary := os.Getenv("UNIPROXY_TEST_CORE_PATH")
	if binary == "" {
		t.Skip("set UNIPROXY_TEST_CORE_PATH for offline pinned-core validation")
	}
	const id = "11111111-1111-4111-8111-111111111111"
	cases := []struct {
		name, body string
		nodes      int
	}{
		{"clash-plaintext-defaults", "proxies:\n - {name: plain, type: vmess, server: example.invalid, port: 443, uuid: " + id + ", tls: false, skip-cert-verify: false}\n - {name: secure, type: vmess, server: example.invalid, port: 444, uuid: " + id + ", tls: true, skip-cert-verify: false}\n", 2},
		{"qx-trojan", "[server_local]\ntrojan=example.invalid:443,password=synthetic-only,over-tls=true,tls-host=front.example.invalid,tls-verification=true,fast-open=false,tag=fixture\n", 1},
		{"surge-trojan", "[Proxy]\nDIRECT = direct\nfixture = trojan, example.invalid, 443, password=synthetic-only,sni=front.example.invalid,skip-cert-verify=false,tfo=false\n", 1},
		{"loon-trojan", "[Proxy]\nfixture = trojan, example.invalid, 443, \"synthetic-only\",tls-name=front.example.invalid,skip-cert-verify=false\n", 1},
		{"qx-ss-obfs", "shadowsocks=example.invalid:8388,method=aes-128-gcm,password=synthetic-only,obfs=tls,obfs-host=front.example.invalid,udp-relay=true,tag=fixture", 1},
		{"qx-vmess-wss", "vmess=example.invalid:443,method=auto,password=" + id + ",obfs=wss,obfs-host=front.example.invalid,obfs-uri=/socket,tls-verification=true,aead=true,tag=fixture", 1},
		{"surge-vmess-ws", "fixture = vmess, example.invalid, 443, username=" + id + ",vmess-aead=true,tls=true,sni=front.example.invalid,ws=true,ws-path=/socket,ws-headers=Host:front.example.invalid,skip-cert-verify=false", 1},
		{"loon-vless-ws", "fixture = vless, example.invalid, 443, \"" + id + "\",transport=ws,path=/socket,host=front.example.invalid,over-tls=true,tls-name=front.example.invalid,skip-cert-verify=false", 1},
		{"status-uri", base64.StdEncoding.EncodeToString([]byte("STATUS=synthetic status\ntrojan://synthetic-only@example.invalid:443?sni=front.example.invalid&peer=front.example.invalid#fixture")), 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := importer.Parse(tc.body)
			if result.TotalErrors != 0 || len(result.Nodes) != tc.nodes {
				t.Fatalf("synthetic format parsing failed: nodes=%d errors=%v", len(result.Nodes), result.Errors)
			}
			if strings.HasPrefix(tc.name, "surge-") {
				// The same grammar defaults differently in Surge and Surfboard.
				// UniProxy documents conservative TCP-only extraction unless the
				// source explicitly opts into UDP, rather than guessing its client.
				for _, node := range result.Nodes {
					if node.UDP || node.Config["udp"] != false {
						t.Fatal("ambiguous text format inferred unrequested UDP")
					}
				}
			}
			validateSubscriptionOffline(t, result, binary)
		})
	}
}

// Private files are explicitly opt-in, outside the repository and read only.
// A manifest contains [{"file":"absolute path","nodes":7,"errors":0}, ...].
// Nothing fetches URLs, starts proxy nodes, persists a config, or prints paths,
// submitted values, node labels, credentials or raw core diagnostics.
func TestPrivateSubscriptionFilesOffline(t *testing.T) {
	manifest := os.Getenv("UNIPROXY_TEST_SUBSCRIPTION_MANIFEST")
	if manifest == "" {
		t.Skip("set UNIPROXY_TEST_SUBSCRIPTION_MANIFEST for authorized offline samples")
	}
	raw := privateSubscriptionRead(t, manifest, 64*1024)
	var entries []struct {
		File          string
		Nodes         int
		Errors        int
		ErrorContains string
	}
	if json.Unmarshal(raw, &entries) != nil || len(entries) == 0 || len(entries) > 32 {
		t.Fatal("invalid private subscription manifest")
	}
	binary := os.Getenv("UNIPROXY_TEST_CORE_PATH")
	for i, entry := range entries {
		t.Run(fmt.Sprintf("sample-%02d", i+1), func(t *testing.T) {
			if !filepath.IsAbs(entry.File) || entry.Nodes < 0 || entry.Errors < 0 {
				t.Fatal("invalid private sample specification")
			}
			body := privateSubscriptionRead(t, entry.File, importer.MaxContentBytes)
			result := importer.Parse(string(body))
			if len(result.Nodes) != entry.Nodes || result.TotalErrors != entry.Errors {
				t.Fatalf("private sample count mismatch: nodes=%d errors=%d expectedNodes=%d expectedErrors=%d", len(result.Nodes), result.TotalErrors, entry.Nodes, entry.Errors)
			}
			if entry.ErrorContains != "" {
				matched := false
				for _, issue := range result.Errors {
					if strings.Contains(issue.Message, entry.ErrorContains) {
						matched = true
					}
				}
				if !matched {
					t.Fatal("private sample did not report the expected safe error category")
				}
			}
			if result.TotalErrors == 0 && len(result.Nodes) > 0 {
				if binary != "" {
					validateSubscriptionOffline(t, result, binary)
				} else {
					t.Log("core -t not requested; parser only")
				}
				for _, node := range result.Nodes {
					// Current sample corpus has no relays; do not roundtrip a raw dialer label.
					if node.DialerProxy != "" {
						continue
					}
					encoded, err := json.Marshal(node.Config)
					if err != nil {
						t.Fatal("normalized sample is not JSON-compatible")
					}
					round := importer.Parse(string(encoded))
					if round.TotalErrors != 0 || len(round.Nodes) != 1 || importer.Fingerprint(round.Nodes[0]) != importer.Fingerprint(node) {
						t.Fatal("private sample identity did not survive native reparse")
					}
				}
			}
			t.Logf("offline sample: nodes=%d errors=%d; no proxy server contacted", len(result.Nodes), result.TotalErrors)
		})
	}
}

func privateSubscriptionRead(t *testing.T, path string, limit int64) []byte {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal("cannot open explicitly authorized sample")
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(data)) > limit {
		t.Fatal("cannot read sample within its size bound")
	}
	return data
}
