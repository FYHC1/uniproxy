package app

import (
	"context"
	"net"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/uniproxy/uniproxy/internal/model"
)

func TestPoolWithListenerPortConflictsAndExhaustion(t *testing.T) {
	for _, test := range []struct {
		name       string
		manual     bool
		udp        bool
		configured bool
	}{
		{name: "auto-skips-all", udp: true, configured: true},
		{name: "auto-exhausted", udp: true, configured: true},
		{name: "manual-admin", manual: true},
		{name: "manual-enabled-config", manual: true, configured: true},
		{name: "manual-disabled-config", manual: true, configured: true},
		{name: "manual-tcp-occupied", manual: true},
		{name: "manual-udp-occupied", manual: true, udp: true},
		{name: "tcp-only-ignores-udp-occupied", manual: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			a, f := fixture(t, true)
			base := poolTestPortRange(t, 6)
			s := a.store.Snapshot()
			s.Settings.PortRangeStart, s.Settings.PortRangeEnd = base, base+5
			a.opts.AdminPort = base
			if test.name == "auto-exhausted" {
				s.Settings.PortRangeEnd = base + 4
			}
			if test.configured {
				s.Nodes = []model.Node{{ID: "reserved-node", Enabled: true, ParsedNode: model.ParsedNode{Name: "reserved", Protocol: "http", Server: "127.0.0.1", Port: 1080, Config: map[string]any{"type": "http", "server": "127.0.0.1", "port": 1080}}}}
				s.Listeners = []model.Listener{
					{ID: "configured-live", Name: "configured enabled", Kind: "node", TargetID: "reserved-node", Enabled: true, Protocol: "mixed", Host: "127.0.0.2", Port: base + 1},
					{ID: "configured-disabled", Name: "configured disabled", Kind: "node", TargetID: "reserved-node", Enabled: false, Protocol: "http", Host: "127.0.0.2", Port: base + 2},
				}
			}
			if err := ValidateState(s, a.opts.AdminPort); err != nil {
				t.Fatal(err)
			}
			if err := a.store.Save(s); err != nil {
				t.Fatal(err)
			}
			tcp, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(base+3)))
			if err != nil {
				t.Fatal(err)
			}
			defer tcp.Close()
			udp, err := net.ListenPacket("udp", net.JoinHostPort("127.0.0.1", strconv.Itoa(base+4)))
			if err != nil {
				t.Fatal(err)
			}
			defer udp.Close()
			body := poolTestBody()
			body["pool"].(map[string]any)["udp"] = test.udp
			listener := body["listener"].(map[string]any)
			listener["udp"] = test.udp
			port := 0
			switch test.name {
			case "manual-admin":
				port = base
			case "manual-enabled-config":
				port = base + 1
			case "manual-disabled-config":
				port = base + 2
			case "manual-tcp-occupied":
				port = base + 3
			case "manual-udp-occupied", "tcp-only-ignores-udp-occupied":
				port = base + 4
			}
			if test.manual {
				listener["port"] = port
			}
			before := a.store.Snapshot()
			code, out := request(t, a, "POST", "/api/pools/with-listener", body)
			success := test.name == "auto-skips-all" || test.name == "tcp-only-ignores-udp-occupied"
			if !success {
				if code != 400 || !reflect.DeepEqual(a.store.Snapshot(), before) {
					t.Fatalf("port collision/exhaustion committed a partial pair: %d", code)
				}
				if test.name == "auto-exhausted" && !strings.Contains(out["error"].(string), "空闲端口") {
					t.Fatal("exhaustion did not report an allocation failure")
				}
				poolTestAssertCalls(t, f, 0, 0)
				return
			}
			if code != 200 {
				t.Fatalf("valid allocation failed: %v", out["error"])
			}
			result := poolTestDecode[poolTestCreated](t, out["data"])
			want := base + 5
			if test.manual {
				want = port
			}
			if result.Listener.Port != want {
				t.Fatalf("port %d, want %d", result.Listener.Port, want)
			}
			poolTestAssertCalls(t, f, 1, 1)
			// Both probes were closed, including TCP after a failed UDP candidate.
			check, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(base+4)))
			if err != nil {
				t.Fatal("TCP probe leaked a socket after UDP conflict", err)
			}
			_ = check.Close()
		})
	}
}

func TestPoolWithListenerManualPortAndProtocolChoices(t *testing.T) {
	for _, protocol := range []string{"mixed", "http", "socks"} {
		t.Run(protocol, func(t *testing.T) {
			a, f := fixture(t, true)
			base := poolTestPortRange(t, 2)
			// The allocation range constrains auto allocation only.
			poolTestSetRange(t, a, base, base)
			body := poolTestBody()
			body["pool"].(map[string]any)["udp"] = true
			body["listener"] = map[string]any{"name": "custom entrance", "protocol": protocol, "port": base + 1, "enabled": false}
			code, out := request(t, a, "POST", "/api/pools/with-listener", body)
			if code != 200 {
				t.Fatal(out["error"])
			}
			result := poolTestDecode[poolTestCreated](t, out["data"])
			if result.Listener.Port != base+1 || result.Listener.Protocol != protocol || result.Listener.Enabled || result.Listener.UDP || result.Listener.Name != "custom entrance" {
				t.Fatal("manual settings lost or pool UDP implicitly enabled listener UDP")
			}
			poolTestAssertCalls(t, f, 1, 1)
		})
	}
}

func TestPoolWithListenerRejectsHostAndProtocolBeforeAllocation(t *testing.T) {
	for _, test := range []struct {
		name     string
		listener map[string]any
		message  string
	}{
		{"hostname", map[string]any{"host": "localhost"}, "必须是 IP"},
		{"unresolvable", map[string]any{"host": "unresolvable.invalid"}, "必须是 IP"},
		{"host-port", map[string]any{"host": "127.0.0.1:1234"}, "必须是 IP"},
		{"bracketed-ip", map[string]any{"host": "[::1]"}, "必须是 IP"},
		{"bad-protocol", map[string]any{"protocol": "socks5"}, "协议/UDP"},
		{"http-udp", map[string]any{"protocol": "http", "udp": true}, "协议/UDP"},
		{"public-udp", map[string]any{"host": "0.0.0.0", "udp": true, "users": []model.Credential{{Username: "u", Password: "fixture-password"}}}, "仅允许本机回环"},
		{"public-no-auth", map[string]any{"host": "0.0.0.0"}, "配置认证"},
	} {
		t.Run(test.name, func(t *testing.T) {
			a, f := fixture(t, true)
			// No allocatable port: these requests must still report prevalidation,
			// not allocation exhaustion or any network/DNS error.
			poolTestSetRange(t, a, a.opts.AdminPort, a.opts.AdminPort)
			body := poolTestBody()
			body["pool"].(map[string]any)["udp"] = true
			body["listener"] = test.listener
			before := a.store.Snapshot()
			code, out := request(t, a, "POST", "/api/pools/with-listener", body)
			if code != 400 || !strings.Contains(out["error"].(string), test.message) {
				t.Fatalf("prevalidation was not performed before allocation: %d %v", code, out["error"])
			}
			if !reflect.DeepEqual(a.store.Snapshot(), before) {
				t.Fatal("invalid request changed state")
			}
			poolTestAssertCalls(t, f, 0, 0)
		})
	}
}

func TestPoolWithListenerAuthUDPAndHashValidation(t *testing.T) {
	validUsers := []model.Credential{{Username: "fixture-user", Password: "test-only-password"}}
	for _, test := range []struct {
		name     string
		pool     map[string]any
		listener map[string]any
		accepted bool
	}{
		{"missing-password", nil, map[string]any{"users": []model.Credential{{Username: "u"}}}, false},
		{"metadata-is-not-password", nil, map[string]any{"users": []model.Credential{{Username: "u", HasPassword: true}}}, false},
		{"duplicate-user", nil, map[string]any{"users": []model.Credential{{Username: "u", Password: "a"}, {Username: "u", Password: "b"}}}, false},
		{"empty-user", nil, map[string]any{"users": []model.Credential{{Password: "a"}}}, false},
		{"colon-user", nil, map[string]any{"users": []model.Credential{{Username: "u:v", Password: "a"}}}, false},
		{"control-password", nil, map[string]any{"users": []model.Credential{{Username: "u", Password: "a\n"}}}, false},
		{"long-password", nil, map[string]any{"users": []model.Credential{{Username: "u", Password: strings.Repeat("p", 256)}}}, false},
		{"public-tcp-auth", nil, map[string]any{"host": "0.0.0.0", "users": validUsers}, true},
		{"ipv6-public-no-auth", nil, map[string]any{"host": "::"}, false},
		{"udp-without-pool-udp", nil, map[string]any{"protocol": "socks", "udp": true}, false},
		{"explicit-mixed-udp", map[string]any{"udp": true}, map[string]any{"udp": true}, true},
		{"explicit-socks-udp", map[string]any{"udp": true}, map[string]any{"protocol": "socks", "udp": true}, true},
		{"public-udp-auth", map[string]any{"udp": true}, map[string]any{"host": "0.0.0.0", "udp": true, "users": validUsers}, false},
		{"hash-url-test", map[string]any{"hashKey": "in-user"}, map[string]any{"users": validUsers}, false},
		{"hash-round-robin", map[string]any{"strategy": "round-robin", "hashKey": "in-user"}, map[string]any{"users": validUsers}, false},
		{"unknown-hash", map[string]any{"strategy": "consistent-hashing", "hashKey": "dynamic-session"}, map[string]any{"users": validUsers}, false},
		{"hash-without-auth", map[string]any{"strategy": "consistent-hashing", "hashKey": "in-user"}, nil, false},
		{"hash-with-udp", map[string]any{"strategy": "sticky-sessions", "hashKey": "in-user", "udp": true}, map[string]any{"udp": true, "users": validUsers}, false},
		{"hash-consistent-auth-tcp", map[string]any{"strategy": "consistent-hashing", "hashKey": "in-user"}, map[string]any{"users": validUsers}, true},
		{"hash-sticky-auth-tcp", map[string]any{"strategy": "sticky-sessions", "hashKey": "in-user"}, map[string]any{"users": validUsers}, true},
		{"disabled-hash-no-auth", map[string]any{"strategy": "consistent-hashing", "hashKey": "in-user"}, map[string]any{"enabled": false}, true},
		{"invalid-pool-ref", map[string]any{"nodeIds": []string{"missing-node"}}, nil, false},
		{"invalid-group-ref", map[string]any{"groupIds": []string{"missing-group"}}, nil, false},
		{"empty-pool-name", map[string]any{"name": " "}, nil, false},
		{"bad-listener-name", nil, map[string]any{"name": strings.Repeat("x", 161)}, false},
		{"negative-port", nil, map[string]any{"port": -1}, false},
		{"privileged-port", nil, map[string]any{"port": 1023}, false},
		{"port-overflow", nil, map[string]any{"port": 65536}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			a, f := fixture(t, true)
			port := poolTestPortRange(t, 1)
			poolTestSetRange(t, a, port, port)
			body := poolTestBody()
			for k, v := range test.pool {
				body["pool"].(map[string]any)[k] = v
			}
			for k, v := range test.listener {
				body["listener"].(map[string]any)[k] = v
			}
			before := a.store.Snapshot()
			code, out := request(t, a, "POST", "/api/pools/with-listener", body)
			if !test.accepted {
				if code != 400 || !reflect.DeepEqual(before, a.store.Snapshot()) {
					t.Fatalf("invalid security configuration accepted/partially saved: %d", code)
				}
				poolTestAssertCalls(t, f, 0, 0)
				return
			}
			if code != 200 {
				t.Fatalf("valid security configuration rejected: %v", out["error"])
			}
			poolTestAssertCalls(t, f, 1, 1)
			result := poolTestDecode[poolTestCreated](t, out["data"])
			if result.Listener.TargetID != result.Pool.ID || result.Listener.Kind != "pool" {
				t.Fatal("security path lost server binding")
			}
			for _, user := range result.Listener.Users {
				if user.Password != "" || !user.HasPassword {
					t.Fatal("public listener user projection is unsafe")
				}
			}
		})
	}
}

func TestPoolWithListenerCancelledAllocation(t *testing.T) {
	a, f := fixture(t, true)
	base := poolTestPortRange(t, 1)
	poolTestSetRange(t, a, base, base)
	before := a.store.Snapshot()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := allocateListenerPort(ctx, before.Settings, "127.0.0.1", true, reservedListenerPorts(before, a.opts.AdminPort))
	if err == nil || !strings.Contains(err.Error(), "已取消") {
		t.Fatal("cancelled allocation continued")
	}
	poolTestAssertCalls(t, f, 0, 0)
	if !reflect.DeepEqual(a.store.Snapshot(), before) {
		t.Fatal("cancelled allocation changed state")
	}
}

func TestBatchListenerSharedAllocatorStillAtomic(t *testing.T) {
	a, f := fixture(t, true)
	code, out := request(t, a, "POST", "/api/import", map[string]any{"mode": "text", "content": "socks5://127.0.0.1:1080?udp=true#one\nsocks5://127.0.0.1:1081?udp=true#two"})
	if code != 200 {
		t.Fatal(out["error"])
	}
	base := poolTestPortRange(t, 2)
	poolTestSetRange(t, a, base, base)
	s := a.store.Snapshot()
	ids := []string{s.Nodes[0].ID, s.Nodes[1].ID}
	v, ap := poolTestCoreCounts(f)
	body := map[string]any{"nodeIds": ids, "protocol": "mixed", "udp": true}
	code, _ = request(t, a, "POST", "/api/listeners/batch", body)
	if code != 400 || !reflect.DeepEqual(a.store.Snapshot(), s) {
		t.Fatal("batch allocation exhaustion partially saved listeners")
	}
	poolTestAssertCalls(t, f, v, ap)
	poolTestSetRange(t, a, base, base+1)
	code, out = request(t, a, "POST", "/api/listeners/batch", body)
	if code != 200 {
		t.Fatal(out["error"])
	}
	listeners := poolTestDecode[[]model.Listener](t, out["data"])
	if len(listeners) != 2 || listeners[0].Port != base || listeners[1].Port != base+1 || !listeners[0].UDP || !listeners[1].UDP {
		t.Fatal("shared allocator changed legacy batch behavior")
	}
	poolTestAssertCalls(t, f, v+1, ap+1)
}
