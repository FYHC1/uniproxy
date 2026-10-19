package core

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uniproxy/uniproxy/internal/model"
	"gopkg.in/yaml.v3"
)

type mockInbound struct {
	Name   string             `yaml:"name"`
	Type   string             `yaml:"type"`
	Listen string             `yaml:"listen"`
	Port   int                `yaml:"port"`
	UDP    bool               `yaml:"udp"`
	Users  []model.Credential `yaml:"users"`
	Proxy  string             `yaml:"proxy"`
	Rule   string             `yaml:"rule"`
}
type mockDocument struct {
	Controller string           `yaml:"external-controller"`
	Secret     string           `yaml:"secret"`
	Proxies    []map[string]any `yaml:"proxies"`
	Listeners  []mockInbound    `yaml:"listeners"`
}
type mockBinding struct {
	config mockInbound
	tcp    net.Listener
	udp    net.PacketConn
}
type mockCore struct {
	mu       sync.Mutex
	config   mockDocument
	bindings map[string]mockBinding
	poisoned bool
	reloads  int
}

// The test binary impersonates only the documented external process/controller
// boundary. This exercises real exec, files, auth, reload/rollback and Stop on
// Windows and Linux without depending on a downloaded core for unit tests.
func TestCoreHelperProcess(t *testing.T) {
	if os.Getenv("UNIPROXY_CORE_HELPER") != "1" {
		return
	}
	start := 0
	for i, arg := range os.Args {
		if arg == "--" {
			start = i + 1
			break
		}
	}
	args := os.Args[start:]
	if len(args) == 1 && args[0] == "-v" {
		version := os.Getenv("UNIPROXY_MOCK_VERSION")
		if version == "" {
			version = PinnedVersion
		}
		fmt.Println("Mihomo Meta v" + version + " windows amd64 with test helper")
		os.Exit(0)
	}
	path := ""
	validate := false
	for i, arg := range args {
		if arg == "-t" {
			validate = true
		}
		if arg == "-f" && i+1 < len(args) {
			path = args[i+1]
		}
	}
	var data []byte
	var err error
	if path == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	var doc mockDocument
	if err = yaml.Unmarshal(data, &doc); err != nil {
		os.Exit(3)
	}
	if validate {
		for _, p := range doc.Proxies {
			if p["server"] == "invalid-native.invalid" {
				fmt.Fprintln(os.Stderr, "password=super-sensitive-native-secret invalid proxy")
				os.Exit(1)
			}
		}
		os.Exit(0)
	}
	core := &mockCore{bindings: map[string]mockBinding{}}
	if err = core.apply(doc); err != nil {
		fmt.Fprintln(os.Stderr, "listen err:", err)
		os.Exit(4)
	}
	server := &http.Server{Addr: doc.Controller, Handler: http.HandlerFunc(core.serve), ReadHeaderTimeout: time.Second}
	if err = server.ListenAndServe(); err != nil {
		os.Exit(5)
	}
	os.Exit(0)
}
func (c *mockCore) apply(doc mockDocument) error {
	// Caller holds mu once serving. Unlike the real core, the fixture returns bind
	// errors to make unexpected test port interference easy to diagnose.
	for name, b := range c.bindings {
		found := false
		for _, n := range doc.Listeners {
			if n.Name == name && reflect.DeepEqual(n, b.config) {
				found = true
				break
			}
		}
		if !found {
			b.tcp.Close()
			if b.udp != nil {
				b.udp.Close()
			}
			delete(c.bindings, name)
		}
	}
	for _, l := range doc.Listeners {
		if _, exists := c.bindings[l.Name]; exists {
			continue
		}
		socket, err := net.Listen("tcp", net.JoinHostPort(l.Listen, strconv.Itoa(l.Port)))
		if err != nil {
			return err
		}
		b := mockBinding{config: l, tcp: socket}
		if l.UDP {
			b.udp, err = net.ListenPacket("udp", socket.Addr().String())
			if err != nil {
				socket.Close()
				return err
			}
		}
		c.bindings[l.Name] = b
		go func(l mockInbound, s net.Listener) {
			for {
				conn, err := s.Accept()
				if err != nil {
					return
				}
				go mockProxyConn(conn, l)
			}
		}(l, socket)
	}
	c.config = doc
	return nil
}
func (c *mockCore) serve(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer "+c.config.Secret {
		http.Error(w, "unauthorized", 401)
		return
	}
	switch {
	case r.URL.Path == "/version":
		json.NewEncoder(w).Encode(map[string]any{"version": "v" + PinnedVersion})
	case r.URL.Path == "/proxies":
		proxies := map[string]any{"REJECT": map[string]any{}}
		for _, p := range c.config.Proxies {
			proxies[p["name"].(string)] = map[string]any{"type": "Socks5"}
		}
		json.NewEncoder(w).Encode(map[string]any{"proxies": proxies})
	case strings.HasSuffix(r.URL.Path, "/delay"):
		if r.URL.Query().Get("expected") != "200-399" {
			http.Error(w, "expected status missing", 400)
			return
		}
		timeout, _ := strconv.Atoi(r.URL.Query().Get("timeout"))
		if timeout < 1 || timeout > 32767 {
			http.Error(w, "timeout invalid", 400)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"delay": 17})
	case r.URL.Path == "/test-counts":
		json.NewEncoder(w).Encode(map[string]any{"reloads": c.reloads})
	case r.URL.Path == "/configs" && r.Method == http.MethodPut:
		c.reloads++
		if r.URL.Query().Get("force") != "true" {
			http.Error(w, "missing force", 400)
			return
		}
		var request struct {
			Payload string `json:"payload"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		var doc mockDocument
		if err := yaml.Unmarshal([]byte(request.Payload), &doc); err != nil {
			http.Error(w, "bad YAML", 400)
			return
		}
		fail, verifyFail := false, false
		for _, p := range doc.Proxies {
			switch p["server"] {
			case "reload-fail.invalid":
				fail = true
			case "rollback-fail.invalid":
				c.poisoned = true
			case "verify-fail.invalid":
				verifyFail = true
			}
		}
		if fail || c.poisoned {
			http.Error(w, "super-sensitive-native-secret", 500)
			return
		}
		if verifyFail {
			for name, b := range c.bindings {
				b.tcp.Close()
				if b.udp != nil {
					b.udp.Close()
				}
				delete(c.bindings, name)
			}
			c.config = doc
			w.WriteHeader(204)
			return
		}
		if err := c.apply(doc); err != nil {
			http.Error(w, "listen err", 500)
			return
		}
		w.WriteHeader(204)
	default:
		http.NotFound(w, r)
	}
}
func mockProxyConn(conn net.Conn, l mockInbound) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	br := bufio.NewReader(conn)
	first, err := br.Peek(1)
	if err != nil {
		return
	}
	if first[0] == 5 {
		var header [2]byte
		if _, err = io.ReadFull(br, header[:]); err != nil {
			return
		}
		methods := make([]byte, int(header[1]))
		if _, err = io.ReadFull(br, methods); err != nil {
			return
		}
		method := byte(0)
		if len(l.Users) > 0 {
			method = 2
		}
		supported := false
		for _, m := range methods {
			if m == method {
				supported = true
			}
		}
		if !supported {
			conn.Write([]byte{5, 255})
			return
		}
		conn.Write([]byte{5, method})
		if method == 2 {
			if _, err = io.ReadFull(br, header[:]); err != nil {
				return
			}
			name := make([]byte, int(header[1]))
			if _, err = io.ReadFull(br, name); err != nil {
				return
			}
			n, err := br.ReadByte()
			if err != nil {
				return
			}
			password := make([]byte, int(n))
			if _, err = io.ReadFull(br, password); err != nil {
				return
			}
			if !mockAuth(l, string(name), string(password)) {
				conn.Write([]byte{1, 1})
				return
			}
			conn.Write([]byte{1, 0})
		}
		io.Copy(io.Discard, br)
		return
	}
	request, err := http.ReadRequest(br)
	if err != nil {
		return
	}
	defer request.Body.Close()
	credential := strings.TrimPrefix(request.Header.Get("Proxy-Authorization"), "Basic ")
	decoded, _ := base64.StdEncoding.DecodeString(credential)
	username, password, _ := strings.Cut(string(decoded), ":")
	if !mockAuth(l, username, password) {
		writeMockStatus(conn, request, http.StatusProxyAuthRequired)
		return
	}
	if request.URL.Scheme == "" || request.URL.Host == "" {
		writeMockStatus(conn, request, http.StatusBadRequest)
		return
	}
	request.RequestURI = ""
	request.Header.Del("Proxy-Authorization")
	request.Header.Set("X-Mock-Proxy-User", username)
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	response, err := transport.RoundTrip(request)
	if err != nil {
		writeMockStatus(conn, request, 502)
		return
	}
	defer response.Body.Close()
	response.Close = true
	response.Write(conn)
}
func mockAuth(l mockInbound, username, password string) bool {
	if len(l.Users) == 0 {
		return true
	}
	for _, u := range l.Users {
		if u.Username == username && u.Password == password {
			return true
		}
	}
	return false
}
func writeMockStatus(w io.Writer, request *http.Request, status int) {
	response := &http.Response{StatusCode: status, ProtoMajor: 1, ProtoMinor: 1, Header: http.Header{}, Body: http.NoBody, ContentLength: 0, Close: true, Request: request}
	response.Write(w)
}

// freePort returns a loopback port where both TCP and UDP can bind. It scans a
// fixed low range instead of asking the OS for an ephemeral port: on Windows the
// dynamic range overlaps excluded ranges, so ":0" often yields a port whose UDP
// twin is unavailable and the helper failed intermittently.
func freePort(t *testing.T) int {
	t.Helper()
	for port := 21000; port < 48000; port++ {
		addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
		tcp, err := net.Listen("tcp4", addr)
		if err != nil {
			continue
		}
		udp, err := net.ListenPacket("udp4", addr)
		tcp.Close()
		if err != nil {
			continue
		}
		udp.Close()
		return port
	}
	t.Fatal("cannot allocate an available TCP/UDP fixture port")
	return 0
}
func mockManager(t *testing.T) (*Manager, *atomic.Int32, *atomic.Int32) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	m := New(Options{BinaryPath: exe, DataDir: t.TempDir()})
	starts, versions := &atomic.Int32{}, &atomic.Int32{}
	makeCmd := func(ctx context.Context, args []string) *exec.Cmd {
		all := append([]string{"-test.run=^TestCoreHelperProcess$", "--"}, args...)
		cmd := exec.CommandContext(ctx, exe, all...)
		cmd.Env = append(os.Environ(), "UNIPROXY_CORE_HELPER=1")
		return cmd
	}
	m.command = func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		if len(args) == 1 && args[0] == "-v" {
			versions.Add(1)
		}
		return makeCmd(ctx, args)
	}
	m.startCommand = func(_ string, args ...string) *exec.Cmd { starts.Add(1); return makeCmd(context.Background(), args) }
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := m.Stop(ctx); err != nil {
			t.Errorf("stop: %v", err)
		}
	})
	return m, starts, versions
}
func lifecycleState(t *testing.T) model.State {
	s := testState()
	s.Listeners[0].Port = freePort(t)
	return s
}

func TestLifecycleValidateApplyReloadStop(t *testing.T) {
	m, starts, versions := mockManager(t)
	ctx := context.Background()
	state := lifecycleState(t)
	if err := m.Validate(ctx, state); err != nil {
		t.Fatal(err)
	}
	if m.Status().Running || starts.Load() != 0 {
		t.Fatal("Validate started a live core")
	}
	if err := m.Apply(ctx, state); err != nil {
		t.Fatal(err)
	}
	status := m.Status()
	if !status.Available || !status.Running || status.Dirty || status.AppliedRevision != 1 || !status.Listeners[0].Listening {
		t.Fatalf("bad status %+v", status)
	}
	firstPID := m.process.cmd.Process.Pid
	status.Listeners[0].Listening = false
	if !m.Status().Listeners[0].Listening {
		t.Fatal("Status returned mutable internal slice")
	}
	unauth, err := http.Get("http://" + m.buildOpts.ControllerAddress + "/version")
	if err != nil {
		t.Fatal(err)
	}
	unauth.Body.Close()
	if unauth.StatusCode != 401 {
		t.Fatal("mock controller not authenticated")
	}
	state.Nodes[0].Name = "renamed"
	state.Revision = 2
	if err = m.Apply(ctx, state); err != nil {
		t.Fatal(err)
	}
	state.Pools[0].Strategy = "consistent-hashing"
	state.Revision = 3
	if err = m.Apply(ctx, state); err != nil {
		t.Fatal(err)
	}
	if m.process.cmd.Process.Pid != firstPID || starts.Load() != 1 {
		t.Fatal("normal update restarted the process")
	}
	if versions.Load() != 1 {
		t.Fatalf("version not cached: %d", versions.Load())
	}
	state.Listeners = nil
	state.Revision = 4
	if err = m.Apply(ctx, state); err != nil {
		t.Fatal(err)
	}
	if len(m.Status().Listeners) != 0 {
		t.Fatal("deleted listener still in runtime")
	}
	if err = m.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if m.Status().Running || !m.Status().Dirty {
		t.Fatal("incorrect stopped status")
	}
	if err = m.Stop(ctx); err != nil {
		t.Fatal("Stop should be idempotent")
	}
}
func TestLifecycleFailedReloadRollsBack(t *testing.T) {
	for _, failure := range []string{"reload-fail.invalid", "verify-fail.invalid", "rollback-fail.invalid"} {
		t.Run(failure, func(t *testing.T) {
			m, starts, _ := mockManager(t)
			state := lifecycleState(t)
			if err := m.Apply(context.Background(), state); err != nil {
				t.Fatal(err)
			}
			state.Revision = 2
			state.Nodes[0].Config["server"] = failure
			err := m.Apply(context.Background(), state)
			if err == nil {
				t.Fatal("failed transaction committed")
			}
			if strings.Contains(err.Error(), "super-sensitive-native-secret") {
				t.Fatal("secret appeared in lifecycle error")
			}
			status := m.Status()
			if !status.Running || status.AppliedRevision != 1 || !status.Dirty || status.LastError == "" {
				t.Fatalf("rollback status %+v", status)
			}
			expectedStarts := int32(1)
			if failure == "rollback-fail.invalid" {
				expectedStarts = 2
				if !strings.Contains(err.Error(), "connections reset") {
					t.Fatal("restart recovery must disclose connection reset")
				}
			}
			if starts.Load() != expectedStarts {
				t.Fatalf("unexpected process restarts %d", starts.Load())
			}
			if strings.Contains(string(m.live.data), failure) {
				t.Fatal("failed config committed as live")
			}
			entries, err := os.ReadDir(m.options.DataDir)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if strings.HasSuffix(entry.Name(), ".yaml") {
					t.Fatal("plaintext core configuration left on disk")
				}
			}
		})
	}
}
func TestLifecycleValidationAndUnavailable(t *testing.T) {
	m, starts, _ := mockManager(t)
	state := lifecycleState(t)
	if err := m.Apply(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	state.Nodes[0].Config["server"] = "invalid-native.invalid"
	state.Revision = 2
	if err := m.Validate(context.Background(), state); err == nil || strings.Contains(err.Error(), "super-sensitive") {
		t.Fatalf("unsafe validation failure: %v", err)
	}
	if err := m.Apply(context.Background(), state); err == nil {
		t.Fatal("invalid core configuration applied")
	}
	if starts.Load() != 1 || !m.Status().Running || m.Status().AppliedRevision != 1 {
		t.Fatal("validation touched running core")
	}
	absent := New(Options{BinaryPath: filepath.Join(t.TempDir(), "missing-core"), DataDir: t.TempDir()})
	if err := absent.Validate(context.Background(), model.DefaultState()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("missing ErrUnavailable: %v", err)
	}
	if status := absent.Inspect(context.Background()); status.Available || status.Running {
		t.Fatal(status)
	}
}
func TestLifecycleUnsupportedVersion(t *testing.T) {
	m, starts, _ := mockManager(t)
	original := m.command
	m.command = func(ctx context.Context, path string, args ...string) *exec.Cmd {
		cmd := original(ctx, path, args...)
		cmd.Env = append(cmd.Env, "UNIPROXY_MOCK_VERSION=1.19.31")
		return cmd
	}
	err := m.Apply(context.Background(), lifecycleState(t))
	if !errors.Is(err, ErrUnsupportedVersion) || starts.Load() != 0 {
		t.Fatalf("unpinned core allowed: %v", err)
	}
}
func TestLifecycleOccupiedListenerDoesNotTouchLiveCore(t *testing.T) {
	m, _, _ := mockManager(t)
	s := lifecycleState(t)
	if err := m.Apply(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	occupied, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	s.Listeners[0].Port = occupied.Addr().(*net.TCPAddr).Port
	s.Revision = 2
	if err = m.Apply(context.Background(), s); err == nil {
		t.Fatal("occupied listener accepted")
	}
	if !m.Status().Running || m.Status().AppliedRevision != 1 {
		t.Fatal("bind collision touched live config")
	}
}
func TestLifecycleUnexpectedExit(t *testing.T) {
	m, _, _ := mockManager(t)
	if err := m.Apply(context.Background(), lifecycleState(t)); err != nil {
		t.Fatal(err)
	}
	p := m.process
	if err := p.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	<-p.done
	deadline := time.After(2 * time.Second)
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		if !m.Status().Running {
			break
		}
		select {
		case <-deadline:
			t.Fatal("exit not observed")
		case <-ticker.C:
		}
	}
	if !m.Status().Dirty || m.Status().LastError == "" {
		t.Fatal("unexpected exit not exposed")
	}
}
func TestCheckAndBoundedSpeedUseAuthenticatedCore(t *testing.T) {
	m, _, _ := mockManager(t)
	if err := m.Apply(context.Background(), lifecycleState(t)); err != nil {
		t.Fatal(err)
	}
	checked, err := m.Check(context.Background(), "b", "http://example.invalid/path?private=redacted", time.Minute)
	if err != nil || !checked.OK || checked.LatencyMS == nil || *checked.LatencyMS != 17 {
		t.Fatalf("delay API failed: %+v %v", checked, err)
	}
	if _, err = m.Check(context.Background(), "deleted", "http://example.invalid", time.Second); err == nil {
		t.Fatal("stale node tested")
	}
	var proxyUsed atomic.Bool
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Mock-Proxy-User") == "n-b" && r.Header.Get("Proxy-Authorization") == "" {
			proxyUsed.Store(true)
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		io.CopyN(w, strings.NewReader(strings.Repeat("download", 2048)), 16384)
	}))
	defer endpoint.Close()
	result, err := m.Speed(context.Background(), "b", endpoint.URL, 3*time.Second, 1024)
	if err != nil || !result.OK || result.Bytes != 1024 || result.SpeedMbps == nil || *result.SpeedMbps <= 0 || !proxyUsed.Load() {
		t.Fatalf("speed was not bounded/proxied: %+v %v", result, err)
	}
	if _, err = m.Speed(context.Background(), "b", endpoint.URL, 3*time.Second, MaxSpeedBytes+1); err == nil {
		t.Fatal("oversized test accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if result, err = m.Speed(ctx, "b", endpoint.URL, time.Second, 1024); err == nil || result.OK {
		t.Fatal("canceled speed test succeeded")
	}
}

func TestSpeedCancellationDuringBodyAndZeroBody(t *testing.T) {
	m, _, _ := mockManager(t)
	if err := m.Apply(context.Background(), lifecycleState(t)); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	var once sync.Once
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		io.WriteString(w, "partial")
		w.(http.Flusher).Flush()
		once.Do(func() { close(started) })
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer endpoint.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan model.CheckResult, 1)
	go func() { result, _ := m.Speed(ctx, "b", endpoint.URL, 3*time.Second, 1024); done <- result }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("speed download never started")
	}
	cancel()
	select {
	case result := <-done:
		if result.OK || result.Error == "" {
			t.Fatalf("canceled speed result %+v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("speed body ignored cancellation")
	}
	empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer empty.Close()
	if result, err := m.Speed(context.Background(), "b", empty.URL, time.Second, 1024); err == nil || result.OK || result.SpeedMbps != nil {
		t.Fatal("empty download reported fake throughput")
	}
}
func TestCanceledReloadCompletesRollback(t *testing.T) {
	m, starts, _ := mockManager(t)
	state := lifecycleState(t)
	if err := m.Apply(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	original := m.client.Transport
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var once sync.Once
	m.client.Transport = controllerRoundTripper(func(r *http.Request) (*http.Response, error) {
		response, err := original.RoundTrip(r)
		if r.Method == http.MethodPut {
			once.Do(cancel)
		}
		return response, err
	})
	state.Revision++
	state.Pools[0].Strategy = "consistent-hashing"
	if err := m.Apply(ctx, state); err == nil {
		t.Fatal("caller-canceled update unexpectedly committed")
	}
	if s := m.Status(); !s.Running || s.AppliedRevision != 1 || !s.Dirty || starts.Load() != 1 {
		t.Fatalf("canceled request abandoned rollback: %+v", s)
	}
}

type controllerRoundTripper func(*http.Request) (*http.Response, error)

func (f controllerRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestLifecycleConcurrentStatusAndChecks(t *testing.T) {
	m, _, _ := mockManager(t)
	state := lifecycleState(t)
	if err := m.Apply(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	var readers sync.WaitGroup
	for i := 0; i < 8; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for j := 0; j < 25; j++ {
				s := m.Status()
				if s.Listeners == nil {
					t.Error("nil collection")
				}
				_, _ = m.Check(context.Background(), "b", "http://example.invalid", time.Second)
			}
		}()
	}
	for i := 0; i < 4; i++ {
		state.Revision++
		if i%2 == 0 {
			state.Pools[0].Strategy = "consistent-hashing"
		} else {
			state.Pools[0].Strategy = "round-robin"
		}
		if err := m.Apply(context.Background(), state); err != nil {
			t.Fatal(err)
		}
	}
	readers.Wait()
}

func TestCoreChildIgnoresAmbientOverrides(t *testing.T) {
	command := exec.Command("ignored")
	command.Env = []string{"PATH=kept", "CLASH_OVERRIDE_EXTERNAL_CONTROLLER=0.0.0.0:9000", "CLASH_OVERRIDE_SECRET=", "clash_post_up=untrusted", "MIHOMO_CONFIG=unsafe", "UNIPROXY_CORE_HELPER=1"}
	secureCommandEnvironment(command)
	if !reflect.DeepEqual(command.Env, []string{"PATH=kept", "UNIPROXY_CORE_HELPER=1"}) {
		t.Fatalf("unsafe child environment: %v", command.Env)
	}
}
func TestCoreConfigurationNeverWrittenToDisk(t *testing.T) {
	m, _, _ := mockManager(t)
	state := lifecycleState(t)
	if err := m.Validate(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	if err := m.Apply(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(m.options.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".yaml") || strings.HasPrefix(entry.Name(), ".candidate-") {
			t.Fatal("plaintext credentials persisted", entry.Name())
		}
	}
	for _, arg := range m.process.cmd.Args {
		if strings.Contains(arg, "private") || strings.Contains(arg, "credential") {
			t.Fatal("credentials appeared in process arguments")
		}
	}
}
func TestCheckRejectsInvalidControllerMetrics(t *testing.T) {
	m, _, _ := mockManager(t)
	if err := m.Apply(context.Background(), lifecycleState(t)); err != nil {
		t.Fatal(err)
	}
	original := m.client.Transport
	for _, body := range []string{`{}`, `{"delay":0}`, `{"delay":-1}`, `{"delay":65536}`, `{"delay":"secret"}`} {
		m.client.Transport = controllerRoundTripper(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
		})
		if result, err := m.Check(context.Background(), "b", "http://example.invalid", time.Second); err == nil || result.OK || result.LatencyMS != nil {
			t.Fatal("invalid delay became healthy", body)
		}
	}
	m.client.Transport = original
}

func TestControllerRefusesRedirect(t *testing.T) {
	m := New(Options{})
	var leaked atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Store(true) }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer redirect.Close()
	u, _ := url.Parse(redirect.URL)
	m.buildOpts = BuildOptions{ControllerAddress: u.Host, ControllerSecret: "secret"}
	if err := m.controller(context.Background(), http.MethodGet, "/version", nil, nil); err == nil || leaked.Load() {
		t.Fatal("controller followed a redirect")
	}
}
