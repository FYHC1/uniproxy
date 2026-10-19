package model

import (
	"testing"
)

func TestRelayValidationAndUnavailableChain(t *testing.T) {
	ns := []Node{{ID: "relay", Enabled: true}, {ID: "leaf", Enabled: true, DialerID: "relay"}, {ID: "deep", Enabled: true, DialerID: "leaf"}}
	if e := ValidateRelayGraph(ns); e != nil {
		t.Fatal(e)
	}
	if len(RelayErrors(ns)) != 0 {
		t.Fatal("valid chain blocked")
	}
	ns[0].Enabled = false
	if len(RelayErrors(ns)) != 2 {
		t.Fatal("descendants not blocked")
	}
	ns[0].Enabled = true
	ns = ns[1:]
	if e := ValidateRelayGraph(ns); e != nil {
		t.Fatal("deletion should retain blocked metadata")
	}
	if len(RelayErrors(ns)) != 2 {
		t.Fatal("missing relay must block descendants")
	}
	ns[0].DialerID = "deep"
	if ValidateRelayGraph(ns) == nil {
		t.Fatal("cycle allowed")
	}
}
func TestRelayPoolMembersExcludeBrokenDependencies(t *testing.T) {
	s := DefaultState()
	s.Nodes = []Node{{ID: "r", Enabled: false}, {ID: "leaf", Enabled: true, DialerID: "r"}, {ID: "direct", Enabled: true}}
	p := Pool{Enabled: true, NodeIDs: []string{"leaf", "direct"}}
	got := PoolMembers(s, p)
	if len(got) != 1 || got[0].ID != "direct" {
		t.Fatal(got)
	}
}
