package app

import (
	"net"
	"strings"
	"testing"

	"github.com/uniproxy/uniproxy/internal/model"
)

// usableInterface returns a real, currently-up, non-loopback interface name so
// the positive cases never hardcode an adapter name from one machine.
func usableInterface(t *testing.T) string {
	t.Helper()
	all, err := net.Interfaces()
	if err != nil {
		t.Fatal(err)
	}
	for _, iface := range all {
		if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagLoopback == 0 {
			return iface.Name
		}
	}
	t.Skip("no up non-loopback interface on this host")
	return ""
}
func downInterface(t *testing.T) string {
	t.Helper()
	all, err := net.Interfaces()
	if err != nil {
		t.Fatal(err)
	}
	for _, iface := range all {
		if iface.Flags&net.FlagUp == 0 {
			return iface.Name
		}
	}
	t.Skip("no down interface on this host")
	return ""
}
func TestValidateStateOutboundInterface(t *testing.T) {
	base := model.DefaultState()
	if err := ValidateState(base, 9090); err != nil {
		t.Fatalf("default state rejected: %v", err)
	}
	cases := []struct{ name, value, want string }{
		{"automatic", "", ""},
		{"missing", "definitely-not-an-interface-xyz", "全局出站网卡不存在或名称不正确"},
		{"loopback", "Loopback Pseudo-Interface 1", "全局出站网卡"},
		{"padded", " " + usableInterface(t) + " ", "全局出站网卡名称不能包含首尾空白"},
		{"usable", usableInterface(t), ""},
	}
	if down := downInterface(t); down != "" {
		cases = append(cases, struct{ name, value, want string }{"down", down, "全局出站网卡当前未启用"})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := base
			s.Settings.OutboundInterface = tc.value
			err := ValidateState(s, 9090)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("unexpected rejection: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
}
func TestSettingsOutboundInterfaceRoundTrip(t *testing.T) {
	a, _ := fixture(t, true)
	name := usableInterface(t)
	code, out := request(t, a, "PATCH", "/api/settings", map[string]any{"outboundInterface": name})
	if code != 200 {
		t.Fatalf("patch %d %#v", code, out)
	}
	saved := a.store.Snapshot()
	if saved.Settings.OutboundInterface != name {
		t.Fatalf("not persisted: %q", saved.Settings.OutboundInterface)
	}
	code, out = request(t, a, "GET", "/api/bootstrap", nil)
	settings, _ := out["settings"].(map[string]any)
	if code != 200 || settings["outboundInterface"] != name {
		t.Fatalf("bootstrap missing interface %d %#v", code, settings)
	}
	// Reverting to automatic must persist the empty string, not leave the pin.
	code, out = request(t, a, "PATCH", "/api/settings", map[string]any{"outboundInterface": ""})
	if code != 200 {
		t.Fatalf("reset %d %#v", code, out)
	}
	if v := a.store.Snapshot().Settings.OutboundInterface; v != "" {
		t.Fatalf("reset not persisted: %q", v)
	}
}
func TestSettingsOutboundInterfaceRejected(t *testing.T) {
	a, f := fixture(t, true)
	code, out := request(t, a, "PATCH", "/api/settings", map[string]any{"outboundInterface": "definitely-not-an-interface-xyz"})
	msg, _ := out["error"].(string)
	if code != 400 || !strings.Contains(msg, "全局出站网卡") {
		t.Fatalf("invalid interface accepted %d %#v", code, out)
	}
	if v := a.store.Snapshot().Settings.OutboundInterface; v != "" {
		t.Fatalf("invalid value persisted: %q", v)
	}
	if f.validateCalls != 0 {
		t.Fatalf("kernel validation ran for a rejected setting: %d", f.validateCalls)
	}
	code, out = request(t, a, "PATCH", "/api/settings", map[string]any{"outboundInterface2": "x"})
	msg, _ = out["error"].(string)
	if code != 400 || !strings.Contains(msg, "不允许修改字段") {
		t.Fatalf("unknown field accepted %d %#v", code, out)
	}
}
func TestInterfacesEndpoint(t *testing.T) {
	a, _ := fixture(t, false)
	code, out := request(t, a, "GET", "/api/interfaces", nil)
	if code != 200 {
		t.Fatalf("interfaces %d %#v", code, out)
	}
	list, ok := out["interfaces"].([]any)
	if !ok || len(list) == 0 {
		t.Fatalf("missing interfaces %#v", out)
	}
	seenLoopback, seenUsable := false, false
	for _, raw := range list {
		item, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("bad entry %#v", raw)
		}
		name, _ := item["name"].(string)
		up, upOK := item["up"].(bool)
		loop, loopOK := item["loopback"].(bool)
		addrs, addrOK := item["addresses"].([]any)
		if name == "" || !upOK || !loopOK || !addrOK {
			t.Fatalf("incomplete entry %#v", item)
		}
		if loop {
			seenLoopback = true
		}
		if up && !loop {
			seenUsable = true
		}
		for _, addr := range addrs {
			text, _ := addr.(string)
			if strings.Contains(text, "fe80:") || strings.HasPrefix(text, "169.254.") {
				t.Fatalf("link-local address leaked: %q", text)
			}
			if strings.Contains(text, "mac") || strings.Contains(text, "gateway") {
				t.Fatalf("unexpected field: %q", text)
			}
		}
	}
	if !seenLoopback || !seenUsable {
		t.Fatalf("expected both loopback and usable interfaces, loop=%v usable=%v", seenLoopback, seenUsable)
	}
}
