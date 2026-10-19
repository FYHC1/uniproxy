package core

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/uniproxy/uniproxy/internal/model"
	"gopkg.in/yaml.v3"
)

func testBuildOptions() BuildOptions {
	return BuildOptions{ControllerAddress: "127.0.0.1:39001", ControllerSecret: "controller-secret", ProbeAddress: "127.0.0.1:39002", ProbeSecret: "probe-secret"}
}
func testNode(id string, udp bool) model.Node {
	return model.Node{ID: id, Enabled: true, ParsedNode: model.ParsedNode{Name: "label-" + id, Protocol: "socks5", Server: "127.0.0.1", Port: 1080, UDP: udp, Config: map[string]any{"type": "socks5", "server": "127.0.0.1", "port": 1080, "udp": udp, "username": "upstream", "password": "private"}}}
}
func testState() model.State {
	s := model.DefaultState()
	s.Revision = 1
	s.Nodes = []model.Node{testNode("b", true), testNode("a", false)}
	s.Pools = []model.Pool{{ID: "pool", Enabled: true, NodeIDs: []string{"b", "a"}, Strategy: "round-robin", TestURL: "http://127.0.0.1:9/health", IntervalSeconds: 300, TimeoutMS: 8000, ToleranceMS: 50}}
	s.Listeners = []model.Listener{{ID: "listener", Enabled: true, Kind: "pool", TargetID: "pool", Protocol: "mixed", Host: "127.0.0.1", Port: 20100, Users: []model.Credential{{Username: "client", Password: "secret"}}}}
	return s
}
func decodedBuild(t *testing.T, s model.State) map[string]any {
	t.Helper()
	data, err := Build(s, testBuildOptions())
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err = yaml.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	return document
}
func firstMap(t *testing.T, doc map[string]any, key string) map[string]any {
	t.Helper()
	entries, ok := doc[key].([]any)
	if !ok || len(entries) == 0 {
		t.Fatalf("missing %s", key)
	}
	entry, ok := entries[0].(map[string]any)
	if !ok {
		t.Fatalf("invalid %s", key)
	}
	return entry
}
func TestBuildNativeStrategies(t *testing.T) {
	for _, strategy := range []string{"url-test", "round-robin", "consistent-hashing", "sticky-sessions"} {
		for _, hashKey := range []string{"", "in-user"} {
			if hashKey != "" && (strategy == "url-test" || strategy == "round-robin") {
				continue
			}
			t.Run(strategy+"/"+hashKey, func(t *testing.T) {
				s := testState()
				s.Pools[0].Strategy = strategy
				s.Pools[0].HashKey = hashKey
				doc := decodedBuild(t, s)
				group := firstMap(t, doc, "proxy-groups")
				expectedType := "load-balance"
				if strategy == "url-test" {
					expectedType = "url-test"
				}
				if group["type"] != expectedType {
					t.Fatalf("group not native: %#v", group)
				}
				if strategy != "url-test" && group["strategy"] != strategy {
					t.Fatal("incorrect native strategy")
				}
				if hashKey != "" && group["hash-key"] != hashKey {
					t.Fatal("missing native hash-key")
				}
				if group["empty-fallback"] != "REJECT" {
					t.Fatal("unsafe native empty fallback")
				}
				if !reflect.DeepEqual(group["proxies"], []any{"n-a", "n-b"}) {
					t.Fatalf("unstable ordering: %#v", group["proxies"])
				}
				if firstMap(t, doc, "listeners")["proxy"] != "p-pool" {
					t.Fatal("listener is not directly bound to native group")
				}
			})
		}
	}
}
func TestBuildStableAndDoesNotMutate(t *testing.T) {
	s := testState()
	before, _ := json.Marshal(s)
	a, err := Build(s, testBuildOptions())
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(s)
	if !bytes.Equal(before, after) {
		t.Fatal("compiler mutated source model")
	}
	s.Nodes[0].Name = "REJECT"
	s.Nodes[1].Name = "COMMA,\nname"
	s.Listeners[0].Name = "renamed"
	s.Pools[0].Name = "other"
	s.Nodes[0], s.Nodes[1] = s.Nodes[1], s.Nodes[0]
	b, err := Build(s, testBuildOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatal("UI names or input node order changed compiled configuration")
	}
}
func TestBuildClosedForEmptyDisabledDeletedTargets(t *testing.T) {
	for _, mode := range []string{"empty", "disabled-pool", "deleted-pool", "disabled-node", "deleted-node"} {
		t.Run(mode, func(t *testing.T) {
			s := testState()
			switch mode {
			case "empty":
				s.Pools[0].NodeIDs = nil
			case "disabled-pool":
				s.Pools[0].Enabled = false
			case "deleted-pool":
				s.Pools = nil
			case "disabled-node":
				s.Listeners[0].Kind = "node"
				s.Listeners[0].TargetID = "a"
				s.Nodes[1].Enabled = false
			case "deleted-node":
				s.Listeners[0].Kind = "node"
				s.Listeners[0].TargetID = "deleted"
			}
			doc := decodedBuild(t, s)
			rules := doc["rules"].([]any)
			if len(rules) != 1 || rules[0] != "MATCH,REJECT" {
				t.Fatal("global traffic is not rejected")
			}
			target := firstMap(t, doc, "listeners")["proxy"]
			if mode == "empty" {
				if target != "p-pool" {
					t.Fatal(target)
				}
				if !reflect.DeepEqual(firstMap(t, doc, "proxy-groups")["proxies"], []any{"REJECT"}) {
					t.Fatal("empty pool not rejected")
				}
			} else if target != "REJECT" {
				t.Fatalf("stale target route: %v", target)
			}
			encoded, _ := yaml.Marshal(doc)
			if strings.Contains(string(encoded), "DIRECT") || strings.Contains(string(encoded), "COMPATIBLE") {
				t.Fatal("unsafe fallback in generated config")
			}
		})
	}
}
func TestBuildPrivateProbeAuthenticationAndIsolation(t *testing.T) {
	s := testState()
	doc := decodedBuild(t, s)
	listeners := doc["listeners"].([]any)
	probe := listeners[len(listeners)-1].(map[string]any)
	if probe["name"] != probeName || probe["listen"] != "127.0.0.1" || probe["udp"] != false || probe["rule"] != probeName {
		t.Fatalf("unsafe probe: %#v", probe)
	}
	users := probe["users"].([]any)
	u := users[0].(map[string]any)
	if u["username"] != "n-a" || u["password"] != probePassword("probe-secret", "a") {
		t.Fatal("probe identity is not node-specific")
	}
	if users[1].(map[string]any)["password"] == u["password"] {
		t.Fatal("probe passwords reused across nodes")
	}
	subRules := doc["sub-rules"].(map[string]any)[probeName].([]any)
	if !reflect.DeepEqual(subRules, []any{"IN-USER,n-a,n-a", "IN-USER,n-b,n-b", "MATCH,REJECT"}) {
		t.Fatalf("unsafe probe rules: %#v", subRules)
	}
	s.Nodes = nil
	s.Listeners = nil
	doc = decodedBuild(t, s)
	probe = firstMap(t, doc, "listeners")
	if len(probe["users"].([]any)) != 1 {
		t.Fatal("empty state must not disable private probe auth")
	}
}
func TestBuildUDPFiltersMembership(t *testing.T) {
	s := testState()
	s.Pools[0].UDP = true
	s.Listeners[0].UDP = true
	doc := decodedBuild(t, s)
	g := firstMap(t, doc, "proxy-groups")
	if !reflect.DeepEqual(g["proxies"], []any{"n-b"}) || g["disable-udp"] != false {
		t.Fatalf("bad UDP group %#v", g)
	}
	if firstMap(t, doc, "listeners")["udp"] != true {
		t.Fatal("UDP option dropped")
	}
}
func TestBuildGroupMembershipExclusions(t *testing.T) {
	s := testState()
	s.Nodes[0].GroupIDs = []string{"g1", "g2"}
	s.Nodes[1].GroupIDs = []string{"g1"}
	s.Pools[0].NodeIDs = nil
	s.Pools[0].GroupIDs = []string{"g1", "g2"}
	s.Pools[0].GroupMode = "all"
	g := firstMap(t, decodedBuild(t, s), "proxy-groups")
	if !reflect.DeepEqual(g["proxies"], []any{"n-b"}) {
		t.Fatal(g)
	}
	s.Pools[0].ExcludeNodeIDs = []string{"b"}
	g = firstMap(t, decodedBuild(t, s), "proxy-groups")
	if !reflect.DeepEqual(g["proxies"], []any{"REJECT"}) {
		t.Fatal(g)
	}
}
func TestBuildRejectsUnsafeOrInconsistentConfiguration(t *testing.T) {
	cases := map[string]func(*model.State){
		"http-udp": func(s *model.State) { s.Listeners[0].Protocol = "http"; s.Listeners[0].UDP = true },
		"node-udp": func(s *model.State) {
			s.Listeners[0].Kind = "node"
			s.Listeners[0].TargetID = "a"
			s.Listeners[0].UDP = true
		},
		"pool-udp":            func(s *model.State) { s.Listeners[0].UDP = true },
		"public-without-auth": func(s *model.State) { s.Listeners[0].Host = "0.0.0.0"; s.Listeners[0].Users = nil },
		"public-udp": func(s *model.State) {
			s.Listeners[0].Host = "0.0.0.0"
			s.Listeners[0].UDP = true
			s.Pools[0].UDP = true
		},
		"hash-no-auth": func(s *model.State) {
			s.Pools[0].Strategy = "consistent-hashing"
			s.Pools[0].HashKey = "in-user"
			s.Listeners[0].Users = nil
		},
		"hash-udp": func(s *model.State) {
			s.Pools[0].Strategy = "sticky-sessions"
			s.Pools[0].HashKey = "in-user"
			s.Pools[0].UDP = true
			s.Listeners[0].UDP = true
		},
		"roundrobin-hash":     func(s *model.State) { s.Pools[0].HashKey = "in-user" },
		"duplicate-node":      func(s *model.State) { s.Nodes = append(s.Nodes, s.Nodes[0]) },
		"invalid-id":          func(s *model.State) { s.Nodes[0].ID = "injected,MATCH,DIRECT" },
		"invalid-user":        func(s *model.State) { s.Listeners[0].Users[0].Username = "contains:colon" },
		"credential-required": func(s *model.State) { s.Listeners[0].Users[0].Password = "" },
		"port-overlap":        func(s *model.State) { l := s.Listeners[0]; l.ID = "other"; s.Listeners = append(s.Listeners, l) },
		"controller-overlap":  func(s *model.State) { s.Listeners[0].Port = 39001 },
		"proxy-alias":         func(s *model.State) { s.Nodes[0].Config["type"] = "socks" },
		"proxy-direct":        func(s *model.State) { s.Nodes[0].Protocol = "direct"; s.Nodes[0].Config["type"] = "direct" },
		"native-detour":       func(s *model.State) { s.Nodes[0].Config["dialer-proxy"] = "DIRECT" },
		"unsafe-key-path":     func(s *model.State) { s.Nodes[0].Config["private-key"] = "/etc/ssh/secret" },
		"credential-url":      func(s *model.State) { s.Pools[0].TestURL = "https://user:password@example.org" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			s := testState()
			mutate(&s)
			if _, err := Build(s, testBuildOptions()); err == nil {
				t.Fatal("unsafe configuration accepted")
			}
		})
	}
}
func TestBuildRequiresPrivateControllerAndProbe(t *testing.T) {
	for _, mutate := range []func(*BuildOptions){func(o *BuildOptions) { o.ControllerAddress = "0.0.0.0:9091" }, func(o *BuildOptions) { o.ProbeAddress = "localhost:12345" }, func(o *BuildOptions) { o.ProbeSecret = "" }, func(o *BuildOptions) { o.ControllerSecret = "" }, func(o *BuildOptions) { o.ProbeAddress = o.ControllerAddress }} {
		o := testBuildOptions()
		mutate(&o)
		if _, err := Build(testState(), o); err == nil {
			t.Fatal("unsafe private endpoint accepted")
		}
	}
}
func TestVersionParser(t *testing.T) {
	for _, s := range []string{"Mihomo Meta v1.19.32 windows amd64 with go1.24", "Mihomo v1.19.32", "mihomo 1.19.32 linux arm64"} {
		if v := extractVersion(s); v != PinnedVersion {
			t.Fatalf("version %q from %q", v, s)
		}
	}
	for _, s := range []string{"Clash 1.19.32", "Mihomo Meta 1x19x32", "Mihomo Meta v1.19.32-dev"} {
		if extractVersion(s) != "" {
			t.Fatal("unsupported version accepted")
		}
	}
}
func TestBoundedLogDoesNotGrow(t *testing.T) {
	b := &boundedLog{limit: 32}
	mark := b.mark()
	_, _ = b.Write([]byte(strings.Repeat("private", 100) + "listen err: secret"))
	if len(b.text()) != 32 || !b.listenerErrorSince(mark) {
		t.Fatal("bounded log behavior incorrect")
	}
	if b.listenerErrorSince(b.mark()) {
		t.Fatal("old bind error leaked into new apply")
	}
}
