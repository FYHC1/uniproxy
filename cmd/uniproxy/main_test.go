package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uniproxy/uniproxy/internal/store"
)

func cleanEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{"UNIPROXY_HOST", "UNIPROXY_PORT", "UNIPROXY_DATA_DIR", "UNIPROXY_CORE_PATH", "UNIPROXY_ADMIN_TOKEN", "UNIPROXY_ALLOWED_ORIGINS", "UNIPROXY_ALLOW_PRIVATE_SUBSCRIPTIONS"} {
		t.Setenv(name, "")
	}
}

func TestParseOptionsKeepsCLIBehaviorAndRedactsInvalidInput(t *testing.T) {
	cleanEnvironment(t)
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"unknown", []string{"-secret-token-CANARY"}, "invalid command-line"},
		{"invalid-port", []string{"-port", "secret-token-CANARY"}, "invalid command-line"},
		{"positional", []string{"secret-token-CANARY"}, "unexpected positional"},
		{"public", []string{"-host", "0.0.0.0"}, "requires UNIPROXY_ADMIN_TOKEN"},
		{"host", []string{"-host", "secret-token-CANARY"}, "host must be an IP"},
		{"port-range", []string{"-port", "0"}, "1024..65535"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			_, err := parseOptions(tc.args, &output)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("unexpected result: %v", err)
			}
			if strings.Contains(err.Error()+output.String(), "CANARY") {
				t.Fatal("raw invalid arguments leaked to error output")
			}
		})
	}
	var help bytes.Buffer
	if _, err := parseOptions([]string{"-h"}, &help); !errors.Is(err, flag.ErrHelp) || !strings.Contains(help.String(), "-data") {
		t.Fatalf("help: %v %s", err, help.String())
	}
	var output bytes.Buffer
	if err := run(context.Background(), []string{"-version"}, &output, log.New(io.Discard, "", 0)); err != nil || !strings.HasPrefix(output.String(), "UniProxy ") {
		t.Fatalf("version: %v %s", err, output.String())
	}
}

func TestCLIUsageKeepsExitCodeTwo(t *testing.T) {
	if failureExitCode(errCLIUsage) != 2 || failureExitCode(errors.New("startup failure")) != 1 {
		t.Fatal("CLI syntax and startup failure exit codes changed")
	}
}

func TestAdminBindFailureDoesNotCreateOrOpenData(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	dir := filepath.Join(t.TempDir(), "must-not-create")
	opts := options{host: "127.0.0.1", port: listener.Addr().(*net.TCPAddr).Port, data: dir}
	err = serve(context.Background(), opts, log.New(io.Discard, "", 0))
	if err == nil || !strings.Contains(err.Error(), "admin listen") {
		t.Fatalf("bind error: %v", err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("duplicate/busy-port launch touched data directory")
	}
}

func TestStoreFailureReportsSelectedPathWithoutResettingKey(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "master.key")
	if err := os.WriteFile(keyPath, []byte("synthetic-invalid-key-CANARY"), 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	err := serve(context.Background(), options{host: "127.0.0.1", port: 0, data: dir}, log.New(&output, "", 0))
	if err == nil || !strings.Contains(err.Error(), "master.key") || !strings.Contains(err.Error(), "open persistent data") || !strings.Contains(output.String(), dir) {
		t.Fatalf("store error missing actionable context: %v, %s", err, output.String())
	}
	if strings.Contains(err.Error()+output.String(), "CANARY") {
		t.Fatal("key content leaked")
	}
	key, err := os.ReadFile(keyPath)
	if err != nil || string(key) != "synthetic-invalid-key-CANARY" {
		t.Fatal("invalid original key changed")
	}
	if _, err := os.Stat(filepath.Join(dir, "uniproxy.db")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("invalid-key startup created a database")
	}
}

func TestServiceCancelThenRestartKeepsSameSyntheticState(t *testing.T) {
	cleanEnvironment(t)
	dir := t.TempDir()
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	state := s.Snapshot()
	state.Revision = 47
	if err := s.Save(state); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	for round := 0; round < 2; round++ {
		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		port := listener.Addr().(*net.TCPAddr).Port
		listener.Close()
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		opts := options{host: "127.0.0.1", port: port, data: dir, binary: filepath.Join(dir, "absent-mihomo")}
		go func() { done <- serve(ctx, opts, log.New(io.Discard, "", 0)) }()
		client := &http.Client{Timeout: 200 * time.Millisecond, Transport: &http.Transport{Proxy: nil}}
		deadline := time.Now().Add(5 * time.Second)
		ready := false
		for time.Now().Before(deadline) {
			res, reqErr := client.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/api/health")
			if reqErr == nil {
				res.Body.Close()
				if res.StatusCode == http.StatusOK {
					ready = true
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
		}
		client.CloseIdleConnections()
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("shutdown: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("shutdown did not finish")
		}
		if !ready {
			t.Fatal("management did not start without core")
		}
		s, err := store.Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		got := s.Snapshot()
		s.Close()
		if got.Revision != 47 {
			t.Fatal("restarting silently replaced synthetic state")
		}
	}
}

type blockingStartup struct {
	closed chan struct{}
	calls  atomic.Int32
}

func (a *blockingStartup) Close() {
	if a.calls.Add(1) == 1 {
		close(a.closed)
	}
}
func TestShutdownCancelsDuringBlockedStartupAndClosesOnce(t *testing.T) {
	for _, alreadyCanceled := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		if alreadyCanceled {
			cancel()
		}
		app := &blockingStartup{closed: make(chan struct{})}
		cleanup := watchApplication(ctx, app)
		started, done := make(chan struct{}), make(chan struct{})
		go func() { close(started); <-app.closed; close(done) }()
		<-started
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("cancel did not unblock startup")
		}
		cleanup()
		cleanup()
		if app.calls.Load() != 1 {
			t.Fatal("concurrent close ran more than once")
		}
	}
}
