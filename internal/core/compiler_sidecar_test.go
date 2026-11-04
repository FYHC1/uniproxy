package core

import (
	"errors"
	"strings"
	"testing"

	"github.com/uniproxy/uniproxy/internal/model"
)

func sidecarTestNode(id string, port int) model.Node {
	return model.Node{
		ID: id, Enabled: true, Engine: model.EngineXray,
		ParsedNode: model.ParsedNode{
			Name: id, Protocol: "vless", Server: "real.example.com", Port: 443, UDP: true,
			Config: map[string]any{
				"xray": map[string]any{
					"protocol": "vless",
					"settings": map[string]any{"vnext": []any{map[string]any{"address": "real.example.com", "port": 443}}},
				},
			},
		},
		SidecarEndpoint: &model.SidecarEndpoint{Host: "127.0.0.1", Port: port, Username: "u-" + id, Password: "pass-" + id},
	}
}

func TestBuildSidecarUsesOnlyLoopbackSocksOutbound(t *testing.T) {
	s := testState()
	s.Nodes = append(s.Nodes, sidecarTestNode("s", 39010))
	doc := decodedBuild(t, s)
	proxy := proxyByName(t, doc, "x-s")
	if proxy["type"] != "socks5" || proxy["server"] != "127.0.0.1" || proxy["port"] != 39010 {
		t.Fatalf("unexpected sidecar outbound: %#v", proxy)
	}
	if proxy["username"] != "u-s" || proxy["password"] != "pass-s" || proxy["udp"] != true {
		t.Fatalf("sidecar credentials or UDP were not preserved: %#v", proxy)
	}
	data, err := Build(s, testBuildOptions())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "real.example.com") {
		t.Fatal("real remote address leaked into Mihomo configuration")
	}
}

func TestBuildSidecarAllowedAtChainHead(t *testing.T) {
	s := testState()
	s.Nodes = []model.Node{sidecarTestNode("s", 39011), testNode("a", false)}
	s.Pools = nil
	s.Chains = []model.Chain{{ID: "route", Name: "route", NodeIDs: []string{"s", "a"}, Enabled: true}}
	s.Listeners = []model.Listener{{ID: "chain-in", Kind: "chain", TargetID: "route", Protocol: "mixed", Host: "127.0.0.1", Port: 20200, Enabled: true}}
	doc := decodedBuild(t, s)
	first := proxyByName(t, doc, "c-route-1")
	second := proxyByName(t, doc, "c-route-2")
	if first["type"] != "socks5" || first["server"] != "127.0.0.1" || first["dialer-proxy"] != nil {
		t.Fatalf("unexpected sidecar head hop: %#v", first)
	}
	if second["dialer-proxy"] != "c-route-1" {
		t.Fatalf("chain did not relay through the sidecar head: %#v", second)
	}
}

func TestBuildSidecarRejectedOutsideChainHead(t *testing.T) {
	for _, nodeIDs := range [][]string{{"a", "s"}, {"a", "s", "b"}} {
		s := testState()
		s.Nodes = []model.Node{testNode("a", false), sidecarTestNode("s", 39012), testNode("b", true)}
		s.Pools = nil
		s.Chains = []model.Chain{{ID: "route", Name: "route", NodeIDs: nodeIDs, Enabled: true}}
		s.Listeners = []model.Listener{{ID: "chain-in", Kind: "chain", TargetID: "route", Protocol: "mixed", Host: "127.0.0.1", Port: 20200, Enabled: true}}
		if _, err := Build(s, testBuildOptions()); !errors.Is(err, model.ErrSidecarMustBeChainHead) {
			t.Fatalf("chain %v error = %v", nodeIDs, err)
		}
	}
}
