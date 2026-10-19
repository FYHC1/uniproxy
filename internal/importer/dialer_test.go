package importer

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/uniproxy/uniproxy/internal/model"
	"gopkg.in/yaml.v3"
)

func dialerFixture(name, upstream string) map[string]any {
	obj := nativeFixture("vless")
	obj["name"] = name
	if upstream != "" {
		obj["dialer-proxy"] = upstream
	}
	return obj
}

func clashDoc(t *testing.T, entries ...map[string]any) string {
	t.Helper()
	return jsonDoc(t, map[string]any{"proxies": entries})
}

func requireDialerResult(t *testing.T, result model.ParseResult, names []string, errors int) {
	t.Helper()
	if result.TotalErrors != errors || len(result.Errors) != min(errors, maxIssues) || result.ErrorsTruncated != (errors > maxIssues) {
		t.Fatalf("incorrect error accounting: total=%d details=%d truncated=%v want=%d", result.TotalErrors, len(result.Errors), result.ErrorsTruncated, errors)
	}
	if len(result.Nodes) != len(names) {
		t.Fatalf("incorrect surviving nodes: got=%d want=%d errors=%+v", len(result.Nodes), len(names), result.Errors)
	}
	for i, node := range result.Nodes {
		if node.Name != names[i] {
			t.Fatalf("node ordering changed at entry %d", i)
		}
		if _, ok := node.Config["dialer-proxy"]; ok {
			t.Fatal("raw delegation escaped into native config")
		}
		if _, err := normalizeNative(node.Config); err != nil {
			t.Fatalf("normalized config is no longer safe: %v", err)
		}
		if len(Fingerprint(node)) != 64 {
			t.Fatal("accepted node has no resolved fingerprint")
		}
	}
}

func TestClashDialerForwardReferencesAndAliases(t *testing.T) {
	entries := []map[string]any{
		dialerFixture("exit-one", "middle-one"),
		dialerFixture("relay alias", ""),
		dialerFixture("exit-two", "middle-two"),
		dialerFixture("middle-one", "🛰️ Relay 東京"),
		dialerFixture("middle-two", "relay alias"),
		dialerFixture("🛰️ Relay 東京", ""),
		dialerFixture("independent", ""),
	}
	entries[1]["server"], entries[5]["server"] = "relay.example", "RELAY.example."
	entries[3]["server"], entries[4]["server"] = "middle.example", "middle.example"
	doc := map[string]any{"proxies": entries}
	yamlDoc, err := yaml.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	yamlArray, err := yaml.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	jsonContent := jsonDoc(t, doc)
	inputs := map[string]string{
		"json":       jsonContent,
		"yaml":       string(yamlDoc),
		"json-array": jsonDoc(t, entries),
		"yaml-array": string(yamlArray),
		"base64":     base64.StdEncoding.EncodeToString([]byte(jsonContent)),
	}
	for format, content := range inputs {
		t.Run(format, func(t *testing.T) {
			result := Parse(content)
			requireDialerResult(t, result, []string{"exit-one", "relay alias", "exit-two", "middle-one", "middle-two", "🛰️ Relay 東京", "independent"}, 0)
			nodes := result.Nodes
			for _, pair := range [][2]int{{0, 2}, {3, 4}, {1, 5}} {
				if Fingerprint(nodes[pair[0]]) != Fingerprint(nodes[pair[1]]) {
					t.Fatal("equivalent aliases changed recursive identity")
				}
			}
			for _, edge := range [][2]int{{0, 3}, {2, 4}, {3, 5}, {4, 1}} {
				if nodes[edge[0]].DialerProxy != nodes[edge[1]].Name || nodes[edge[0]].DialerFingerprint != Fingerprint(nodes[edge[1]]) {
					t.Fatal("wrong resolved forward reference")
				}
			}
			if Fingerprint(nodes[0]) == Fingerprint(nodes[6]) || nodes[6].DialerFingerprint != "" || nodes[6].DialerProxy != "" {
				t.Fatal("chain identity collapsed into a direct route")
			}
			encoded, err := json.Marshal(nodes[0])
			if err != nil || strings.Contains(string(encoded), "dialerFingerprint") || !strings.Contains(string(encoded), "dialerProxy") {
				t.Fatal("parsed metadata serialization violates contract")
			}
		})
	}
}

func TestClashDialerRecursiveIdentityChanges(t *testing.T) {
	makeEntries := func(prefix string) []map[string]any {
		relay := nativeFixture("trojan")
		relay["name"], relay["server"] = prefix+"relay", "relay.example"
		middle := dialerFixture(prefix+"middle", prefix+"relay")
		middle["server"] = "middle.example"
		return []map[string]any{dialerFixture(prefix+"exit", prefix+"middle"), middle, relay}
	}
	baseline := Parse(clashDoc(t, makeEntries("")...))
	requireDialerResult(t, baseline, []string{"exit", "middle", "relay"}, 0)
	renamed := Parse(clashDoc(t, makeEntries("🧭 renamed ")...))
	for i := range baseline.Nodes {
		if Fingerprint(baseline.Nodes[i]) != Fingerprint(renamed.Nodes[i]) {
			t.Fatal("renaming graph labels affected identity")
		}
	}
	for _, change := range []struct {
		name  string
		index int
		key   string
		value any
	}{
		{"relay-credential", 2, "password", "other-synthetic-password"},
		{"relay-tls", 2, "skip-cert-verify", true},
		{"relay-endpoint", 2, "port", 8443},
		{"middle-udp", 1, "udp", false},
		{"middle-endpoint", 1, "server", "other.example"},
		{"exit-credential", 0, "uuid", "12345678-1234-4234-8234-123456789abd"},
	} {
		t.Run(change.name, func(t *testing.T) {
			entries := makeEntries("")
			entries[change.index][change.key] = change.value
			result := Parse(clashDoc(t, entries...))
			requireDialerResult(t, result, []string{"exit", "middle", "relay"}, 0)
			for i := range result.Nodes {
				changed := Fingerprint(result.Nodes[i]) != Fingerprint(baseline.Nodes[i])
				if changed != (i <= change.index) {
					t.Fatalf("recursive identity propagation wrong at entry %d", i)
				}
			}
		})
	}
}

// This is the pre-relay identity encoding. Do not add dependency metadata here:
// old stored hashes must continue to deduplicate imports of direct nodes.
func legacyDirectFingerprint(node model.ParsedNode) string {
	config := make(map[string]any, len(node.Config))
	for k, v := range node.Config {
		if k != "name" {
			config[k] = v
		}
	}
	identity := struct {
		Protocol string         `json:"protocol"`
		Server   string         `json:"server"`
		Port     int            `json:"port"`
		UDP      bool           `json:"udp"`
		Config   map[string]any `json:"config"`
	}{node.Protocol, node.Server, node.Port, node.UDP, config}
	data, _ := json.Marshal(identity)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestFingerprintDirectHashCompatibility(t *testing.T) {
	for _, protocol := range []string{"vless", "vmess", "trojan", "ss", "hysteria", "hysteria2", "tuic", "ssh", "socks5", "http"} {
		t.Run(protocol, func(t *testing.T) {
			node := parseOne(t, nativeDoc(t, nativeFixture(protocol)))
			if Fingerprint(node) != legacyDirectFingerprint(node) {
				t.Fatal("direct hash changed from pre-relay encoding")
			}
		})
	}
}

func TestFingerprintRejectsUnresolvedDialer(t *testing.T) {
	node := parseOne(t, nativeDoc(t, nativeFixture("vless")))
	node.DialerProxy = "upstream"
	for _, invalid := range []string{"", "short", strings.Repeat("x", 64), strings.Repeat("a", 63), strings.Repeat("a", 66)} {
		node.DialerFingerprint = invalid
		if Fingerprint(node) != "" {
			t.Fatal("invalid dependency metadata received a usable identity")
		}
	}
	node.DialerFingerprint = strings.Repeat("a", 64)
	chained := Fingerprint(node)
	node.DialerProxy = "renamed upstream"
	if chained == "" || Fingerprint(node) != chained {
		t.Fatal("resolved reference name affected identity")
	}
	node.DialerProxy = ""
	if Fingerprint(node) != chained {
		t.Fatal("stable-ID callers must not require a raw document label")
	}
	node.Config["dialer-proxy"] = "upstream"
	if Fingerprint(node) != "" {
		t.Fatal("raw config delegation must not receive a fingerprint")
	}
}

func TestClashDialerRejectsInvalidGraphs(t *testing.T) {
	invalid := dialerFixture("invalid", "")
	invalid["port"] = 0
	duplicateInvalid := dialerFixture("duplicate", "")
	duplicateInvalid["port"] = 0
	for _, test := range []struct {
		name    string
		entries []map[string]any
		message string
	}{
		{"missing", []map[string]any{dialerFixture("exit", "missing")}, "missing"},
		{"self", []map[string]any{dialerFixture("self", "self")}, "itself"},
		{"cycle", []map[string]any{dialerFixture("a", "b"), dialerFixture("b", "a")}, "cycle"},
		{"cycle-with-descendant", []map[string]any{dialerFixture("exit", "a"), dialerFixture("a", "b"), dialerFixture("b", "a")}, "invalid"},
		{"invalid-target", []map[string]any{dialerFixture("exit", "invalid"), invalid}, "invalid"},
		{"invalid-dependency", []map[string]any{dialerFixture("exit", "middle"), dialerFixture("middle", "missing")}, "invalid"},
		{"duplicate-equivalent", []map[string]any{dialerFixture("exit", "duplicate"), dialerFixture("duplicate", ""), dialerFixture("duplicate", "")}, "duplicate"},
		{"duplicate-invalid", []map[string]any{dialerFixture("exit", "duplicate"), duplicateInvalid, dialerFixture("duplicate", "")}, "duplicate"},
		{"duplicate-unused", []map[string]any{dialerFixture("duplicate", ""), dialerFixture("duplicate", "")}, "duplicate"},
	} {
		t.Run(test.name, func(t *testing.T) {
			entries := append([]map[string]any{}, test.entries...)
			entries = append(entries, dialerFixture("survivor", ""))
			result := Parse(clashDoc(t, entries...))
			requireDialerResult(t, result, []string{"survivor"}, len(test.entries))
			if !strings.Contains(result.Errors[0].Message, test.message) {
				t.Fatalf("missing actionable graph diagnostic: %s", result.Errors[0].Message)
			}
		})
	}
}

func TestClashDialerRejectsBuiltinAndGroupTargets(t *testing.T) {
	for _, target := range []string{"DIRECT", "REJECT", "GLOBAL", "REJECT-DROP", "PASS", "COMPATIBLE", "direct", " GLOBAL "} {
		t.Run("builtin-"+strings.TrimSpace(target), func(t *testing.T) {
			// Even a native proxy with this exact label cannot shadow a builtin.
			result := Parse(clashDoc(t, dialerFixture("exit", target), dialerFixture(target, "")))
			requireDialerResult(t, result, []string{target}, 1)
			if !strings.Contains(result.Errors[0].Message, "builtin") {
				t.Fatal("builtin reference must be explicitly rejected")
			}
		})
	}
	for _, kind := range []string{"select", "fallback", "url-test", "load-balance", "relay"} {
		t.Run("group-"+kind, func(t *testing.T) {
			doc := map[string]any{
				"proxies":      []any{dialerFixture("exit", "group"), dialerFixture("group", ""), dialerFixture("survivor", "")},
				"proxy-groups": []any{map[string]any{"name": "group", "type": kind, "proxies": []string{"survivor"}}},
			}
			result := Parse(jsonDoc(t, doc))
			requireDialerResult(t, result, []string{"group", "survivor"}, 1)
			if !strings.Contains(result.Errors[0].Message, "proxy group") {
				t.Fatal("proxy/group name collision must not silently choose the native node")
			}
		})
	}
	result := Parse(jsonDoc(t, map[string]any{
		"proxies":      []any{dialerFixture("exit", "only-group"), dialerFixture("survivor", "")},
		"proxy-groups": []any{map[string]any{"name": "only-group", "type": "select", "proxies": []string{"survivor"}}},
	}))
	requireDialerResult(t, result, []string{"survivor"}, 1)
	if !strings.Contains(result.Errors[0].Message, "proxy group") {
		t.Fatal("group-only target must receive an explicit explanation")
	}
}

func TestClashDialerReferenceValidationAndScope(t *testing.T) {
	for i, ref := range []any{nil, false, 42, []any{"relay"}, map[string]any{"name": "relay"}, "", " \t", "bad\nlabel", strings.Repeat("a", 1025)} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			exit := dialerFixture("exit", "")
			exit["dialer-proxy"] = ref
			result := Parse(clashDoc(t, exit, dialerFixture("relay", "")))
			requireDialerResult(t, result, []string{"relay"}, 1)
		})
	}
	for _, ref := range []string{"Relay", " relay", "relay ", "other-document"} {
		result := Parse(clashDoc(t, dialerFixture("exit", ref), dialerFixture("relay", "")))
		requireDialerResult(t, result, []string{"relay"}, 1)
	}
	// Literal Unicode/space labels resolve exactly, without lossy slugging.
	ref := " 🧭 日本 relay @ edge "
	result := Parse(clashDoc(t, dialerFixture("exit", ref), dialerFixture(ref, "")))
	requireDialerResult(t, result, []string{"exit", ref}, 0)

	unnamed := dialerFixture("", "")
	delete(unnamed, "name")
	generated := defaultName("vless", "example.com", 443)
	result = Parse(clashDoc(t, dialerFixture("exit", generated), unnamed))
	requireDialerResult(t, result, []string{generated}, 1)

	parseOne(t, clashDoc(t, dialerFixture("other-document", "")))
	requireInvalid(t, clashDoc(t, dialerFixture("exit", "other-document")))
	requireInvalid(t, jsonDoc(t, dialerFixture("exit", "relay")))
	for _, key := range []string{"detour", "dialer-proxy"} {
		obj := singFixture("vless")
		obj[key] = "relay"
		requireInvalid(t, singDoc(t, obj))
		requireInvalid(t, "vless://"+testUUID+"@example.com:443?"+key+"=relay")
	}
	for _, key := range []string{"dialer-proxy", "Dialer-Proxy", "dialer_proxy", "detour"} {
		obj := dialerFixture("exit", "")
		obj[key] = "relay"
		if _, err := normalizeNative(obj); err == nil {
			t.Fatal("document support weakened strict native normalization")
		}
	}
}

func TestClashDialerKeepsOtherNativeOptionsStrict(t *testing.T) {
	for _, key := range []string{"detour", "interface-name", "routing-mark", "certificate-path", "provider", "exec", "unknown", "Dialer-Proxy", "dialer_proxy"} {
		t.Run(key, func(t *testing.T) {
			exit := dialerFixture("exit", "relay")
			exit[key] = "synthetic-unsafe-option"
			result := Parse(clashDoc(t, exit, dialerFixture("relay", "")))
			requireDialerResult(t, result, []string{"relay"}, 1)
		})
	}
	exit := dialerFixture("exit", "relay")
	exit["network"] = "ws"
	exit["ws-opts"] = map[string]any{"dialer-proxy": "relay"}
	result := Parse(clashDoc(t, exit, dialerFixture("relay", "")))
	requireDialerResult(t, result, []string{"relay"}, 1)

	// Case-distinct explicit native names are not fuzzy-matched or merged.
	result = Parse(clashDoc(t, dialerFixture("exit", "Relay"), dialerFixture("Relay", ""), dialerFixture("relay", "")))
	requireDialerResult(t, result, []string{"exit", "Relay", "relay"}, 0)

	// A native definition without a usable name cannot be a target.
	for _, name := range []any{42, nil, "bad\nlabel", strings.Repeat("a", 1025)} {
		target := dialerFixture("", "")
		target["name"] = name
		result := Parse(clashDoc(t, dialerFixture("exit", "target"), target, dialerFixture("survivor", "")))
		requireDialerResult(t, result, []string{"survivor"}, 2)
	}
}

func TestClashDialerSourceOrderAndLines(t *testing.T) {
	content := "proxies:\n" +
		"  - {name: dependent, type: http, server: exit.example, port: 80, dialer-proxy: invalid}\n" +
		"  - {name: before, type: http, server: good.example, port: 80}\n" +
		"  - {name: a, type: http, server: a.example, port: 80, dialer-proxy: b}\n" +
		"  - {name: middle, type: http, server: good.example, port: 80}\n" +
		"  - {name: invalid, type: http, server: bad.example, port: 0}\n" +
		"  - {name: b, type: http, server: b.example, port: 80, dialer-proxy: a}\n" +
		"  - {name: after, type: http, server: good.example, port: 80}\n"
	result := Parse(content)
	requireDialerResult(t, result, []string{"before", "middle", "after"}, 4)
	for i, line := range []int{2, 4, 6, 7} {
		if result.Errors[i].Line != line {
			t.Fatalf("wrong diagnostic order/line: got=%d want=%d", result.Errors[i].Line, line)
		}
	}
}

func TestClashDialerDepthLimitAndValidSuffixes(t *testing.T) {
	for _, edges := range []int{MaxDialerDepth - 1, MaxDialerDepth, MaxDialerDepth + 1, MaxDialerDepth + 2} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("edges-%d/reverse-%v", edges, reverse), func(t *testing.T) {
				entries := make([]map[string]any, 0, edges+1)
				names := []string{}
				for n := 0; n <= edges; n++ {
					i := n
					if reverse {
						i = edges - n
					}
					upstream := ""
					if i < edges {
						upstream = fmt.Sprintf("node-%d", i+1)
					}
					name := fmt.Sprintf("node-%d", i)
					entries = append(entries, dialerFixture(name, upstream))
					if edges-i <= MaxDialerDepth {
						names = append(names, name)
					}
				}
				result := Parse(clashDoc(t, entries...))
				requireDialerResult(t, result, names, max(0, edges-MaxDialerDepth))
			})
		}
	}
}

func TestClashDialerLongCycleAndSurvivor(t *testing.T) {
	entries := make([]map[string]any, 0, MaxNodes)
	for i := 0; i < MaxNodes-1; i++ {
		entries = append(entries, dialerFixture(fmt.Sprintf("cycle-%d", i), fmt.Sprintf("cycle-%d", (i+1)%(MaxNodes-1))))
	}
	entries = append(entries, dialerFixture("survivor", ""))
	result := Parse(clashDoc(t, entries...))
	requireDialerResult(t, result, []string{"survivor"}, MaxNodes-1)
	if !strings.Contains(result.Errors[0].Message, "cycle") {
		t.Fatal("long cycle must be diagnosed without overflowing recursion")
	}
}

func TestClashDialerLargeSyntheticSubscription(t *testing.T) {
	const total, chained = 318, 168
	aliases := []string{"🛰️ Relay 日本", "relay alias"}
	entries := make([]map[string]any, 0, total)
	names := make([]string, 0, total)
	for i := 0; i < total-2; i++ {
		name, upstream := fmt.Sprintf("synthetic-%03d", i), ""
		if i < chained {
			upstream = aliases[i%len(aliases)]
		}
		entry := dialerFixture(name, upstream)
		entry["port"] = 10000 + i/2
		entries = append(entries, entry)
		names = append(names, name)
	}
	for _, alias := range aliases {
		entry := dialerFixture(alias, "")
		entry["server"] = "relay.example"
		entries = append(entries, entry)
		names = append(names, alias)
	}
	result := Parse(clashDoc(t, entries...))
	requireDialerResult(t, result, names, 0)
	refs := 0
	for i, node := range result.Nodes {
		if node.DialerProxy != "" {
			refs++
			if node.DialerFingerprint != Fingerprint(result.Nodes[total-1]) {
				t.Fatal("synthetic forward alias resolved incorrectly")
			}
			if i%2 == 1 && Fingerprint(node) != Fingerprint(result.Nodes[i-1]) {
				t.Fatal("equivalent relay aliases prevented deduplication")
			}
		}
	}
	if refs != chained {
		t.Fatalf("lost relay references: got=%d want=%d", refs, chained)
	}
}

func TestClashDialerDiagnosticsNeverEchoLabelsOrCredentials(t *testing.T) {
	secret := "SYNTHETIC_DO_NOT_ECHO_8f1b"
	invalid := dialerFixture(secret, "")
	invalid["uuid"] = secret
	for _, entries := range [][]map[string]any{
		{dialerFixture("exit", secret)},
		{dialerFixture(secret, secret)},
		{dialerFixture("exit", secret), invalid},
		{dialerFixture("exit", secret), dialerFixture(secret, ""), dialerFixture(secret, "")},
	} {
		result := Parse(clashDoc(t, entries...))
		if result.TotalErrors == 0 {
			t.Fatal("bad graph fixture unexpectedly passed")
		}
		for _, problem := range result.Errors {
			if strings.Contains(problem.Message, secret) {
				t.Fatal("dependency diagnostic leaked a submitted value")
			}
		}
	}
}

func TestClashDialerConcurrentDeterminism(t *testing.T) {
	content := clashDoc(t, dialerFixture("exit", "relay"), dialerFixture("relay", ""))
	expected := Parse(content)
	var group sync.WaitGroup
	for i := 0; i < 16; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for j := 0; j < 10; j++ {
				if actual := Parse(content); !reflect.DeepEqual(actual, expected) {
					t.Error("document-local dependency resolution is not deterministic")
				}
			}
		}()
	}
	group.Wait()
}
