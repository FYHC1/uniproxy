package model

import "errors"

// ErrSidecarMustBeChainHead is the stable public message for an invalid local
// sidecar position. A local 127.0.0.1 sidecar is only reachable from Mihomo on
// this host; a previous remote hop would resolve 127.0.0.1 on the wrong machine.
var ErrSidecarMustBeChainHead = errors.New("sidecar 节点只允许位于链首")

// ValidateSidecarChainPlacement enforces that every sidecar node in a chain is
// the first hop. Missing nodes are ignored here: disabled chains keep their
// ordered identity after a hop is deleted, and ValidateState reports missing
// hops separately. Unknown engines are rejected by node validation, not here.
func ValidateSidecarChainPlacement(nodes []Node, chainNodeIDs []string) error {
	byID := make(map[string]Node, len(nodes))
	for _, n := range nodes {
		byID[n.ID] = n
	}
	for index, id := range chainNodeIDs {
		if index == 0 {
			continue
		}
		n, ok := byID[id]
		if !ok {
			continue
		}
		if n.IsSidecar() {
			return ErrSidecarMustBeChainHead
		}
	}
	return nil
}
