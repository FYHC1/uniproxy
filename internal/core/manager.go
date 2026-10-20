package core

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/uniproxy/uniproxy/internal/coreinstall"
	"github.com/uniproxy/uniproxy/internal/model"
	"gopkg.in/yaml.v3"
)

var ErrUnavailable = errors.New("Mihomo core is not installed")
var ErrNotRunning = errors.New("Mihomo core is not running")
var ErrUnsupportedVersion = errors.New("unsupported Mihomo core version")

// Options deliberately does not expose the core controller to API clients.
type Options struct {
	BinaryPath, DataDir string
	Log                 func(level, message string)
}

type process struct {
	cmd      *exec.Cmd
	done     chan struct{}
	stopping bool
	err      error
}
type binaryIdentity struct {
	path     string
	size     int64
	modified time.Time
}

// Manager owns exactly one child. op serializes lifecycle transactions; mu
// protects fast status/test snapshots. No UI/API lock need be held during Apply.
type Manager struct {
	op            sync.Mutex
	mu            sync.RWMutex
	options       Options
	status        model.RuntimeStatus
	buildOpts     BuildOptions
	binary        binaryIdentity
	binaryVersion string
	binaryError   error
	process       *process
	live          *compiledConfig
	client        *http.Client
	logs          *boundedLog
	command       func(context.Context, string, ...string) *exec.Cmd
	startCommand  func(string, ...string) *exec.Cmd
	installation  model.CoreInstallation
	installWorker bool
	install       func(context.Context, coreinstall.Options, func(coreinstall.Progress)) (string, error)
}

func New(options Options) *Manager {
	if options.DataDir == "" {
		options.DataDir = filepath.Join("data", "core")
	}
	dir, err := filepath.Abs(options.DataDir)
	if err == nil {
		options.DataDir = dir
	}
	return &Manager{options: options, status: model.RuntimeStatus{Dirty: true, Listeners: []model.ListenerRuntime{}}, logs: &boundedLog{limit: 32 << 10}, install: coreinstall.Install, command: exec.CommandContext, startCommand: exec.Command, client: &http.Client{Transport: &http.Transport{Proxy: nil, MaxIdleConnsPerHost: 8, ResponseHeaderTimeout: 35 * time.Second, MaxResponseHeaderBytes: 64 << 10}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (m *Manager) Status() model.RuntimeStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s := m.status
	s.Listeners = append([]model.ListenerRuntime{}, s.Listeners...)
	return s
}
func (m *Manager) Inspect(ctx context.Context) model.RuntimeStatus {
	if m.installing() {
		return m.Status()
	}
	m.op.Lock()
	defer m.op.Unlock()
	if m.installing() {
		return m.Status()
	}
	if err := m.inspectBinary(ctx); err != nil {
		m.mu.Lock()
		m.status.LastError = err.Error()
		m.status.Dirty = true
		m.mu.Unlock()
	} else {
		m.mu.Lock()
		// A previously timed-out post-install probe can be retried without
		// replacing the verified binary or leaving a stale installation failure.
		if m.installation.State == "failed" {
			if m.installation.Stage == "inspecting" {
				m.installation.State, m.installation.Stage = "succeeded", "complete"
				m.installation.Error = ""
				m.installation.FinishedAt = model.Now()
			} else {
				// A host administrator may have installed it manually after a
				// failed download. Clear obsolete task errors, but do not claim
				// our download succeeded or keep stale transferred byte counts.
				m.installation = model.CoreInstallation{State: "idle"}
			}
			m.status.LastError = ""
		}
		m.mu.Unlock()
	}
	return m.Status()
}

func (m *Manager) Validate(ctx context.Context, state model.State) error {
	if m.installing() {
		return ErrInstalling
	}
	m.op.Lock()
	defer m.op.Unlock()
	if m.installing() {
		return ErrInstalling
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := m.ensureBuildOptions(); err != nil {
		return err
	}
	cfg, err := compile(state, m.buildOpts)
	if err != nil {
		return err
	}
	if err = m.inspectBinary(ctx); err != nil {
		return err
	}
	return m.validateConfig(ctx, cfg.data)
}

// Apply validates before touching live traffic, applies native config in-process,
// then verifies listener handshakes. On normal reload no process is restarted and
// no /connections deletion is issued. Core group selection/cache and UDP sessions
// can reset on reload; changed binds/auth briefly recreate accept sockets. Only
// failed rollback recovery may require a process restart (and reset connections).
func (m *Manager) Apply(ctx context.Context, state model.State) (err error) {
	if m.installing() {
		return ErrInstalling
	}
	m.op.Lock()
	defer m.op.Unlock()
	if m.installing() {
		return ErrInstalling
	}
	m.mu.Lock()
	m.status.Applying = true
	m.status.Dirty = true
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.status.Applying = false
		if err != nil {
			m.status.Dirty = true
			m.status.LastError = err.Error()
		}
		m.mu.Unlock()
	}()
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = m.ensureBuildOptions(); err != nil {
		return err
	}
	cfg, err := compile(state, m.buildOpts)
	if err != nil {
		return err
	}
	if err = m.inspectBinary(ctx); err != nil {
		return err
	}
	if err = m.validateConfig(ctx, cfg.data); err != nil {
		return err
	}
	m.mu.RLock()
	old := m.live
	running := m.status.Running && processAlive(m.process)
	m.mu.RUnlock()
	if running && old != nil && bytes.Equal(old.data, cfg.data) {
		if err = m.verify(ctx, cfg); err != nil {
			return m.recoverApply(old, cfg, err)
		}
		return m.commit(cfg)
	}
	release, err := m.preflight(cfg, old, running)
	if err != nil {
		return err
	}
	defer release()
	if running {
		// Mihomo starts new names before dropping removed names. Free only conflicting
		// old accepts in a staged reload, so atomic port swaps do not depend on map order.
		staged, stageErr := withoutConflictingListeners(old, cfg)
		if stageErr != nil {
			return stageErr
		}
		release()
		if staged != nil {
			if err = m.reload(ctx, staged.data); err != nil {
				return m.recoverApply(old, cfg, err)
			}
		}
		mark := m.logs.mark()
		if err = m.reload(ctx, cfg.data); err == nil {
			err = m.verify(ctx, cfg)
		}
		if err == nil && m.logs.listenerErrorSince(mark) {
			err = errors.New("Mihomo could not bind a requested listener")
		}
		if err != nil {
			return m.recoverApply(old, cfg, err)
		}
	} else {
		release()
		mark := m.logs.mark()
		if err = m.start(ctx, cfg.data); err == nil {
			err = m.verify(ctx, cfg)
		}
		if err == nil && m.logs.listenerErrorSince(mark) {
			err = errors.New("Mihomo could not bind a requested listener")
		}
		if err != nil {
			return m.recoverApply(nil, cfg, err)
		}
	}
	if err = m.commit(cfg); err != nil {
		return m.recoverApply(old, cfg, err)
	}
	m.log("info", "Mihomo configuration applied; existing TCP connections are retained where the core supports it")
	return nil
}

func (m *Manager) Stop(ctx context.Context) error {
	m.op.Lock()
	defer m.op.Unlock()
	err := m.stopProcess(ctx)
	m.mu.Lock()
	m.status.Running = false
	m.status.Applying = false
	m.status.Dirty = true
	m.status.StartedAt = ""
	for i := range m.status.Listeners {
		m.status.Listeners[i].Listening = false
	}
	if err != nil {
		m.status.LastError = err.Error()
	} else {
		m.status.LastError = ""
	}
	m.mu.Unlock()
	m.log("info", "Mihomo stopped; proxy connections were closed")
	return err
}

func (m *Manager) inspectBinary(ctx context.Context) error {
	return m.inspectBinaryWithTimeout(ctx, 20*time.Second)
}

func (m *Manager) inspectBinaryWithTimeout(ctx context.Context, timeout time.Duration) error {
	resolved, exists, err := m.discoverBinary()
	if err != nil {
		m.setBinaryStatus(false, "")
		return err
	}
	if !exists {
		m.setBinaryStatus(false, "")
		return ErrUnavailable
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		m.setBinaryStatus(false, "")
		return ErrUnavailable
	}
	ident := binaryIdentity{resolved, info.Size(), info.ModTime()}
	if ident == m.binary && m.binaryVersion != "" {
		m.setBinaryStatus(m.binaryError == nil, m.binaryVersion)
		return m.binaryError
	}
	// A version probe is a one-line print, but the first exec of a freshly written
	// binary can be slow when an antivirus scanner inspects it or the disk is busy.
	// A five second cap produced spurious "core unavailable" results on such hosts,
	// so allow a realistic cold-start budget while staying bounded.
	inspectCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	output := &boundedLog{limit: 16 << 10}
	cmd := m.command(inspectCtx, resolved, "-v")
	secureCommandEnvironment(cmd)
	cmd.Stdout = output
	cmd.Stderr = output
	cmd.WaitDelay = time.Second
	if err = cmd.Run(); err != nil {
		m.setBinaryStatus(false, "")
		if inspectCtx.Err() != nil {
			return errors.New("Mihomo version inspection timed out or was canceled")
		}
		return errors.New("could not execute Mihomo version inspection")
	}
	version := extractVersion(output.text())
	if version == "" {
		m.setBinaryStatus(false, "")
		return errors.New("binary did not identify itself as a supported Mihomo release")
	}
	m.binary = ident
	m.binaryVersion = version
	m.binaryError = nil
	if version != PinnedVersion {
		m.binaryError = fmt.Errorf("%w: require v%s, found v%s", ErrUnsupportedVersion, PinnedVersion, version)
	}
	m.setBinaryStatus(m.binaryError == nil, version)
	return m.binaryError
}

var versionPattern = regexp.MustCompile(`(?im)\bMihomo(?:\s+Meta)?\s+v?([0-9]+\.[0-9]+\.[0-9]+)(?:\s|$)`)

func extractVersion(s string) string {
	v := versionPattern.FindStringSubmatch(s)
	if len(v) == 2 {
		return v[1]
	}
	return ""
}
func (m *Manager) setBinaryStatus(available bool, version string) {
	m.mu.Lock()
	m.status.Available = available
	m.status.Version = version
	m.mu.Unlock()
}

func (m *Manager) ensureBuildOptions() error {
	if m.buildOpts.ControllerAddress != "" {
		return nil
	}
	controller, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return errors.New("cannot allocate private core controller")
	}
	defer controller.Close()
	probe, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return errors.New("cannot allocate private core probe")
	}
	defer probe.Close()
	secret := make([]byte, 64)
	if _, err = rand.Read(secret); err != nil {
		return errors.New("cannot generate core authentication secrets")
	}
	m.mu.Lock()
	m.buildOpts = BuildOptions{ControllerAddress: controller.Addr().String(), ControllerSecret: base64.RawURLEncoding.EncodeToString(secret[:32]), ProbeAddress: probe.Addr().String(), ProbeSecret: base64.RawURLEncoding.EncodeToString(secret[32:])}
	m.mu.Unlock()
	return nil
}

// The pinned core accepts -f - for both validation and startup. Secrets travel
// over child stdin and the authenticated loopback controller, never command-line
// arguments, environment variables, or persistent plaintext runtime YAML.
func (m *Manager) validateConfig(ctx context.Context, data []byte) error {
	if err := os.MkdirAll(m.options.DataDir, 0700); err != nil {
		return errors.New("cannot create private core data directory")
	}
	validationCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	output := &boundedLog{limit: 32 << 10}
	cmd := m.command(validationCtx, m.binary.path, "-t", "-d", m.options.DataDir, "-f", "-")
	cmd.Stdin = bytes.NewReader(data)
	secureCommandEnvironment(cmd)
	cmd.Stdout = output
	cmd.Stderr = output
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil {
		if validationCtx.Err() != nil {
			return errors.New("Mihomo configuration validation timed out or was canceled")
		}
		return errors.New("Mihomo rejected the generated configuration; check native outbound options (raw diagnostics withheld to protect credentials)")
	}
	return nil
}

func (m *Manager) start(ctx context.Context, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	cmd := m.startCommand(m.binary.path, "-d", m.options.DataDir, "-f", "-")
	cmd.Stdin = bytes.NewReader(data)
	secureCommandEnvironment(cmd)
	cmd.Stdout = m.logs
	cmd.Stderr = m.logs
	cmd.WaitDelay = time.Second
	if err := cmd.Start(); err != nil {
		return errors.New("could not start the Mihomo process")
	}
	p := &process{cmd: cmd, done: make(chan struct{})}
	m.mu.Lock()
	m.process = p
	m.status.StartedAt = model.Now()
	m.mu.Unlock()
	go func() {
		p.err = cmd.Wait()
		close(p.done)
		m.mu.Lock()
		unexpected := m.process == p && !p.stopping
		if m.process == p {
			m.status.Running = false
			m.status.Dirty = true
			for i := range m.status.Listeners {
				m.status.Listeners[i].Listening = false
			}
			if unexpected {
				m.status.LastError = "Mihomo exited unexpectedly; proxy listeners are stopped"
			}
		}
		m.mu.Unlock()
		if unexpected {
			m.log("error", "Mihomo exited unexpectedly; proxy listeners are stopped")
		}
	}()
	readyCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	ticker := time.NewTicker(75 * time.Millisecond)
	defer ticker.Stop()
	for {
		if !processAlive(p) {
			return errors.New("Mihomo exited before its authenticated controller became ready")
		}
		if err := m.controllerReady(readyCtx); err == nil {
			m.mu.Lock()
			m.status.Running = true
			m.mu.Unlock()
			return nil
		}
		select {
		case <-readyCtx.Done():
			return errors.New("Mihomo authenticated controller did not become ready")
		case <-p.done:
			return errors.New("Mihomo exited during startup")
		case <-ticker.C:
		}
	}
}
func processAlive(p *process) bool {
	if p == nil {
		return false
	}
	select {
	case <-p.done:
		return false
	default:
		return true
	}
}

func (m *Manager) stopProcess(ctx context.Context) error {
	m.mu.Lock()
	p := m.process
	if p != nil {
		p.stopping = true
	}
	m.mu.Unlock()
	if p == nil || !processAlive(p) {
		return nil
	}
	if runtime.GOOS == "windows" {
		_ = p.cmd.Process.Kill()
	} else {
		_ = p.cmd.Process.Signal(os.Interrupt)
	}
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		_ = p.cmd.Process.Kill()
	case <-timer.C:
		_ = p.cmd.Process.Kill()
	}
	cleanup := time.NewTimer(3 * time.Second)
	defer cleanup.Stop()
	select {
	case <-p.done:
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return nil
	case <-cleanup.C:
		return errors.New("Mihomo process did not terminate after being killed")
	}
}

func (m *Manager) commit(cfg *compiledConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !processAlive(m.process) {
		return errors.New("Mihomo exited before configuration commit")
	}
	m.live = cfg
	m.status.Running = true
	m.status.Dirty = false
	m.status.LastError = ""
	m.status.AppliedRevision = cfg.revision
	m.status.Listeners = append([]model.ListenerRuntime{}, cfg.runtime...)
	enabled := map[string]bool{}
	for _, l := range cfg.listeners {
		enabled[l.ID] = true
	}
	for i := range m.status.Listeners {
		m.status.Listeners[i].Listening = enabled[m.status.Listeners[i].ID]
	}
	return nil
}

func (m *Manager) recoverApply(old, attempted *compiledConfig, cause error) error {
	// Even if the HTTP caller canceled, complete bounded rollback independently.
	recoveryCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if old != nil {
		// Reverse a port swap before restoring old binds. The core attempts new
		// listeners before it drops old names, so a direct rollback can collide.
		if stage, err := withoutConflictingListeners(attempted, old); err == nil && stage != nil {
			_ = m.reload(recoveryCtx, stage.data)
		}
		mark := m.logs.mark()
		if err := m.reload(recoveryCtx, old.data); err == nil {
			if err = m.verify(recoveryCtx, old); err == nil && !m.logs.listenerErrorSince(mark) {
				if err = m.commit(old); err == nil {
					m.log("warning", "Core update failed; the previous live configuration was restored without restarting")
					return fmt.Errorf("%w; previous live configuration restored", cause)
				}
			}
		}
		m.log("warning", "Core reload rollback failed; restarting the previous configuration will reset established connections")
		_ = m.stopProcess(recoveryCtx)
		if recoveryCtx.Err() == nil {
			mark = m.logs.mark()
			if err := m.start(recoveryCtx, old.data); err == nil {
				if err = m.verify(recoveryCtx, old); err == nil && !m.logs.listenerErrorSince(mark) {
					if err = m.commit(old); err == nil {
						return fmt.Errorf("%w; previous configuration recovered by restart (established connections reset)", cause)
					}
				}
			}
		}
	}
	_ = m.stopProcess(recoveryCtx)
	m.mu.Lock()
	m.status.Running = false
	m.status.Dirty = true
	m.status.StartedAt = ""
	for i := range m.status.Listeners {
		m.status.Listeners[i].Listening = false
	}
	m.mu.Unlock()
	return fmt.Errorf("%w; core stopped to prevent a partially applied configuration", cause)
}

// Ignore inherited Mihomo/Clash overrides. An ambient CLASH_OVERRIDE_SECRET,
// controller override or post-up command must never invalidate our private config.
func secureCommandEnvironment(cmd *exec.Cmd) {
	environment := cmd.Env
	if environment == nil {
		environment = os.Environ()
	}
	filtered := make([]string, 0, len(environment))
	for _, entry := range environment {
		key, _, _ := strings.Cut(entry, "=")
		upper := strings.ToUpper(key)
		if strings.HasPrefix(upper, "CLASH_") || strings.HasPrefix(upper, "MIHOMO_") {
			continue
		}
		filtered = append(filtered, entry)
	}
	cmd.Env = filtered
}

func (m *Manager) log(level, message string) {
	if m.options.Log != nil {
		m.options.Log(level, message)
	}
}

func (m *Manager) preflight(next, old *compiledConfig, running bool) (func(), error) {
	var held []io.Closer
	var once sync.Once
	release := func() {
		once.Do(func() {
			for _, c := range held {
				_ = c.Close()
			}
		})
	}
	listeners := append([]model.Listener{}, next.listeners...)
	if !running {
		host, port, _ := privateAddress(m.buildOpts.ControllerAddress)
		listeners = append(listeners, next.probe, model.Listener{ID: "controller", Host: host, Port: port})
	}
	for _, l := range listeners {
		tcpOwned, udpOwned := false, false
		if running && old != nil {
			for _, prev := range old.listeners {
				if addressesOverlap(l.Host, l.Port, prev.Host, prev.Port) {
					tcpOwned = true
					udpOwned = prev.UDP
				}
			}
		}
		address := net.JoinHostPort(l.Host, fmt.Sprint(l.Port))
		if !tcpOwned {
			socket, err := net.Listen("tcp", address)
			if err != nil {
				release()
				return func() {}, fmt.Errorf("listener %s: TCP bind is unavailable", l.ID)
			}
			held = append(held, socket)
		}
		if l.UDP && !udpOwned {
			socket, err := net.ListenPacket("udp", address)
			if err != nil {
				release()
				return func() {}, fmt.Errorf("listener %s: UDP bind is unavailable", l.ID)
			}
			held = append(held, socket)
		}
	}
	return release, nil
}

func withoutConflictingListeners(old, next *compiledConfig) (*compiledConfig, error) {
	if old == nil {
		return nil, nil
	}
	removed := map[string]bool{}
	for _, prev := range old.listeners {
		for _, n := range next.listeners {
			if prev.ID != n.ID && addressesOverlap(prev.Host, prev.Port, n.Host, n.Port) {
				removed[listenerTag(prev.ID)] = true
			}
		}
	}
	if len(removed) == 0 {
		return nil, nil
	}
	var raw map[string]any
	if err := yaml.Unmarshal(old.data, &raw); err != nil {
		return nil, errors.New("cannot prepare listener bind transition")
	}
	entries, _ := raw["listeners"].([]any)
	kept := make([]any, 0, len(entries))
	for _, entry := range entries {
		v, _ := entry.(map[string]any)
		name, _ := v["name"].(string)
		if !removed[name] {
			kept = append(kept, entry)
		}
	}
	raw["listeners"] = kept
	data, err := yaml.Marshal(raw)
	if err != nil {
		return nil, errors.New("cannot encode listener bind transition")
	}
	return &compiledConfig{data: data}, nil
}

// boundedLog never streams raw core output into application activities. Mihomo
// errors can contain credentials, private keys and test URLs. Only fixed, safe
// messages leave this package; the bounded bytes are used for version/bind checks.
type boundedLog struct {
	mu    sync.Mutex
	data  []byte
	total int64
	limit int
}

func (b *boundedLog) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	b.total += int64(n)
	if b.limit <= 0 {
		b.limit = 32 << 10
	}
	if len(p) >= b.limit {
		b.data = append(b.data[:0], p[len(p)-b.limit:]...)
	} else {
		b.data = append(b.data, p...)
		if len(b.data) > b.limit {
			b.data = append(b.data[:0], b.data[len(b.data)-b.limit:]...)
		}
	}
	return n, nil
}
func (b *boundedLog) text() string { b.mu.Lock(); defer b.mu.Unlock(); return string(b.data) }
func (b *boundedLog) mark() int64  { b.mu.Lock(); defer b.mu.Unlock(); return b.total }
func (b *boundedLog) listenerErrorSince(mark int64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	count := b.total - mark
	if count <= 0 {
		return false
	}
	if count > int64(len(b.data)) {
		count = int64(len(b.data))
	}
	s := strings.ToLower(string(b.data[len(b.data)-int(count):]))
	return strings.Contains(s, "listen err:") || strings.Contains(s, "address already in use") || strings.Contains(s, "failed to listen")
}
