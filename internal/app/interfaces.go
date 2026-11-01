package app

import (
	"net"
	"sort"
)

// hostInterface is the administrator-facing view of one local network
// interface. It deliberately carries only what the outbound selector needs:
// the OS name that Mihomo's interface-name expects, liveness flags and usable
// address prefixes. Routes, gateways, MAC addresses and DNS state stay local.
type hostInterface struct {
	Name      string   `json:"name"`
	Up        bool     `json:"up"`
	Loopback  bool     `json:"loopback"`
	Addresses []string `json:"addresses"`
}

// hostInterfaces lists every interface on the host, including ones that are
// currently down, so the UI can explain why a saved selection stopped working.
// Link-local addresses (fe80::/10, 169.254.0.0/16) are dropped because they can
// never identify a usable outbound source.
func hostInterfaces() (any, error) {
	all, err := net.Interfaces()
	if err != nil {
		return nil, bad("读取本机网卡列表失败")
	}
	out := make([]hostInterface, 0, len(all))
	for _, iface := range all {
		item := hostInterface{
			Name:      iface.Name,
			Up:        iface.Flags&net.FlagUp != 0,
			Loopback:  iface.Flags&net.FlagLoopback != 0,
			Addresses: []string{},
		}
		addrs, addrErr := iface.Addrs()
		if addrErr == nil {
			for _, addr := range addrs {
				network, ok := addr.(*net.IPNet)
				if !ok {
					continue
				}
				if network.IP.IsLinkLocalUnicast() || network.IP.IsLinkLocalMulticast() {
					continue
				}
				item.Addresses = append(item.Addresses, network.String())
			}
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return map[string]any{"interfaces": out}, nil
}
