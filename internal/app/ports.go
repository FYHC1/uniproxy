package app

import (
	"context"
	"net"
	"strconv"
	"strings"

	"github.com/uniproxy/uniproxy/internal/model"
)

func reservedListenerPorts(s model.State, adminPort int) map[int]bool {
	used := map[int]bool{adminPort: true}
	// Disabled listeners still own their configured port, regardless of host.
	for _, listener := range s.Listeners {
		used[listener.Port] = true
	}
	return used
}

func allocateListenerPort(ctx context.Context, settings model.Settings, host string, udp bool, used map[int]bool) (int, error) {
	for port := settings.PortRangeStart; port <= settings.PortRangeEnd; port++ {
		if ctx.Err() != nil {
			return 0, bad("端口分配请求已取消")
		}
		if used[port] {
			continue
		}
		available, err := listenerPortAvailable(host, port, udp)
		if err != nil {
			return 0, err
		}
		if available {
			return port, nil
		}
	}
	return 0, bad("端口范围内没有足够的空闲端口")
}

// These probes cannot reserve the socket until the external core binds it. A
// later bind race is handled by mutate's existing saved-but-pending apply path.
func listenerPortAvailable(host string, port int, udp bool) (bool, error) {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	tcp, err := net.Listen("tcp", addr)
	if err != nil {
		if portOccupied(err) {
			return false, nil
		}
		return false, bad("无法绑定指定的本机监听地址")
	}
	defer tcp.Close()
	if udp {
		packet, err := net.ListenPacket("udp", addr)
		if err != nil {
			if portOccupied(err) {
				return false, nil
			}
			return false, bad("无法绑定指定的 UDP 监听地址")
		}
		defer packet.Close()
	}
	return true, nil
}

func portOccupied(err error) bool {
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "address already in use") || strings.Contains(text, "only one usage of each socket address") || strings.Contains(text, "10048")
}
