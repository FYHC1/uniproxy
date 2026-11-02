package importer

import (
	"encoding/base64"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func requireConfig(t *testing.T, node map[string]any, key string, want any) {
	t.Helper()
	if !reflect.DeepEqual(node[key], want) {
		t.Fatalf("config %s = %#v, want %#v", key, node[key], want)
	}
}

func TestModernVLESSURI(t *testing.T) {
	t.Run("xhttp-extra", func(t *testing.T) {
		extra := `{"xPaddingBytes":"100-200","xPaddingObfsMode":true,"xPaddingKey":"pad","xPaddingPlacement":"header","xPaddingMethod":"base64","scMaxEachPostBytes":"1000000","scMinPostsIntervalMs":"10-20","xmux":{"maxConcurrency":"8-16","maxConnections":4},"downloadSettings":{"path":"/dl","host":"dl.example","server":"dl.example","port":8443,"tls":true,"serverName":"dl.example","skipCertVerify":false}}`
		q := url.Values{}
		q.Set("security", "tls")
		q.Set("type", "xhttp")
		q.Set("path", "/split")
		q.Set("host", "front.example")
		q.Set("mode", "stream-up")
		q.Set("extra", extra)
		n := parseOne(t, "vless://"+testUUID+"@example.com:443?"+q.Encode())
		requireConfig(t, n.Config, "network", "xhttp")
		opts, ok := n.Config["xhttp-opts"].(map[string]any)
		if !ok {
			t.Fatalf("missing xhttp opts: %#v", n.Config)
		}
		if opts["path"] != "/split" || opts["host"] != "front.example" || opts["mode"] != "stream-up" {
			t.Fatalf("outer XHTTP identity lost: %#v", opts)
		}
		if opts["x-padding-bytes"] != "100-200" || opts["x-padding-obfs-mode"] != true {
			t.Fatalf("padding mapping wrong: %#v", opts)
		}
		reuse, ok := opts["reuse-settings"].(map[string]any)
		if !ok || reuse["max-concurrency"] != "8-16" || reuse["max-connections"] != "4" {
			t.Fatalf("xmux mapping wrong: %#v", opts)
		}
		download, ok := opts["download-settings"].(map[string]any)
		if !ok || download["path"] != "/dl" || download["host"] != "dl.example" || download["server"] != "dl.example" || download["port"] != 8443 || download["tls"] != true || download["servername"] != "dl.example" {
			t.Fatalf("download settings mapping wrong: %#v", download)
		}
	})
	t.Run("xhttp-extra-outer-wins", func(t *testing.T) {
		extra := `{"path":"/inside","host":"inside.example","mode":"packet-up","xPaddingBytes":100}`
		q := url.Values{}
		q.Set("security", "tls")
		q.Set("type", "xhttp")
		q.Set("path", "/outside")
		q.Set("host", "outside.example")
		q.Set("mode", "auto")
		q.Set("extra", extra)
		n := parseOne(t, "vless://"+testUUID+"@example.com:443?"+q.Encode())
		opts := n.Config["xhttp-opts"].(map[string]any)
		if opts["path"] != "/outside" || opts["host"] != "outside.example" || opts["mode"] != "auto" {
			t.Fatalf("outer values did not win: %#v", opts)
		}
	})
	t.Run("vision-with-encryption", func(t *testing.T) {
		q := url.Values{}
		q.Set("security", "none")
		q.Set("type", "ws")
		q.Set("path", "/ws")
		q.Set("flow", "xtls-rprx-vision")
		q.Set("encryption", "mlkem768x25519plus.native.0rtt.100-111-1111.75-0-111.50-0-3333.ptjHQxBQxTJ9MWr2cd5qWIflBSACHOevTauCQwa_71U")
		n := parseOne(t, "vless://"+testUUID+"@example.com:443?"+q.Encode())
		requireConfig(t, n.Config, "flow", "xtls-rprx-vision")
		requireConfig(t, n.Config, "network", "ws")
		requireConfig(t, n.Config, "tls", false)
		if n.Config["encryption"] == "" {
			t.Fatalf("VLESS encryption was not preserved: %#v", n.Config)
		}
	})
	t.Run("vision-without-tls-or-encryption-rejected", func(t *testing.T) {
		requireInvalid(t, "vless://"+testUUID+"@example.com:443?security=none&type=ws&path=%2Fws&flow=xtls-rprx-vision")
	})
	t.Run("xhttp-extra-inner-fallback", func(t *testing.T) {
		extra := `{"path":"/inside","host":"inside.example","mode":"packet-up"}`
		q := url.Values{}
		q.Set("security", "tls")
		q.Set("type", "xhttp")
		q.Set("extra", extra)
		n := parseOne(t, "vless://"+testUUID+"@example.com:443?"+q.Encode())
		opts := n.Config["xhttp-opts"].(map[string]any)
		if opts["path"] != "/inside" || opts["host"] != "inside.example" || opts["mode"] != "packet-up" {
			t.Fatalf("inner fallback lost: %#v", opts)
		}
	})
	t.Run("xhttp-defaults", func(t *testing.T) {
		n := parseOne(t, "vless://"+testUUID+"@example.com:443?security=tls&type=xhttp")
		opts := n.Config["xhttp-opts"].(map[string]any)
		if opts["path"] != "/" || opts["mode"] != nil {
			t.Fatalf("unexpected defaults: %#v", opts)
		}
	})
	t.Run("xhttp-stream-one-download-rejected", func(t *testing.T) {
		extra := `{"downloadSettings":{"path":"/dl"}}`
		q := url.Values{}
		q.Set("security", "tls")
		q.Set("type", "xhttp")
		q.Set("mode", "stream-one")
		q.Set("extra", extra)
		requireInvalid(t, "vless://"+testUUID+"@example.com:443?"+q.Encode())
	})
	t.Run("httpupgrade", func(t *testing.T) {
		q := url.Values{}
		q.Set("security", "tls")
		q.Set("type", "httpupgrade")
		q.Set("path", "/up")
		q.Set("host", "up.example")
		n := parseOne(t, "vless://"+testUUID+"@example.com:443?"+q.Encode())
		requireConfig(t, n.Config, "network", "ws")
		ws := n.Config["ws-opts"].(map[string]any)
		if ws["v2ray-http-upgrade"] != true || ws["path"] != "/up" {
			t.Fatalf("httpupgrade mapping wrong: %#v", ws)
		}
		if headers, ok := ws["headers"].(map[string]any); !ok || headers["Host"] != "up.example" {
			t.Fatalf("httpupgrade host lost: %#v", ws)
		}
	})
	t.Run("vless-encryption", func(t *testing.T) {
		key := base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))
		value := "mlkem768x25519plus.native.0rtt." + key
		q := url.Values{}
		q.Set("security", "tls")
		q.Set("encryption", value)
		n := parseOne(t, "vless://"+testUUID+"@example.com:443?"+q.Encode())
		requireConfig(t, n.Config, "encryption", value)
	})
	t.Run("vless-encryption-invalid", func(t *testing.T) {
		for _, value := range []string{
			"mlkem768x25519plus.native.0rtt",
			"mlkem768x25519plus.native.0rtt.not-a-key",
			"mlkem768x25519plus.bad.0rtt." + base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("k", 32))),
		} {
			q := url.Values{}
			q.Set("security", "tls")
			q.Set("encryption", value)
			requireInvalid(t, "vless://"+testUUID+"@example.com:443?"+q.Encode())
		}
	})
	t.Run("ech-vcn-pcs", func(t *testing.T) {
		ech := base64.StdEncoding.EncodeToString([]byte("ech-config"))
		q := url.Values{}
		q.Set("security", "tls")
		q.Set("ech", ech)
		q.Set("vcn", "verify.example")
		q.Set("pcs", strings.Repeat("ab", 32))
		n := parseOne(t, "vless://"+testUUID+"@example.com:443?"+q.Encode())
		opts := n.Config["ech-opts"].(map[string]any)
		if opts["enable"] != true || opts["config"] != ech {
			t.Fatalf("ECH mapping wrong: %#v", opts)
		}
		requireConfig(t, n.Config, "name-cert-verify", "verify.example")
		requireConfig(t, n.Config, "fingerprint", strings.Repeat("ab", 32))
	})
	t.Run("ech-without-tls-rejected", func(t *testing.T) {
		q := url.Values{}
		q.Set("security", "none")
		q.Set("ech", base64.StdEncoding.EncodeToString([]byte("cfg")))
		requireInvalid(t, "vless://"+testUUID+"@example.com:443?"+q.Encode())
	})
	t.Run("reality-ech-conflict-rejected", func(t *testing.T) {
		q := url.Values{}
		q.Set("security", "reality")
		q.Set("pbk", testPublicKey)
		q.Set("ech", base64.StdEncoding.EncodeToString([]byte("cfg")))
		requireInvalid(t, "vless://"+testUUID+"@example.com:443?"+q.Encode())
	})
	t.Run("unsupported-xray-fields", func(t *testing.T) {
		for _, key := range []string{"spx", "pqv", "fm"} {
			q := url.Values{}
			q.Set("security", "tls")
			q.Set(key, "value")
			requireInvalid(t, "vless://"+testUUID+"@example.com:443?"+q.Encode())
		}
	})
}

func TestModernXHTTPDownloadSchema(t *testing.T) {
	t.Run("xray-schema", func(t *testing.T) {
		extra := `{"downloadSettings":{"address":"dl.example","port":8443,"security":"tls","tlsSettings":{"serverName":"dl.example","allowInsecure":false,"fingerprint":"chrome","alpn":["h2"]},"xhttpSettings":{"path":"/dl","host":"dl.example","headers":{"X-Test":"1"},"extra":{"xmux":{"maxConcurrency":"4-8"}}}}}`
		q := url.Values{}
		q.Set("security", "tls")
		q.Set("type", "xhttp")
		q.Set("path", "/split")
		q.Set("host", "front.example")
		q.Set("extra", extra)
		n := parseOne(t, "vless://"+testUUID+"@example.com:443?"+q.Encode())
		opts := n.Config["xhttp-opts"].(map[string]any)
		download, ok := opts["download-settings"].(map[string]any)
		if !ok {
			t.Fatalf("missing Xray download settings: %#v", opts)
		}
		if download["server"] != "dl.example" || download["port"] != 8443 || download["tls"] != true || download["servername"] != "dl.example" || download["client-fingerprint"] != "chrome" {
			t.Fatalf("Xray download identity mapping wrong: %#v", download)
		}
		if download["path"] != "/dl" || download["host"] != "dl.example" {
			t.Fatalf("Xray download xhttp mapping wrong: %#v", download)
		}
		if download["skip-cert-verify"] != false {
			t.Fatalf("download certificate inheritance was not disabled: %#v", download)
		}
		if reality, ok := download["reality-opts"].(map[string]any); !ok || len(reality) != 0 {
			t.Fatalf("download Reality inheritance was not disabled: %#v", download)
		}
		if reuse, ok := download["reuse-settings"].(map[string]any); !ok || reuse["max-concurrency"] != "4-8" {
			t.Fatalf("Xray download xmux mapping wrong: %#v", download)
		}
	})
	t.Run("inactive-tls-blocks-ignored", func(t *testing.T) {
		extra := `{"downloadSettings":{"security":"none","tlsSettings":{"serverName":"bad host","fingerprint":"bogus"},"realitySettings":{"publicKey":"bad"}}}`
		q := url.Values{}
		q.Set("security", "tls")
		q.Set("type", "xhttp")
		q.Set("extra", extra)
		n := parseOne(t, "vless://"+testUUID+"@example.com:443?"+q.Encode())
		opts := n.Config["xhttp-opts"].(map[string]any)
		download, ok := opts["download-settings"].(map[string]any)
		if !ok || download["tls"] != false {
			t.Fatalf("inactive TLS blocks were not ignored: %#v", download)
		}
	})
}

func TestModernVMessURI(t *testing.T) {
	t.Run("kcp", func(t *testing.T) {
		obj := vmessBase()
		obj["net"] = "kcp"
		obj["type"] = "srtp"
		obj["path"] = "seed-value"
		obj["host"] = ""
		n := parseOne(t, vmessLink(obj))
		requireConfig(t, n.Config, "network", "kcp")
		opts := n.Config["kcp-opts"].(map[string]any)
		if opts["header"] != "srtp" || opts["seed"] != "seed-value" {
			t.Fatalf("KCP mapping wrong: %#v", opts)
		}
	})
	t.Run("insecure-alias", func(t *testing.T) {
		obj := vmessBase()
		obj["insecure"] = "1"
		n := parseOne(t, vmessLink(obj))
		requireConfig(t, n.Config, "skip-cert-verify", true)
	})
	t.Run("httpupgrade", func(t *testing.T) {
		obj := vmessBase()
		obj["net"] = "httpupgrade"
		obj["type"] = "none"
		obj["path"] = "/up"
		obj["host"] = "up.example"
		n := parseOne(t, vmessLink(obj))
		requireConfig(t, n.Config, "network", "ws")
		ws := n.Config["ws-opts"].(map[string]any)
		if ws["v2ray-http-upgrade"] != true || ws["path"] != "/up" {
			t.Fatalf("VMess httpupgrade mapping wrong: %#v", ws)
		}
	})
	t.Run("xhttp-rejected", func(t *testing.T) {
		obj := vmessBase()
		obj["net"] = "xhttp"
		requireInvalid(t, vmessLink(obj))
	})
}

func TestModernNativeOptions(t *testing.T) {
	t.Run("native-xhttp", func(t *testing.T) {
		obj := nativeFixture("vless")
		obj["network"] = "xhttp"
		obj["xhttp-opts"] = map[string]any{
			"path":           "/native",
			"mode":           "auto",
			"reuse-settings": map[string]any{"max-concurrency": "4-8"},
		}
		n := parseOne(t, nativeDoc(t, obj))
		opts := n.Config["xhttp-opts"].(map[string]any)
		if opts["path"] != "/native" || opts["mode"] != "auto" {
			t.Fatalf("native XHTTP mapping wrong: %#v", opts)
		}
		if reuse, ok := opts["reuse-settings"].(map[string]any); !ok || reuse["max-concurrency"] != "4-8" {
			t.Fatalf("native XHTTP reuse mapping wrong: %#v", opts)
		}
	})
	t.Run("native-xhttp-stream-one-download-rejected", func(t *testing.T) {
		obj := nativeFixture("vless")
		obj["network"] = "xhttp"
		obj["xhttp-opts"] = map[string]any{"mode": "stream-one", "download-settings": map[string]any{"path": "/dl"}}
		requireInvalid(t, nativeDoc(t, obj))
	})
	t.Run("native-httpupgrade", func(t *testing.T) {
		obj := nativeFixture("vless")
		obj["network"] = "httpupgrade"
		obj["ws-opts"] = map[string]any{"path": "/up"}
		n := parseOne(t, nativeDoc(t, obj))
		requireConfig(t, n.Config, "network", "ws")
		ws := n.Config["ws-opts"].(map[string]any)
		if ws["v2ray-http-upgrade"] != true || ws["path"] != "/up" {
			t.Fatalf("native httpupgrade mapping wrong: %#v", ws)
		}
	})
	t.Run("native-kcp", func(t *testing.T) {
		obj := nativeFixture("vmess")
		obj["network"] = "kcp"
		obj["kcp-opts"] = map[string]any{"header": "wechat-video", "seed": "seed", "mtu": 1300}
		n := parseOne(t, nativeDoc(t, obj))
		requireConfig(t, n.Config, "network", "kcp")
		opts := n.Config["kcp-opts"].(map[string]any)
		if opts["header"] != "wechat-video" || opts["seed"] != "seed" || opts["mtu"] != 1300 {
			t.Fatalf("native KCP mapping wrong: %#v", opts)
		}
	})
	t.Run("native-ech", func(t *testing.T) {
		obj := nativeFixture("vless")
		config := base64.StdEncoding.EncodeToString([]byte("native-ech"))
		obj["ech-opts"] = map[string]any{"enable": true, "config": config}
		n := parseOne(t, nativeDoc(t, obj))
		opts := n.Config["ech-opts"].(map[string]any)
		if opts["enable"] != true || opts["config"] != config {
			t.Fatalf("native ECH mapping wrong: %#v", opts)
		}
	})
	t.Run("vmess-xhttp-native-rejected", func(t *testing.T) {
		obj := nativeFixture("vmess")
		obj["network"] = "xhttp"
		requireInvalid(t, nativeDoc(t, obj))
	})
	t.Run("kcp-header-invalid-rejected", func(t *testing.T) {
		obj := nativeFixture("vmess")
		obj["network"] = "kcp"
		obj["kcp-opts"] = map[string]any{"header": "bogus"}
		requireInvalid(t, nativeDoc(t, obj))
	})
	t.Run("kcp-options-without-kcp-rejected", func(t *testing.T) {
		obj := nativeFixture("vmess")
		obj["network"] = "ws"
		obj["kcp-opts"] = map[string]any{"header": "srtp"}
		requireInvalid(t, nativeDoc(t, obj))
	})
}
