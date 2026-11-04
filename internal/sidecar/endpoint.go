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
		} else if err := alloc.Reserve(endpoint.Port); err != nil {
			return nil, err
		}
		if endpoint.Username == "" || endpoint.Password == "" {
			username, password, err := randomCredential(randReader)
			if err != nil {
				return nil, fmt.Errorf("could not generate sidecar credentials: %w", err)
			}
			endpoint.Username = username
			endpoint.Password = password
		}
		node.SidecarEndpoint = endpoint
	}
	return out, nil
}
