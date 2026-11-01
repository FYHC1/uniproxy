package core

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uniproxy/uniproxy/internal/model"
)

func TestVerifyProxiesWaitsForNativeOutbound(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/proxies" {
			http.NotFound(w, r)
			return
		}
		if calls.Add(1) < 3 {
			_, _ = io.WriteString(w, `{"proxies":{}}`)
			return
		}
		_, _ = io.WriteString(w, `{"proxies":{"n-a":{}}}`)
	}))
	defer server.Close()

	m := New(Options{})
	m.buildOpts = BuildOptions{ControllerAddress: server.Listener.Addr().String(), ControllerSecret: "test-secret"}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := m.verifyProxies(ctx, &compiledConfig{nodes: map[string]struct{}{"a": {}}}); err != nil {
		t.Fatalf("verifyProxies did not wait for the native outbound: %v", err)
	}
	if calls.Load() < 3 {
		t.Fatalf("verifyProxies returned after %d calls, want at least 3", calls.Load())
	}
}

func TestVerifyListenerRetriesUntilBind(t *testing.T) {
	reserved, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := reserved.Addr().(*net.TCPAddr).Port
	_ = reserved.Close()

	listenerCh := make(chan net.Listener, 1)
	errCh := make(chan error, 1)
	go func() {
		// Longer than the old single 750 ms check, so this regression test
		// fails against the previous behavior and passes only with retry.
		time.Sleep(1200 * time.Millisecond)
		ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err != nil {
			errCh <- err
			return
		}
		listenerCh <- ln
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				var greeting [3]byte
				if _, err := io.ReadFull(conn, greeting[:]); err != nil {
					return
				}
				_, _ = conn.Write([]byte{5, 0})
			}(conn)
		}
	}()

	listener := model.Listener{ID: "retry-probe", Protocol: "socks", Host: "127.0.0.1", Port: port}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := verifyListener(ctx, listener); err != nil {
		select {
		case serverErr := <-errCh:
			t.Fatalf("delayed listener failed to bind: %v", serverErr)
		default:
		}
		t.Fatalf("verifyListener did not retry until bind: %v", err)
	}
	select {
	case ln := <-listenerCh:
		_ = ln.Close()
	case err := <-errCh:
		t.Fatalf("delayed listener failed to bind: %v", err)
	default:
		t.Fatal("verifyListener succeeded without recording the delayed listener")
	}
}

func TestVerifyListenerStopsAtDeadline(t *testing.T) {
	reserved, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := reserved.Addr().(*net.TCPAddr).Port
	_ = reserved.Close()
	listener := model.Listener{ID: "closed-probe", Protocol: "socks", Host: "127.0.0.1", Port: port}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	err = verifyListener(ctx, listener)
	if err == nil {
		t.Fatal("verifyListener unexpectedly succeeded for a closed listener")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("verifyListener should return the attempt error, not a bare context deadline")
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("verifyListener ignored the parent deadline: %s", elapsed)
	}
}
