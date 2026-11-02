package core

import (
	"strings"
	"testing"

	"github.com/uniproxy/uniproxy/internal/model"
	"gopkg.in/yaml.v3"
)

func chainTestState(nodeIDs ...string) model.State {
	s := testState()
	s.Chains = []model.Chain{{ID: "route", Name: "route", NodeIDs: nodeIDs, Enabled: true}}
	s.Listeners = []model.Listener{{ID: "chain-in", Name: "chain entry", Kind: "chain", TargetID: "route", Protocol: "mixed", Host: "127.0.0.1", Port: 20200, Enabled: true}}
	return s
}

func proxyByName(t *testing.T, doc map[string]any, name string) map[string]any {
	t.Helper()
	for _, raw := range doc["proxies"].([]any) {
		proxy := raw.(map[string]any)
		if proxy["name"] == name {
			return proxy
		}
	}
	t.Fatalf("proxy %q missing", name)
	return nil
}

func TestBuildChainUsesOrderedSyntheticHops(t *testing.T) {
	s := chainTestState("b", "a")
	doc := decodedBuild(t, s)
	first := proxyByName(t, doc, "c-route-1")
	second := proxyByName(t, doc, "c-route-2")
	if first["dialer-proxy"] != nil {
		t.Fatalf("entry hop unexpectedly relays: %#v", first)
	}
	if second["dialer-proxy"] != "c-route-1" {
		t.Fatalf("exit hop does not relay through entry: %#v", second)
	}
	if proxyByName(t, doc, "n-a")["dialer-proxy"] != nil || proxyByName(t, doc, "n-b")["dialer-proxy"] != nil {
		t.Fatal("chain altered the original node's relay settings")
	}
	if got := firstMap(t, doc, "listeners")["proxy"]; got != "c-route-2" {
		t.Fatalf("listener target = %v", got)
	}
	data, _ := yaml.Marshal(doc)
	if strings.Contains(string(data), "DIRECT") {
		t.Fatal("chain emitted a direct fallback")
	}
}

func TestBuildChainFailsClosedWithoutShortening(t *testing.T) {
	for _, mode := range []string{"disabled-hop", "missing-hop", "disabled-chain", "duplicate-hop"} {
		t.Run(mode, func(t *testing.T) {
			s := chainTestState("b", "a", "missing")
			switch mode {
			case "disabled-hop":
				s.Chains[0].NodeIDs = []string{"b", "a"}
				s.Nodes[1].Enabled = false
			case "missing-hop":
				s.Chains[0].NodeIDs = []string{"b", "ghost"}
			case "disabled-chain":
				s.Chains[0].NodeIDs = []string{"b", "a"}
				s.Chains[0].Enabled = false
			case "duplicate-hop":
				s.Chains[0].NodeIDs = []string{"b", "b"}
			}
			doc := decodedBuild(t, s)
			if got := firstMap(t, doc, "listeners")["proxy"]; got != "REJECT" {
				t.Fatalf("listener did not fail closed: %v", got)
			}
			for _, raw := range doc["proxies"].([]any) {
				if strings.HasPrefix(raw.(map[string]any)["name"].(string), "c-route-") {
					t.Fatal("partial chain outbound emitted")
				}
			}
		})
	}
}

func TestBuildChainRejectsUDPWhenAnyHopIsTCPOnly(t *testing.T) {
	s := chainTestState("b", "a")
	s.Listeners[0].UDP = true
	if _, err := Build(s, testBuildOptions()); err == nil || !strings.Contains(err.Error(), "TCP-only") {
		t.Fatalf("UDP chain error = %v", err)
	}
}
