package core

import (
	"context"
	"net"
	"testing"

	"github.com/uniproxy/uniproxy/internal/model"
)

// The pinned core accepts interface-name without checking that the interface
// exists, so this real-core test documents the two behaviours that matter:
// a valid pin still validates, and an unusable pin is only caught by the
// management layer (internal/app.ValidateState), never by "mihomo -t".
func TestRealGlobalInterfaceNameValidation(t *testing.T) {
	m := realManager(t)
	state := model.DefaultState()
	state.Revision = 1
	state.Nodes = []model.Node{}
	state.Pools = []model.Pool{}
	state.Listeners = []model.Listener{}

	usable := ""
	all, err := net.Interfaces()
	if err != nil {
		t.Fatal(err)
	}
	for _, iface := range all {
		if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagLoopback == 0 {
			usable = iface.Name
			break
		}
	}
	if usable == "" {
		t.Skip("no up non-loopback interface on this host")
	}
	state.Settings.OutboundInterface = usable
	if err := m.Validate(context.Background(), state); err != nil {
		t.Fatalf("valid interface pin rejected by the core: %v", err)
	}

	// Documents why validation cannot be delegated to the core.
	state.Settings.OutboundInterface = "definitely-not-an-interface-xyz"
	if err := m.Validate(context.Background(), state); err != nil {
		t.Fatalf("pinned core unexpectedly rejected an unknown interface: %v", err)
	}
}
