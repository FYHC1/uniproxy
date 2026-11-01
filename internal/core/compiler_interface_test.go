package core

import (
	"strings"
	"testing"

	"github.com/uniproxy/uniproxy/internal/model"
)

func TestBuildGlobalInterfaceName(t *testing.T) {
	s := testState()
	doc := decodedBuild(t, s)
	if _, found := doc["interface-name"]; found {
		t.Fatal("automatic outbound must not emit interface-name")
	}
	// Only the administrator setting may pin a global interface.
	s.Settings.OutboundInterface = "以太网"
	doc = decodedBuild(t, s)
	if got := doc["interface-name"]; got != "以太网" {
		t.Fatalf("global interface-name = %#v", got)
	}
	// Surrounding whitespace from a PATCH or a restored backup must not produce
	// an interface name the core can never match.
	s.Settings.OutboundInterface = "  以太网 "
	doc = decodedBuild(t, s)
	if got := doc["interface-name"]; got != "以太网" {
		t.Fatalf("trimmed interface-name = %#v", got)
	}
	// The pin is global only: proxies keep no per-node source interface.
	proxies, _ := doc["proxies"].([]any)
	if len(proxies) != 2 {
		t.Fatalf("proxies = %#v", doc["proxies"])
	}
	for _, raw := range proxies {
		entry, _ := raw.(map[string]any)
		if _, found := entry["interface-name"]; found {
			t.Fatalf("per-node interface-name leaked: %#v", entry)
		}
	}
}
func TestBuildRejectsPerNodeInterfaceName(t *testing.T) {
	s := testState()
	s.Nodes[0].Config["interface-name"] = "以太网"
	if _, err := Build(s, testBuildOptions()); err == nil || !strings.Contains(err.Error(), "forbidden routing or file option") {
		t.Fatalf("per-node interface-name accepted: %v", err)
	}
	// A global pin must not whitelist the per-node key.
	s = testState()
	s.Settings.OutboundInterface = "以太网"
	s.Nodes[0].Config["interface-name"] = "以太网"
	if _, err := Build(s, testBuildOptions()); err == nil {
		t.Fatal("global pin allowed a per-node interface-name")
	}
	_ = model.DefaultState
}
