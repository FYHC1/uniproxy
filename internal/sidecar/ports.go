package sidecar

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
)

// PortAllocator reserves local loopback ports for sidecar SOCKS inbounds. It
// never reuses a port while the allocation is live, and a bind preflight keeps
// the common "port already occupied" failure at save/apply time rather than at
// first traffic.
type PortAllocator struct {
	mu    sync.Mutex
	start int
	end   int
	used  map[int]bool
}

func NewPortAllocator(start, end int) (*PortAllocator, error) {
	if start < 1024 || end > 65535 || start > end {
		return nil, errors.New("sidecar port range must be within 1024-65535")
	}
	return &PortAllocator{start: start, end: end, used: map[int]bool{}}, nil
}

func (a *PortAllocator) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.used = map[int]bool{}
}

// Reserve marks an already assigned endpoint port as used. Persisted endpoints
// remain authoritative across restarts even when they predate the current range.
func (a *PortAllocator) Reserve(port int) error {
	if port < 1 || port > 65535 {
		return errors.New("sidecar endpoint port is invalid")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.used[port] {
		return errors.New("duplicate sidecar endpoint port")
	}
	a.used[port] = true
	return nil
}

func (a *PortAllocator) Release(port int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.used, port)
}

// Allocate returns the first free port in the configured range that can bind
// both TCP and, when requested, UDP on the loopback host.
func (a *PortAllocator) Allocate(host string, udp bool) (int, error) {
	if host != "127.0.0.1" {
		return 0, errors.New("sidecar ports may only bind 127.0.0.1")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for port := a.start; port <= a.end; port++ {
		if a.used[port] {
			continue
		}
		ok, err := portAvailable(host, port, udp)
		if err != nil {
			return 0, err
		}
		if ok {
			a.used[port] = true
			return port, nil
		}
	}
	return 0, errors.New("no free port in the sidecar port range")
}

func portAvailable(host string, port int, udp bool) (bool, error) {
	address := net.JoinHostPort(host, strconv.Itoa(port))
	listener, err := net.Listen("tcp4", address)
	if err != nil {
		return false, nil
	}
	_ = listener.Close()
	if udp {
		packet, err := net.ListenPacket("udp4", address)
		if err != nil {
			return false, nil
		}
		_ = packet.Close()
	}
	return true, nil
}

func endpointKey(host string, port int) string {
	return fmt.Sprintf("%s:%d", host, port)
}
