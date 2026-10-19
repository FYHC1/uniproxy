package importer

import (
	"encoding/base64"
	"math"
	"net/url"
	"strings"
	"testing"

	"github.com/uniproxy/uniproxy/internal/model"
)

func TestPinnedCoreSecurityBehavior(t *testing.T) {
	for _, protocol := range []string{"vless", "vmess", "trojan"} {
		t.Run("reality-ws-"+protocol, func(t *testing.T) {
			obj := nativeFixture(protocol)
			obj["tls"] = true
			obj["network"] = "ws"
			obj["reality-opts"] = map[string]any{"public-key": testPublicKey}
			requireInvalid(t, nativeDoc(t, obj))
		})
		t.Run("reality-pin-"+protocol, func(t *testing.T) {
			obj := nativeFixture(protocol)
			obj["tls"] = true
			obj["reality-opts"] = map[string]any{"public-key": testPublicKey}
			obj["fingerprint"] = strings.Repeat("ab", 32)
			requireInvalid(t, nativeDoc(t, obj))
		})
		t.Run("reality-name-"+protocol, func(t *testing.T) {
			obj := nativeFixture(protocol)
			obj["tls"] = true
			obj["reality-opts"] = map[string]any{"public-key": testPublicKey}
			obj["name-cert-verify"] = "expected.example"
			requireInvalid(t, nativeDoc(t, obj))
		})
		t.Run("reality-alpn-"+protocol, func(t *testing.T) {
			obj := nativeFixture(protocol)
			obj["tls"] = true
			obj["reality-opts"] = map[string]any{"public-key": testPublicKey}
			obj["alpn"] = []string{"h2"}
			requireInvalid(t, nativeDoc(t, obj))
		})
		t.Run("reality-grpc-"+protocol, func(t *testing.T) {
			obj := nativeFixture(protocol)
			obj["tls"] = true
			obj["network"] = "grpc"
			obj["reality-opts"] = map[string]any{"public-key": testPublicKey}
			parseOne(t, nativeDoc(t, obj))
		})
	}
	t.Run("tuic-disable-sni", func(t *testing.T) {
		obj := nativeFixture("tuic")
		obj["disable-sni"] = true
		requireInvalid(t, nativeDoc(t, obj))
		obj = singFixture("tuic")
		obj["tls"].(map[string]any)["disable_sni"] = true
		requireInvalid(t, singDoc(t, obj))
		requireInvalid(t, "tuic://token@example.com:443?disable_sni=1")
	})
	t.Run("overridden-ws-alpn", func(t *testing.T) {
		requireInvalid(t, "vless://"+testUUID+"@example.com:443?type=ws&security=tls&alpn=h2")
	})
	t.Run("overridden-h2-alpn", func(t *testing.T) {
		requireInvalid(t, "vless://"+testUUID+"@example.com:443?type=h2&security=tls&alpn=http%2F1.1")
	})
	t.Run("overridden-grpc-alpn", func(t *testing.T) { requireInvalid(t, "trojan://p@example.com:443?type=grpc&alpn=http%2F1.1") })
	t.Run("name-cert-verify", func(t *testing.T) {
		obj := nativeFixture("trojan")
		obj["name-cert-verify"] = "verified.example"
		n := parseOne(t, nativeDoc(t, obj))
		if n.Config["name-cert-verify"] != "verified.example" {
			t.Fatal("verification hostname lost")
		}
	})
	t.Run("mtu-field", func(t *testing.T) {
		for _, field := range []string{"disable-mtu-discovery", "disable_mtu_discovery"} {
			obj := nativeFixture("hysteria")
			obj[field] = true
			n := parseOne(t, nativeDoc(t, obj))
			if n.Config["disable-mtu-discovery"] != true || n.Config["disable_mtu_discovery"] != nil {
				t.Fatal("native MTU option is wrong")
			}
		}
	})
}
func TestSingboxRejectsNonstandardTLSProtocolAliases(t *testing.T) {
	for _, protocol := range []string{"https", "socks5s", "socks5h", "hy2", "ss"} {
		requireInvalid(t, singDoc(t, map[string]any{"type": protocol, "server": "example.com", "server_port": 443}))
	}
}

func TestBandwidthNormalization(t *testing.T) {
	good := map[string]string{"10": "1250000 Bps", "10 Mbps": "1250000 Bps", "1.5 Mbps": "187500 Bps", "2 mbps": "250000 Bps", "2 MBps": "2000000 Bps", "12 Kbps": "1500 Bps", "16b/s": "2 Bps", "100 Bps": "100 Bps"}
	for input, want := range good {
		t.Run(input, func(t *testing.T) {
			obj := nativeFixture("hysteria2")
			obj["up"] = input
			n := parseOne(t, nativeDoc(t, obj))
			if n.Config["up"] != want {
				t.Fatalf("wrong bandwidth mapping: got %v want %s", n.Config["up"], want)
			}
			n2 := parseOne(t, nativeDoc(t, n.Config))
			if Fingerprint(n) != Fingerprint(n2) {
				t.Fatal("bandwidth normalization is not idempotent")
			}
		})
	}
	for _, input := range []any{0, -1, 0.5, "0 Mbps", "-1 Mbps", "nonsense", "1 bps", "1e10", "9999999999999999999999999999999999999999999999 Mbps", "0.000000000001 Mbps"} {
		obj := nativeFixture("hysteria2")
		obj["up"] = input
		requireInvalid(t, nativeDoc(t, obj))
	}
}
func TestDecodedUTF8AndEmptySecurityValues(t *testing.T) {
	for _, uri := range []string{"trojan://%FF@example.com:443", "http://%FF:p@example.com", "http://example.com#%FF", "vless://" + testUUID + "@example.com:443?encryption=none&%FF=x", "vless://" + testUUID + "@example.com:443?security=tls&sni=%FF", "ssh://u:@example.com"} {
		requireInvalid(t, uri)
	}
	for _, field := range []string{"alpn", "host-key", "host-key-algorithms"} {
		protocol := "trojan"
		if field != "alpn" {
			protocol = "ssh"
		}
		obj := nativeFixture(protocol)
		obj[field] = []any{}
		requireInvalid(t, nativeDoc(t, obj))
	}
	for _, v := range []any{nil, false, 1.25, map[string]any{"x": "y"}, ""} {
		obj := vmessBase()
		obj["v"] = v
		requireInvalid(t, vmessLink(obj))
	}
	for _, port := range []any{"", true, "1.0", "+80", uint64(math.MaxUint64), nil} {
		obj := nativeFixture("http")
		obj["port"] = port
		requireInvalid(t, nativeDoc(t, obj))
	}
	for _, host := range []string{"", ".", "a..b", "-host.example", "host-.example", "host name", "host:80", strings.Repeat("a", 64) + ".example", strings.Repeat("a", 254), "https://example.com"} {
		obj := nativeFixture("http")
		obj["server"] = host
		requireInvalid(t, nativeDoc(t, obj))
	}
}
func TestPluginValidation(t *testing.T) {
	base := "ss://aes-128-gcm:p@example.com:8388?plugin="
	for _, options := range []string{
		"obfs;mode=unknown", "obfs;mode=tls;host=bad/host", "v2ray-plugin;mode=websocket;path=relative", "v2ray-plugin;mode=websocket;host=bad/host", "v2ray-plugin;mode=websocket;skip-cert-verify=1", "v2ray-plugin;mode=websocket;tls=1;fingerprint=bad", "v2ray-plugin;mode=websocket;tls=1;certificate=/private", "v2ray-plugin;mode=websocket;unknown", "v2ray-plugin;mode=websocket;v2ray-http-upgrade=1", "v2ray-plugin;mode=websocket;tls=1;name-cert-verify=bad/host",
	} {
		requireInvalid(t, base+url.QueryEscape(options))
	}
	n := parseOne(t, base+url.QueryEscape("v2ray-plugin;tls;fingerprint="+strings.Repeat("ab", 32)+";name-cert-verify=verified.example"))
	if n.Config["plugin-opts"].(map[string]any)["name-cert-verify"] != "verified.example" {
		t.Fatal("plugin verification name lost")
	}
	obj := nativeFixture("ss")
	obj["plugin"] = "v2ray-plugin"
	obj["plugin-opts"] = map[string]any{"mode": "websocket", "headers": map[string]any{"X-Header": "value"}, "tls": true, "mux": false}
	parseOne(t, nativeDoc(t, obj))
	obj["plugin-opts"].(map[string]any)["headers"] = map[string]any{"X-Header": "bad\r\nvalue"}
	requireInvalid(t, nativeDoc(t, obj))
}
func TestValidatorsRejectOutOfRangeValues(t *testing.T) {
	for _, v := range []any{uint64(math.MaxUint64), math.Inf(1), math.NaN(), float64(1.5), "--1", "", false, nil} {
		if _, ok := integerValue(v); ok {
			t.Fatalf("bad integer type accepted %T", v)
		}
	}
	for _, v := range []any{int64(40), uint64(40), float64(40), "40"} {
		if n, ok := integerValue(v); !ok || n != 40 {
			t.Fatal("valid integer rejected")
		}
	}
	if _, ok := boolValue(2); ok {
		t.Fatal("numeric bool not restricted")
	}
	for _, value := range []any{"yes", nil, 2, float64(1)} {
		r := newFields(map[string]any{"flag": value})
		r.boolean("flag", false)
		if r.finish() == nil {
			t.Fatal("bad boolean accepted")
		}
	}
	for _, key := range []string{"", "a b", "a\r\nb", strings.Repeat("x", 257)} {
		if headerName(key) {
			t.Fatal("bad header name accepted")
		}
	}
	for _, ports := range []string{"0", "65536", "1-2-3", "2-1", "a", "", strings.Repeat("1,", 129) + "1"} {
		if validatePortRanges(ports) {
			t.Fatal("bad port range accepted")
		}
	}
	for _, cipher := range []string{"2022-blake3-aes-128-gcm", "2022-blake3-aes-256-gcm", "2022-blake3-chacha20-poly1305"} {
		size := 32
		if cipher == "2022-blake3-aes-128-gcm" {
			size = 16
		}
		key := base64.StdEncoding.EncodeToString(make([]byte, size))
		r := newFields(nil)
		validateSS2022(r, cipher, key)
		if r.err != nil {
			t.Fatal("valid SS2022 key rejected")
		}
	}
	r := newFields(nil)
	validateSS2022(r, "2022-blake3-chacha20-poly1305", "a:b")
	if r.err == nil {
		t.Fatal("SS2022 chain unsupported but accepted")
	}
	r = newFields(nil)
	validateSS2022(r, "2022-blake3-aes-128-gcm", strings.Repeat("a:", 17))
	if r.err == nil {
		t.Fatal("SS2022 chain bound missing")
	}
}
func TestStableDiagnosticsWithMixedUnsafeUnknownOptions(t *testing.T) {
	content := nativeDoc(t, map[string]any{"type": "trojan", "server": "example.com", "port": 443, "password": "secret", "unknown": "value", "detour": "other"})
	first := Parse(content)
	for i := 0; i < 100; i++ {
		next := Parse(content)
		if next.Errors[0] != first.Errors[0] {
			t.Fatal("map iteration changed error category")
		}
	}
	content = "trojan://secret@example.com:443?unknown=value&detour=other"
	first = Parse(content)
	for i := 0; i < 100; i++ {
		next := Parse(content)
		if next.Errors[0] != first.Errors[0] {
			t.Fatal("query iteration changed error category")
		}
	}
}
func TestFingerprintMapOrder(t *testing.T) {
	a := model.ParsedNode{Protocol: "http", Server: "example.com", Port: 80, Config: map[string]any{"a": 1, "b": map[string]any{"x": true, "y": "secret"}}}
	b := model.ParsedNode{Protocol: "http", Server: "example.com", Port: 80, Config: map[string]any{"b": map[string]any{"y": "secret", "x": true}, "a": 1}}
	if Fingerprint(a) != Fingerprint(b) {
		t.Fatal("map insertion order affected fingerprint")
	}
}
