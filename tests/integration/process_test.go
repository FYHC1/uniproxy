package integration

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestBuiltServiceWithEmbeddedUI(t *testing.T) {
	appPath := os.Getenv("UNIPROXY_TEST_APP_PATH")
	corePath := os.Getenv("UNIPROXY_TEST_CORE_PATH")
	if appPath == "" || corePath == "" {
		t.Skip("set both UNIPROXY_TEST_APP_PATH and UNIPROXY_TEST_CORE_PATH")
	}
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, appPath, "-host", "127.0.0.1", "-port", strconv.Itoa(port), "-data", t.TempDir(), "-core", corePath)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if e = cmd.Start(); e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	defer func() { cancel(); <-done }()
	client := &http.Client{Timeout: 3 * time.Second}
	base := "http://127.0.0.1:" + strconv.Itoa(port)
	deadline := time.Now().Add(15 * time.Second)
	for {
		res, e := client.Get(base + "/api/health")
		if e == nil {
			res.Body.Close()
			if res.StatusCode == 200 {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("service not ready")
		}
		time.Sleep(75 * time.Millisecond)
	}
	// Stop the owned core via API before process cleanup, including assertion failures.
	defer func() {
		req, _ := http.NewRequest("POST", base+"/api/runtime/stop", nil)
		if res, e := client.Do(req); e == nil {
			res.Body.Close()
		}
	}()
	res, e := client.Get(base + "/api/bootstrap")
	if e != nil {
		t.Fatal(e)
	}
	var boot struct {
		Nodes   []any
		Runtime struct {
			Running, Dirty bool
			Version        string
		}
	}
	e = json.NewDecoder(res.Body).Decode(&boot)
	res.Body.Close()
	if e != nil {
		t.Fatal(e)
	}
	if len(boot.Nodes) != 0 || !boot.Runtime.Running || boot.Runtime.Dirty || boot.Runtime.Version != "1.19.32" {
		t.Fatalf("bad startup state %+v", boot)
	}
	res, e = client.Get(base + "/")
	if e != nil {
		t.Fatal(e)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if !strings.Contains(string(body), "/assets/") || res.Header.Get("Content-Security-Policy") == "" {
		t.Fatal("embedded production UI/security headers missing")
	}
}
