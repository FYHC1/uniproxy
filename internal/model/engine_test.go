package model

import (
	"errors"
	"testing"
)

func TestValidateSidecarChainPlacement(t *testing.T) {
	sidecar := Node{ID: "sidecar", Engine: EngineXray}
	native := Node{ID: "native", Engine: EngineMihomo}
	nodes := []Node{native, sidecar}

	if err := ValidateSidecarChainPlacement(nodes, []string{"sidecar", "native"}); err != nil {
		t.Fatalf("sidecar at chain head rejected: %v", err)
	}
	if err := ValidateSidecarChainPlacement(nodes, []string{"sidecar"}); err != nil {
		t.Fatalf("standalone sidecar rejected: %v", err)
	}
	if err := ValidateSidecarChainPlacement([]Node{sidecar}, []string{"sidecar", "missing"}); err != nil {
		t.Fatalf("missing hop should be checked separately: %v", err)
	}
	if err := ValidateSidecarChainPlacement(nodes, []string{"native", "sidecar"}); !errors.Is(err, ErrSidecarMustBeChainHead) {
		t.Fatalf("sidecar at chain tail error = %v", err)
	}
	if err := ValidateSidecarChainPlacement(nodes, []string{"native", "native", "sidecar"}); !errors.Is(err, ErrSidecarMustBeChainHead) {
		t.Fatalf("sidecar in chain middle error = %v", err)
	}
}

func TestNodeEngineDefaultsAndSidecarClassification(t *testing.T) {
	native := Node{}
	if native.EngineName() != EngineMihomo || native.IsSidecar() {
		t.Fatalf("zero engine is not native mihomo: %#v", native)
	}
	sidecar := Node{Engine: EngineXray}
	if sidecar.EngineName() != EngineXray || !sidecar.IsSidecar() {
		t.Fatalf("xray engine not classified as sidecar: %#v", sidecar)
	}
}
