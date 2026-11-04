package sidecar

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/uniproxy/uniproxy/internal/model"
	"github.com/uniproxy/uniproxy/internal/processguard"
)

type XrayOptions struct {
	// BinaryPath is an explicit operator override. Empty means discover the
	// installer-managed versioned binary, then PATH.
	BinaryPath     string
	WorkDir        string
	PortRangeStart int
	PortRangeEnd   int
	TestTimeout    time.Duration
	StartupTimeout time.Duration
	Rand           io.Reader
}

func DefaultXrayOptions() XrayOptions {
	return XrayOptions{
		PortRangeStart: 30000,
		PortRangeEnd:   30999,
		TestTimeout:    15 * time.Second,
		StartupTimeout: 20 * time.Second,
	}
}

// XrayManager owns exactly one Xray process. Start is transactional: the
// generated document must pass xray -test and every local SOCKS inbound must
// answer an authenticated handshake before the manager reports success.
const (
	maxXrayRestarts = 3
	xrayRestartBase = 500 * time.Millisecond
)

type XrayManager struct {
	mu                sync.Mutex
	opMu              sync.Mutex
	opts              XrayOptions
	alloc             *PortAllocator
	cmd               *exec.Cmd
	done              chan struct{}
	stopping          bool
	ready             bool
	restartAttempt    int
	restartGeneration uint64
	status            model.SidecarStatus
	endpoints         map[string]model.SidecarEndpoint
	active            []model.Node
	hash              [32]byte
	dir               string
	config            string
	logs              *tailBuffer

	installation  model.CoreInstallation
	installWorker bool
	installer     xrayInstallFunc
	probeVersion  xrayVersionFunc
}

func NewXrayManager(opts XrayOptions) (*XrayManager, error) {
	defaults := DefaultXrayOptions()
	if opts.BinaryPath == "" {
		opts.BinaryPath = defaults.BinaryPath
	}
	if opts.PortRangeStart == 0 {
		opts.PortRangeStart = defaults.PortRangeStart
	}
	if opts.PortRangeEnd == 0 {
		opts.PortRangeEnd = defaults.PortRangeEnd
	}
	if opts.TestTimeout <= 0 {
		opts.TestTimeout = defaults.TestTimeout
	}
	if opts.StartupTimeout <= 0 {
		opts.StartupTimeout = defaults.StartupTimeout
	}
	if opts.Rand == nil {
		opts.Rand = cryptorand.Reader
	}
	if opts.WorkDir != "" {
		if err := os.MkdirAll(opts.WorkDir, 0o700); err != nil {
			return nil, fmt.Errorf("create Xray work directory: %w", err)
		}
	}
	alloc, err := NewPortAllocator(opts.PortRangeStart, opts.PortRangeEnd)
	if err != nil {
		return nil, err
	}
	return &XrayManager{
		opts:   opts,
		alloc:  alloc,
		status: model.SidecarStatus{Engine: model.EngineXray, Version: XrayVersion},
	}, nil
}

// ConfigurePortRange replaces the allocator used for future endpoint
// assignments. Existing persisted endpoints stay authoritative; the new range
// only affects ports allocated after this call.
func (m *XrayManager) ConfigurePortRange(start, end int) error {
	alloc, err := NewPortAllocator(start, end)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.opts.PortRangeStart = start
	m.opts.PortRangeEnd = end
	m.alloc = alloc
	return nil
}

// PortRange returns the active allocation range. Callers use it for status and
// diagnostics; the allocator itself enforces the same bounds on every
// assignment.
func (m *XrayManager) PortRange() (int, int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.opts.PortRangeStart, m.opts.PortRangeEnd
}

// Preflight verifies that every already-assigned enabled endpoint can bind
// loopback TCP and, when requested, UDP. Ports owned by the current live Xray
// process are skipped because that process legitimately holds them.
func (m *XrayManager) Preflight(nodes []model.Node) error {
	m.mu.Lock()
	running := m.cmd != nil && m.status.Running
	owned := map[int]bool{}
	if running {
		for _, endpoint := range m.endpoints {
			if endpoint.Host == "127.0.0.1" {
				owned[endpoint.Port] = true
			}
		}
	}
	m.mu.Unlock()
	checked := map[int]bool{}
	for _, node := range nodes {
		if !node.Enabled || node.EngineName() != model.EngineXray || node.SidecarEndpoint == nil || node.SidecarEndpoint.Port == 0 {
			continue
		}
		port := node.SidecarEndpoint.Port
		if owned[port] {
			continue
		}
		if checked[port] {
			return errors.New("sidecar 端点端口重复")
		}
		checked[port] = true
		ok, err := portAvailable("127.0.0.1", port, node.UDP)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("sidecar 端口 %d 已被占用", port)
		}
	}
	return nil
}

func (m *XrayManager) Status() model.SidecarStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	status := m.status
	if status.Engine == "" {
		status.Engine = model.EngineXray
	}
	if status.Version == "" {
		status.Version = XrayVersion
	}
	return status
}

func (m *XrayManager) Logs() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.logs == nil {
		return ""
	}
	return redactSecrets(m.logs.String(), m.active)
}

func (m *XrayManager) Endpoints() map[string]model.SidecarEndpoint {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]model.SidecarEndpoint, len(m.endpoints))
	for id, endpoint := range m.endpoints {
		out[id] = endpoint
	}
	return out
}

// Start launches one Xray process for all enabled Xray nodes. It returns the
// endpoint assignment so the caller can persist it in the encrypted state.
func (m *XrayManager) Start(ctx context.Context, nodes []model.Node) (map[string]model.SidecarEndpoint, error) {
	return m.start(ctx, nodes, true, 0)
}

// start is the shared implementation for explicit applies and crash recovery.
// resetAttempt starts a new crash-recovery budget; expectedGeneration prevents
// a delayed recovery from resurrecting a process after Stop or a newer apply.
func (m *XrayManager) start(ctx context.Context, nodes []model.Node, resetAttempt bool, expectedGeneration uint64) (map[string]model.SidecarEndpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if expectedGeneration != 0 && m.restartGeneration != expectedGeneration {
		return nil, errors.New("xray sidecar restart was cancelled")
	}
	if resetAttempt {
		m.restartAttempt = 0
		m.restartGeneration++
	}
	if m.cmd != nil {
		return nil, errors.New("xray sidecar is already running")
	}
	if m.installation.State == "installing" {
		return nil, ErrSidecarInstalling
	}
	m.stopping = false
	m.ready = false
	active := make([]model.Node, 0, len(nodes))
	for _, node := range nodes {
		if !node.Enabled {
			continue
		}
		switch node.EngineName() {
		case model.EngineXray:
			active = append(active, node)
		case model.EngineMihomo:
			continue
		default:
			return nil, errors.New("unsupported sidecar engine")
		}
	}
	if len(active) == 0 {
		return nil, errors.New("no enabled Xray sidecar nodes")
	}
	if len(active) > MaxSidecarNodes {
		return nil, nodeLimitError(len(active))
	}
	binary, err := m.executable()
	if err != nil {
		return nil, err
	}
	probe := m.probeVersion
	if probe == nil {
		probe = probeXrayVersion
	}
	probeCtx, cancelProbe := context.WithTimeout(ctx, m.opts.TestTimeout)
	version, probeErr := probe(probeCtx, binary)
	cancelProbe()
	if probeErr != nil {
		return nil, fmt.Errorf("Xray 版本检测失败：%w", probeErr)
	}
	if version != XrayVersion {
		return nil, fmt.Errorf("Xray 版本不匹配：需要 %s，实际 %s", XrayVersion, version)
	}
	m.status.Version = version
	m.alloc.Reset()
	prepared := make([]model.Node, 0, len(active))
	endpoints := make(map[string]model.SidecarEndpoint, len(active))
	for _, node := range active {
		assigned := node
		if node.SidecarEndpoint == nil {
			port, err := m.alloc.Allocate("127.0.0.1", node.UDP)
			if err != nil {
				m.alloc.Reset()
				return nil, err
			}
			username, password, err := randomCredential(m.opts.Rand)
			if err != nil {
				m.alloc.Reset()
				return nil, errors.New("could not generate sidecar credentials")
			}
			assigned.SidecarEndpoint = &model.SidecarEndpoint{Host: "127.0.0.1", Port: port, Username: username, Password: password}
		} else {
			if err := m.alloc.Reserve(node.SidecarEndpoint.Port); err != nil {
				m.alloc.Reset()
				return nil, err
			}
		}
		endpoints[node.ID] = *assigned.SidecarEndpoint
		prepared = append(prepared, assigned)
	}
	config, err := BuildXrayConfig(prepared)
	if err != nil {
		m.alloc.Reset()
		return nil, err
	}
	dir, err := os.MkdirTemp(m.opts.WorkDir, "uniproxy-xray-")
	if err != nil {
		m.alloc.Reset()
		return nil, errors.New("could not create Xray temporary directory")
	}
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, config, 0o600); err != nil {
		_ = os.RemoveAll(dir)
		m.alloc.Reset()
		return nil, errors.New("could not write Xray configuration")
	}
	cleanup := func() {
		_ = os.Remove(configPath)
		_ = os.RemoveAll(dir)
		m.alloc.Reset()
	}
	testCtx, cancelTest := context.WithTimeout(ctx, m.opts.TestTimeout)
	err = runXrayTest(testCtx, binary, configPath, prepared)
	cancelTest()
	if err != nil {
		cleanup()
		return nil, err
	}

	cmd := exec.Command(binary, "run", "-c", configPath)
	logs := &tailBuffer{max: 16 << 10}
	cmd.Stdout = logs
	cmd.Stderr = logs
	processguard.PrepareCommand(cmd)
	if err := cmd.Start(); err != nil {
		cleanup()
		return nil, errors.New("could not start Xray sidecar")
	}
	done := make(chan struct{})
	m.cmd = cmd
	m.done = done
	m.status.Running = true
	m.status.Error = ""
	m.endpoints = endpoints
	m.dir = dir
	m.config = configPath
	m.logs = logs
	go m.waitForExit(cmd, done, logs)

	healthCtx, cancelHealth := context.WithTimeout(ctx, m.opts.StartupTimeout)
	err = waitForSOCKS(healthCtx, endpoints)
	cancelHealth()
	if err != nil {
		m.stopping = true
		_ = cmd.Process.Kill()
		<-done
		m.cmd = nil
		m.status.Running = false
		m.status.Error = "xray sidecar did not become ready"
		m.endpoints = nil
		m.stopping = false
		m.dir = ""
		m.config = ""
		cleanup()
		return nil, err
	}
	// Xray reads the document at startup and does not reload it. Removing the
	// transient file after readiness keeps credentials off persistent disk.
	_ = os.Remove(configPath)
	m.config = ""
	m.active = append([]model.Node(nil), prepared...)
	m.hash = sha256.Sum256(config)
	m.ready = true
	return endpoints, nil
}

func (m *XrayManager) waitForExit(cmd *exec.Cmd, done chan struct{}, logs *tailBuffer) {
	_ = cmd.Wait()
	close(done)
	m.mu.Lock()
	dir := ""
	if m.cmd == cmd {
		wasReady := m.ready
		m.cmd = nil
		m.ready = false
		m.status.Running = false
		if !m.stopping {
			if wasReady {
				m.scheduleRestartLocked()
			} else {
				m.status.Error = "xray sidecar exited before it became ready"
			}
		}
		// The process no longer needs its transient configuration. Clean it up
		// here as well so a crash followed by a new Start cannot orphan a temp
		// directory containing upstream credentials.
		dir = m.dir
		m.dir = ""
		m.config = ""
	}
	m.mu.Unlock()
	if dir != "" {
		_ = os.RemoveAll(dir)
	}
}

// scheduleRestartLocked records a bounded backoff restart. The caller must hold
// m.mu. Attempts are counted per explicit Start/Apply cycle, so one permanently
// broken configuration cannot restart forever.
func (m *XrayManager) scheduleRestartLocked() {
	if m.stopping || len(m.active) == 0 {
		return
	}
	if m.restartAttempt >= maxXrayRestarts {
		m.status.Error = "xray sidecar 异常退出；自动重启次数已达上限，相关节点不可用"
		return
	}
	m.restartAttempt++
	attempt := m.restartAttempt
	delay := xrayRestartBase * time.Duration(1<<(attempt-1))
	m.status.Error = fmt.Sprintf("xray sidecar 异常退出；正在自动重启（%d/%d）", attempt, maxXrayRestarts)
	generation := m.restartGeneration
	go m.restartAfterCrash(generation, delay)
}

func (m *XrayManager) restartAfterCrash(generation uint64, delay time.Duration) {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	<-timer.C

	m.mu.Lock()
	if generation != m.restartGeneration || m.stopping || m.cmd != nil || len(m.active) == 0 {
		m.mu.Unlock()
		return
	}
	nodes := append([]model.Node(nil), m.active...)
	m.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), m.opts.TestTimeout+m.opts.StartupTimeout+30*time.Second)
	_, err := m.start(ctx, nodes, false, generation)
	cancel()
	if err == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if generation != m.restartGeneration || m.stopping || m.cmd != nil {
		return
	}
	m.status.Running = false
	m.status.Error = "xray sidecar 自动重启失败：" + err.Error()
	m.scheduleRestartLocked()
}

// Apply reconciles the single Xray process with the enabled Xray nodes. A
// configuration with the same rendered bytes reuses the live process; any
// change stops it before starting the replacement. A failed replacement tries
// to restore the previous process so a bad edit cannot take healthy nodes down.
func (m *XrayManager) Apply(ctx context.Context, nodes []model.Node) (map[string]model.SidecarEndpoint, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.mu.Lock()
	installing := m.installation.State == "installing"
	m.mu.Unlock()
	if installing {
		return nil, ErrSidecarInstalling
	}
	active := make([]model.Node, 0, len(nodes))
	for _, node := range nodes {
		if !node.Enabled {
			continue
		}
		switch node.EngineName() {
		case model.EngineMihomo:
			continue
		case model.EngineXray:
			active = append(active, node)
		default:
			return nil, errors.New("unsupported sidecar engine")
		}
	}
	if len(active) == 0 {
		if err := m.Stop(ctx); err != nil {
			return nil, err
		}
		return map[string]model.SidecarEndpoint{}, nil
	}
	if len(active) > MaxSidecarNodes {
		return nil, nodeLimitError(len(active))
	}
	prepared, err := m.AssignEndpoints(active)
	if err != nil {
		return nil, err
	}
	config, err := BuildXrayConfig(prepared)
	if err != nil {
		return nil, err
	}
	desiredHash := sha256.Sum256(config)

	m.mu.Lock()
	running := m.cmd != nil && m.status.Running
	currentHash := m.hash
	previous := append([]model.Node(nil), m.active...)
	currentEndpoints := make(map[string]model.SidecarEndpoint, len(m.endpoints))
	for id, endpoint := range m.endpoints {
		currentEndpoints[id] = endpoint
	}
	m.mu.Unlock()
	if running && currentHash == desiredHash {
		return currentEndpoints, nil
	}
	if running {
		if err := m.Stop(ctx); err != nil {
			return nil, err
		}
	}
	endpoints, err := m.Start(ctx, prepared)
	if err == nil {
		return endpoints, nil
	}
	if len(previous) > 0 {
		// The failed attempt may have consumed most of the caller's timeout
		// (xray -test + startup). The rollback is cleanup, not part of the user's
		// original apply deadline, so give it a fresh bounded context instead of
		// inheriting an almost-certainly-cancelled one.
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 45*time.Second)
		_, rollbackErr := m.Start(rollbackCtx, previous)
		cancel()
		if rollbackErr != nil {
			m.mu.Lock()
			m.status.Error = "xray sidecar apply failed; previous configuration rollback also failed"
			m.mu.Unlock()
			return nil, fmt.Errorf("%w; rollback failed: %v", err, rollbackErr)
		}
	}
	return nil, err
}

// Stop terminates the process and removes temporary material. The caller must
// stop dependent Mihomo outbounds first; this package does not own that order.
func (m *XrayManager) Stop(ctx context.Context) error {
	m.mu.Lock()
	// Invalidate any pending crash recovery before observing the process. A
	// delayed goroutine must never restart Xray after an explicit Stop.
	m.restartGeneration++
	m.restartAttempt = 0
	m.ready = false
	cmd := m.cmd
	done := m.done
	dir := m.dir
	if cmd == nil {
		m.status.Running = false
		m.status.Error = ""
		m.endpoints = nil
		m.active = nil
		m.hash = [32]byte{}
		m.alloc.Reset()
		m.mu.Unlock()
		if dir != "" {
			_ = os.RemoveAll(dir)
		}
		return nil
	}
	m.stopping = true
	m.mu.Unlock()

	_ = cmd.Process.Kill()
	select {
	case <-done:
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(10 * time.Second):
		return errors.New("xray sidecar did not stop in time")
	}

	m.mu.Lock()
	m.cmd = nil
	m.status.Running = false
	m.status.Error = ""
	m.endpoints = nil
	m.active = nil
	m.hash = [32]byte{}
	m.stopping = false
	m.alloc.Reset()
	dir = m.dir
	m.dir = ""
	m.config = ""
	m.mu.Unlock()
	if dir != "" {
		_ = os.RemoveAll(dir)
	}
	return nil
}

// Health checks every live local SOCKS inbound. A process crash is visible as
// Running=false before this is called; a stuck inbound fails the probe.
func (m *XrayManager) Health(ctx context.Context) error {
	m.mu.Lock()
	running := m.status.Running && m.cmd != nil
	endpoints := make([]model.SidecarEndpoint, 0, len(m.endpoints))
	for _, endpoint := range m.endpoints {
		endpoints = append(endpoints, endpoint)
	}
	m.mu.Unlock()
	if !running {
		return errors.New("xray sidecar is not running")
	}
	sort.Slice(endpoints, func(i, j int) bool { return endpoints[i].Port < endpoints[j].Port })
	for _, endpoint := range endpoints {
		if err := ProbeSOCKS5(ctx, endpoint, 2*time.Second); err != nil {
			return fmt.Errorf("sidecar endpoint %s:%d is unavailable", endpoint.Host, endpoint.Port)
		}
	}
	return nil
}

func runXrayTest(ctx context.Context, binary, configPath string, nodes []model.Node) error {
	cmd := exec.CommandContext(ctx, binary, "run", "-test", "-c", configPath)
	processguard.PrepareCommand(cmd)
	logs := &tailBuffer{max: 8 << 10}
	cmd.Stdout = logs
	cmd.Stderr = logs
	err := cmd.Run()
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return errors.New("xray configuration test timed out")
	}
	detail := redactSidecarOutput(logs.String(), nodes)
	if detail == "" {
		return errors.New("xray configuration test failed")
	}
	return fmt.Errorf("xray configuration test failed: %s", detail)
}

func nodeLimitError(count int) error {
	return fmt.Errorf("Xray sidecar 单进程最多支持 %d 个节点（当前 %d 个），请拆分或仅启用 Xray-only 节点", MaxSidecarNodes, count)
}

func redactSecrets(output string, nodes []model.Node) string {
	for _, node := range nodes {
		if node.SidecarEndpoint == nil {
			continue
		}
		if node.SidecarEndpoint.Username != "" {
			output = strings.ReplaceAll(output, node.SidecarEndpoint.Username, "[redacted]")
		}
		if node.SidecarEndpoint.Password != "" {
			output = strings.ReplaceAll(output, node.SidecarEndpoint.Password, "[redacted]")
		}
	}
	return output
}

func redactSidecarOutput(output string, nodes []model.Node) string {
	output = redactSecrets(output, nodes)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) > 8 {
		lines = lines[len(lines)-8:]
	}
	return strings.TrimSpace(strings.Join(lines, " | "))
}

func waitForSOCKS(ctx context.Context, endpoints map[string]model.SidecarEndpoint) error {
	pending := make([]model.SidecarEndpoint, 0, len(endpoints))
	for _, endpoint := range endpoints {
		pending = append(pending, endpoint)
	}
	sort.Slice(pending, func(i, j int) bool { return pending[i].Port < pending[j].Port })
	for len(pending) > 0 {
		remaining := pending[:0]
		for _, endpoint := range pending {
			attemptCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
			err := ProbeSOCKS5(attemptCtx, endpoint, 500*time.Millisecond)
			cancel()
			if err != nil {
				remaining = append(remaining, endpoint)
			}
		}
		pending = remaining
		if len(pending) == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.New("timed out waiting for sidecar SOCKS endpoints")
		case <-time.After(100 * time.Millisecond):
		}
	}
	return nil
}

// ProbeSOCKS5 performs only RFC 1929 username/password authentication. It does
// not open a remote connection, so it verifies the sidecar inbound itself.
func ProbeSOCKS5(ctx context.Context, endpoint model.SidecarEndpoint, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = time.Second
	}
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp4", net.JoinHostPort(endpoint.Host, strconv.Itoa(endpoint.Port)))
	if err != nil {
		return err
	}
	defer conn.Close()
	deadline := time.Now().Add(timeout)
	_ = conn.SetDeadline(deadline)
	if _, err := conn.Write([]byte{5, 1, 2}); err != nil {
		return err
	}
	reply := make([]byte, 2)
	if _, err := io.ReadFull(conn, reply); err != nil {
		return err
	}
	if reply[0] != 5 || reply[1] != 2 {
		return errors.New("SOCKS5 proxy did not request username authentication")
	}
	if len(endpoint.Username) > 255 || len(endpoint.Password) > 255 {
		return errors.New("SOCKS5 credentials are too long")
	}
	auth := make([]byte, 0, 3+len(endpoint.Username)+len(endpoint.Password))
	auth = append(auth, 1, byte(len(endpoint.Username)))
	auth = append(auth, endpoint.Username...)
	auth = append(auth, byte(len(endpoint.Password)))
	auth = append(auth, endpoint.Password...)
	if _, err := conn.Write(auth); err != nil {
		return err
	}
	if _, err := io.ReadFull(conn, reply); err != nil {
		return err
	}
	if reply[1] != 0 {
		return errors.New("SOCKS5 authentication was rejected")
	}
	return nil
}

func randomCredential(reader io.Reader) (string, string, error) {
	if reader == nil {
		reader = cryptorand.Reader
	}
	userBytes := make([]byte, 8)
	passBytes := make([]byte, 24)
	if _, err := io.ReadFull(reader, userBytes); err != nil {
		return "", "", err
	}
	if _, err := io.ReadFull(reader, passBytes); err != nil {
		return "", "", err
	}
	return "u" + hex.EncodeToString(userBytes), base64.RawURLEncoding.EncodeToString(passBytes), nil
}

type tailBuffer struct {
	mu  sync.Mutex
	max int
	buf []byte
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	if len(b.buf) > b.max {
		b.buf = append([]byte(nil), b.buf[len(b.buf)-b.max:]...)
	}
	return len(p), nil
}

func (b *tailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}
