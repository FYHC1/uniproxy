package importer

import (
	"encoding/base64"
	"net/url"
	"strings"
	"testing"
)

func TestCredentialsAreDecodedExactlyOnce(t *testing.T) {
	password := "literal%2F+%3A@/:密码"
	credentials := url.User(password).String()
	for _, protocol := range []string{"trojan", "hysteria2"} {
		node := parseOne(t, protocol+"://"+credentials+"@example.com:443")
		if node.Config["password"] != password {
			t.Fatal("single-credential password was decoded more than once")
		}
	}
	for _, protocol := range []string{"ssh", "socks5", "http", "https"} {
		node := parseOne(t, protocol+"://"+url.UserPassword("literal%40user", password).String()+"@example.com:443")
		if node.Config["password"] != password || node.Config["username"] != "literal%40user" {
			t.Fatal("username or password decoded more than once")
		}
	}
	for _, encode := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		auth := encode.EncodeToString([]byte("aes-256-gcm:" + password))
		node := parseOne(t, "ss://"+url.QueryEscape(auth)+"@example.com:8388")
		if node.Config["password"] != password {
			t.Fatal("base64 userinfo credential changed")
		}
	}
}
func TestLabelsMayContainSpacesOrAtSigns(t *testing.T) {
	node := parseOne(t, vmessLink(vmessBase())+"#Server @ home")
	if node.Name != "Server @ home" {
		t.Fatal("VMess label made parser choose wrong URI variant")
	}
	node = parseOne(t, "trojan://password@example.com:443#New York + 東京")
	if node.Name != "New York + 東京" {
		t.Fatal("fragment literal plus or unicode changed")
	}
}
func TestSSLegacyCannotSmuggleIgnoredURIOptions(t *testing.T) {
	for _, suffix := range []string{"?plugin=evil", "#secret", "/unmapped"} {
		encoded := base64.StdEncoding.EncodeToString([]byte("aes-256-gcm:password@example.com:443" + suffix))
		requireInvalid(t, "ss://"+encoded)
	}
	requireInvalid(t, "ss://%ZZ@example.com:443")
}
func TestYAMLFlowStyleDocuments(t *testing.T) {
	for _, content := range []string{
		"{proxies: [{type: http, server: example.com, port: 80}]}",
		"[{type: http, server: example.com, port: 80}]",
		"{type: http, server: example.com, port: 80}",
	} {
		parseOne(t, content)
	}
}

func TestNoSecretInDefaultName(t *testing.T) {
	password := "secret-0123456789"
	node := parseOne(t, "trojan://"+password+"@[2001:db8::1]:443")
	if strings.Contains(node.Name, password) || node.Name != "trojan://[2001:db8::1]:443" {
		t.Fatal("default name reveals credentials or loses IPv6 brackets")
	}
}
