package integration

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/uniproxy/uniproxy/internal/core"
	"github.com/uniproxy/uniproxy/internal/importer"
	"github.com/uniproxy/uniproxy/internal/model"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestImportedProtocolsPassPinnedCoreValidation(t *testing.T) {
	binary := os.Getenv("UNIPROXY_TEST_CORE_PATH")
	if binary == "" {
		t.Skip("set UNIPROXY_TEST_CORE_PATH to run real Mihomo validation")
	}
	binary, e := filepath.Abs(binary)
	if e != nil {
		t.Fatal(e)
	}
	id := "12345678-1234-4234-8234-123456789abc"
	links := []string{
		"vless://" + id + "@127.0.0.1:443?security=tls&sni=example.com",
		"vmess://" + id + "@127.0.0.1:443?security=tls",
		"trojan://password@127.0.0.1:443?sni=example.com",
		"ss://" + base64.RawURLEncoding.EncodeToString([]byte("aes-256-gcm:password")) + "@127.0.0.1:8388",
		"hysteria://127.0.0.1:443?auth=password&upmbps=10&downmbps=20&peer=example.com",
		"hy2://password@127.0.0.1:443?sni=example.com",
		"tuic://" + id + ":password@127.0.0.1:443?sni=example.com&congestion_control=bbr",
		"ssh://user:password@127.0.0.1:22",
		"socks5://user:password@127.0.0.1:1080?udp=true",
		"http://user:password@127.0.0.1:8080",
		"vless://" + id + "@127.0.0.1:443?security=reality&pbk=" + base64.RawURLEncoding.EncodeToString([]byte("01234567890123456789012345678901")) + "&sid=abcd&sni=example.com&fp=chrome&flow=xtls-rprx-vision",
		"trojan://password@127.0.0.1:443?type=grpc&serviceName=grpc&sni=example.com",
		"vless://" + id + "@127.0.0.1:443?security=tls&type=ws&path=%2Fproxy&host=example.com",
	}
	manager := core.New(core.Options{BinaryPath: binary, DataDir: t.TempDir()})
	for i, link := range links {
		t.Run(fmt.Sprintf("protocol-%02d", i), func(t *testing.T) {
			parsed := importer.Parse(link)
			if len(parsed.Nodes) != 1 || len(parsed.Errors) != 0 {
				t.Fatalf("parser: %+v", parsed.Errors)
			}
			s := model.DefaultState()
			s.Nodes = []model.Node{{ID: fmt.Sprintf("n%d", i), Enabled: true, ParsedNode: parsed.Nodes[0]}}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if e := manager.Validate(ctx, s); e != nil {
				t.Fatal(e)
			}
		})
	}
}
