package integration

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/uniproxy/uniproxy/internal/core"
	"github.com/uniproxy/uniproxy/internal/importer"
)

// This observes the real pinned core's ClientHello against our own loopback
// server. The server deliberately ends the handshake before authentication or
// proxy forwarding; no public endpoint, private credential or user state exists.
func TestRealStaticSubscriptionTLSIdentity(t *testing.T) {
	binary := os.Getenv("UNIPROXY_TEST_CORE_PATH")
	if binary == "" {
		t.Skip("set UNIPROXY_TEST_CORE_PATH for controlled loopback TLS identity check")
	}
	const id = "11111111-1111-4111-8111-111111111111"
	for _, protocol := range []string{"vmess", "vless", "trojan"} {
		t.Run(protocol, func(t *testing.T) {
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			names := make(chan string, 1)
			done := make(chan struct{})
			go func() {
				defer close(done)
				conn, err := ln.Accept()
				if err != nil {
					return
				}
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(8 * time.Second))
				server := tls.Server(conn, &tls.Config{MinVersion: tls.VersionTLS12, GetConfigForClient: func(info *tls.ClientHelloInfo) (*tls.Config, error) {
					names <- info.ServerName
					return nil, errors.New("synthetic handshake observation complete")
				}})
				_ = server.HandshakeContext(ctx)
			}()
			t.Cleanup(func() {
				cancel()
				_ = ln.Close()
				select {
				case <-done:
				case <-time.After(9 * time.Second):
					t.Error("loopback TLS observer did not stop")
				}
			})
			port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
			base := "fixture = " + protocol + ",127.0.0.1," + port + ","
			switch protocol {
			case "vmess":
				base += "username=" + id + ",vmess-aead=true,tls=true,"
			case "vless":
				base = "fixture = vless,127.0.0.1," + port + ",\"" + id + "\",over-tls=true,"
			case "trojan":
				base += "password=synthetic-only,"
			}
			body := base + "ws=true,ws-path=/socket,ws-headers=Host:cdn.example.invalid,sni=auth.example.invalid,skip-cert-verify=false"
			result := importer.Parse(body)
			if result.TotalErrors != 0 || len(result.Nodes) != 1 {
				t.Fatalf("synthetic TLS source rejected: %v", result.Errors)
			}
			node := result.Nodes[0]
			serverKey := "servername"
			if protocol == "trojan" {
				serverKey = "sni"
			}
			if node.Config[serverKey] != "auth.example.invalid" || node.Config["skip-cert-verify"] != false {
				t.Fatal("explicit TLS identity or verification changed")
			}
			state := subscriptionValidationState(t, result)
			m := core.New(core.Options{BinaryPath: binary, DataDir: t.TempDir()})
			t.Cleanup(func() {
				stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := m.Stop(stopCtx); err != nil {
					t.Error("failed to stop owned test core")
				}
			})
			if err := m.Apply(ctx, state); err != nil {
				t.Fatal("cannot apply isolated synthetic TLS state")
			}
			check, checkErr := m.Check(ctx, state.Nodes[0].ID, "http://127.0.0.1:1/observation-only", 5*time.Second)
			if checkErr == nil || check.OK {
				t.Fatal("deliberately stopped TLS handshake was reported as successful connectivity")
			}
			select {
			case got := <-names:
				if got != "auth.example.invalid" {
					t.Fatal(fmt.Sprintf("core ClientHello used %q instead of the explicit TLS identity", got))
				}
			case <-ctx.Done():
				t.Fatal("no ClientHello observed on owned loopback listener")
			}
		})
	}
}
