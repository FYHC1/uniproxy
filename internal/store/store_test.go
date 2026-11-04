package store

import (
	"encoding/json"
	"testing"

	"github.com/uniproxy/uniproxy/internal/model"
)

func TestNormalizeFillsDefaultSidecarPortRange(t *testing.T) {
	var state model.State
	Normalize(&state)
	if state.Settings.SidecarPortRangeStart != model.DefaultSidecarPortRangeStart || state.Settings.SidecarPortRangeEnd != model.DefaultSidecarPortRangeEnd {
		t.Fatalf("sidecar range = %d-%d", state.Settings.SidecarPortRangeStart, state.Settings.SidecarPortRangeEnd)
	}
}

func TestNormalizePreservesExplicitSidecarPortRange(t *testing.T) {
	state := model.DefaultState()
	state.Settings.SidecarPortRangeStart = 31000
	state.Settings.SidecarPortRangeEnd = 31999
	Normalize(&state)
	if state.Settings.SidecarPortRangeStart != 31000 || state.Settings.SidecarPortRangeEnd != 31999 {
		t.Fatalf("sidecar range = %d-%d", state.Settings.SidecarPortRangeStart, state.Settings.SidecarPortRangeEnd)
	}
}

func TestNormalizeSyncsParsedNodeEngineAfterJSONRoundTrip(t *testing.T) {
	state := model.DefaultState()
	state.Nodes = []model.Node{
		{ID: "xray", Engine: model.EngineXray, ParsedNode: model.ParsedNode{Name: "xray"}},
		{ID: "native", Engine: model.EngineMihomo, ParsedNode: model.ParsedNode{Name: "native"}},
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	var restored model.State
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	Normalize(&restored)
	if restored.Nodes[0].ParsedNode.Engine != model.EngineXray {
		t.Fatalf("xray parsed engine = %q", restored.Nodes[0].ParsedNode.Engine)
	}
	if restored.Nodes[1].ParsedNode.Engine != "" {
		t.Fatalf("legacy mihomo fingerprint input changed: %q", restored.Nodes[1].ParsedNode.Engine)
	}
}
