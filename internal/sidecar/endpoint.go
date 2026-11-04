package sidecar

import (
	"fmt"

	"github.com/uniproxy/uniproxy/internal/model"
)

// AssignEndpoints returns a detached copy of nodes with a loopback SOCKS
// endpoint for every enabled Xray node. Existing endpoints are preserved;
// missing ports and credentials are generated deterministically enough for the
// caller to persist them in the encrypted state before applying Mihomo.
func (m *XrayManager) AssignEndpoints(nodes []model.Node) ([]model.Node, error) {
	m.mu.Lock()
	start, end, randReader := m.opts.PortRangeStart, m.opts.PortRangeEnd, m.opts.Rand
	m.mu.Unlock()
	alloc, err := NewPortAllocator(start, end)
	if err != nil {
		return nil, err
	}
	out := make([]model.Node, len(nodes))
	copy(out, nodes)
	for i := range out {
		if out[i].SidecarEndpoint != nil {
			endpoint := *out[i].SidecarEndpoint
			out[i].SidecarEndpoint = &endpoint
		}
	}
	// Enabled Xray endpoints are authoritative and must be unique. Reserve them
	// before looking at disabled nodes so an old disabled endpoint can never
	// force a healthy enabled node to be rejected.
	for i := range out {
		node := &out[i]
		if !node.Enabled || node.EngineName() != model.EngineXray || node.SidecarEndpoint == nil || node.SidecarEndpoint.Port == 0 {
			continue
		}
		if err := alloc.Reserve(node.SidecarEndpoint.Port); err != nil {
			return nil, err
		}
	}
	// Disabled Xray nodes keep their persisted port when possible, but historical
	// state may contain a disabled node sharing a port with an enabled one. Repair
	// that old conflict by reassigning only the disabled endpoint instead of
	// blocking every future save/apply.
	for i := range out {
		node := &out[i]
		if node.Enabled || node.EngineName() != model.EngineXray || node.SidecarEndpoint == nil || node.SidecarEndpoint.Port == 0 {
			continue
		}
		if err := alloc.Reserve(node.SidecarEndpoint.Port); err != nil {
			port, allocErr := alloc.Allocate("127.0.0.1", node.UDP)
			if allocErr != nil {
				return nil, allocErr
			}
			node.SidecarEndpoint.Port = port
		}
	}
	for i := range out {
		node := &out[i]
		if !node.Enabled {
			continue
		}
		switch node.EngineName() {
		case model.EngineMihomo:
			continue
		case model.EngineXray:
		default:
			return nil, fmt.Errorf("unsupported sidecar engine %q", node.EngineName())
		}
		endpoint := node.SidecarEndpoint
		if endpoint == nil {
			endpoint = &model.SidecarEndpoint{Host: "127.0.0.1"}
		}
		if endpoint.Host == "" {
			endpoint.Host = "127.0.0.1"
		}
		if endpoint.Port == 0 {
			port, err := alloc.Allocate("127.0.0.1", node.UDP)
			if err != nil {
				return nil, err
			}
			endpoint.Port = port
		}
		if endpoint.Username == "" || endpoint.Password == "" {
			username, password, err := randomCredential(randReader)
			if err != nil {
				return nil, fmt.Errorf("could not generate sidecar credentials: %w", err)
			}
			// Only fill the missing half; overwriting a custom username or
			// password would silently rotate credentials the operator set.
			if endpoint.Username == "" {
				endpoint.Username = username
			}
			if endpoint.Password == "" {
				endpoint.Password = password
			}
		}
		node.SidecarEndpoint = endpoint
	}
	return out, nil
}
