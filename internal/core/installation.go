package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/uniproxy/uniproxy/internal/coreinstall"
	"github.com/uniproxy/uniproxy/internal/model"
)

var ErrInstalling = errors.New("Mihomo 内核正在安装，请等待完成后再应用配置")
var ErrCoreBusy = errors.New("Mihomo 正在执行其他操作，请稍后重试")

func binaryFilename() string {
	if runtime.GOOS == "windows" {
		return "mihomo.exe"
	}
	return "mihomo"
}

// A configured path is authoritative. Bare names are PATH lookups, not writable
// installation targets. With no override, the data directory is the only online
// install destination; legacy and PATH binaries are discovered, never overwritten.
func (m *Manager) installDestination() (string, error) {
	name := m.options.BinaryPath
	if name == "" {
		return filepath.Join(m.options.DataDir, binaryFilename()), nil
	}
	if !filepath.IsAbs(name) && !strings.ContainsAny(name, "/\\") {
		return "", errors.New("已用 -core 指定 PATH 命令；请移除该覆盖或改为完整文件路径，再在线安装")
	}
	path, err := filepath.Abs(name)
	if err != nil {
		return "", errors.New("无法确定内核安装路径，请检查 -core 配置")
	}
	return path, nil
}

// discoverBinary reports existence separately from compatibility/executability.
// Invalid, unreadable, or dangling entries are not treated as permission to replace.
func (m *Manager) discoverBinary() (string, bool, error) {
	if name := m.options.BinaryPath; name != "" {
		if filepath.IsAbs(name) || strings.ContainsAny(name, "/\\") {
			return inspectPath(name)
		}
		return lookupBinary(name)
	}
	candidates := []string{filepath.Join(m.options.DataDir, binaryFilename()), filepath.Join("bin", binaryFilename())}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), binaryFilename()))
	}
	for _, candidate := range candidates {
		path, exists, err := inspectPath(candidate)
		if exists || err != nil {
			return path, exists, err
		}
	}
	return lookupBinary(binaryFilename())
}

func inspectPath(name string) (string, bool, error) {
	path, err := filepath.Abs(name)
	if err != nil {
		return "", false, errors.New("无法确定内核路径")
	}
	_, err = os.Lstat(path)
	if err == nil {
		return path, true, nil
	}
	if os.IsNotExist(err) {
		return path, false, nil
	}
	return path, false, errors.New("无法检查内核路径，请确认目录访问权限")
}

func lookupBinary(name string) (string, bool, error) {
	path, err := exec.LookPath(name)
	if errors.Is(err, exec.ErrDot) {
		return path, true, errors.New("PATH 指向当前目录中的内核；请用 -core 指定完整路径")
	}
	if err != nil {
		return "", false, nil
	}
	return inspectPath(path)
}

// InstallationStatus performs only bounded filesystem discovery, never network
// I/O or a version subprocess. It remains responsive throughout the download.
func (m *Manager) InstallationStatus() model.CoreInstallation {
	m.mu.RLock()
	s := m.installation
	r := m.status
	running := r.Running || processAlive(m.process)
	m.mu.RUnlock()
	if s.State == "" {
		s.State = "idle"
	}
	s.Version, s.OS, s.Arch = PinnedVersion, runtime.GOOS, runtime.GOARCH
	s.Supported = coreinstall.Supported(s.OS, s.Arch)
	destination, destErr := m.installDestination()
	path, exists, discoveryErr := m.discoverBinary()
	s.Path = destination
	if exists {
		s.Path = path
	}
	s.Installed = exists
	s.CanInstall, s.Reason = false, ""
	switch {
	case s.State == "installing":
		s.Reason = "内核正在后台安装，关闭窗口不会中断安装"
	case running || r.Applying:
		s.Reason = "内核正在运行或应用配置，不能在线安装或替换"
	case exists && r.Available:
		s.Reason = "已检测到可用的 Mihomo 内核，无需重复安装"
	case exists:
		s.Reason = "已检测到内核文件，但尚未确认可用；请重新检测。版本不匹配或文件损坏时需手动处理，不会覆盖已有文件"
	case discoveryErr != nil:
		s.Reason = discoveryErr.Error()
	case !s.Supported:
		s.Reason = "在线安装仅支持 Windows/Linux 的 amd64、arm64；当前平台请手动配置固定版本内核"
	case destErr != nil:
		s.Reason = destErr.Error()
	default:
		s.CanInstall = true
	}
	return s
}

func (m *Manager) installing() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.installation.State == "installing"
}

// PrepareInstall reserves the missing binary before a background worker is
// launched. The lifecycle mutex plus reservation closes the Apply/Install race.
func (m *Manager) PrepareInstall(ctx context.Context) (model.CoreInstallation, error) {
	if !m.op.TryLock() {
		return m.InstallationStatus(), ErrCoreBusy
	}
	defer m.op.Unlock()
	if err := ctx.Err(); err != nil {
		return m.InstallationStatus(), err
	}
	s := m.InstallationStatus()
	if !s.CanInstall {
		return s, errors.New(s.Reason)
	}
	s.State, s.Stage, s.Error = "installing", "metadata", ""
	s.BytesDownloaded, s.TotalBytes = 0, 0
	s.StartedAt, s.FinishedAt = model.Now(), ""
	s.CanInstall = false
	s.Reason = "内核正在后台安装，关闭窗口不会中断安装"
	m.mu.Lock()
	m.installation = s
	m.status.Available = false
	m.status.Version = ""
	m.status.Dirty = true
	m.mu.Unlock()
	return s, nil
}

// RunInstall must follow a successful reservation. The expensive download does
// not hold op or mu; Apply/Validate/Inspect observe the reservation and fail fast.
// It only verifies a binary and leaves proxy startup to explicit runtime Apply.
func (m *Manager) RunInstall(ctx context.Context) error {
	m.op.Lock()
	m.mu.Lock()
	if m.installation.State != "installing" || m.installWorker {
		m.mu.Unlock()
		m.op.Unlock()
		return ErrCoreBusy
	}
	m.installWorker = true
	path := m.installation.Path
	m.mu.Unlock()
	m.op.Unlock()

	_, err := m.install(ctx, coreinstall.Options{OS: runtime.GOOS, Arch: runtime.GOARCH, Output: path}, func(p coreinstall.Progress) {
		m.mu.Lock()
		m.installation.Stage = p.Stage
		m.installation.BytesDownloaded = p.BytesDownloaded
		m.installation.TotalBytes = p.TotalBytes
		m.mu.Unlock()
	})
	m.op.Lock()
	defer m.op.Unlock()
	if err == nil {
		m.mu.Lock()
		m.installation.Stage = "inspecting"
		m.mu.Unlock()
		// Force an uncached version probe after publication. This does not apply
		// configuration, start listeners, or execute any downloaded shell script.
		m.binary = binaryIdentity{}
		m.binaryVersion, m.binaryError = "", nil
		// Freshly downloaded executables can trigger a first-run OS security scan.
		// Allow one minute only here (ordinary probes keep their 20s budget);
		// retain the outer installation deadline and never bypass that scan.
		err = m.inspectBinaryWithTimeout(ctx, time.Minute)
		if err != nil {
			err = fmt.Errorf("内核文件已安装，但版本检测未通过；请稍后点击重新检测内核，不要重复下载：%w", err)
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.installWorker = false
	m.installation.FinishedAt = model.Now()
	m.status.Dirty = true
	if err != nil {
		m.installation.State = "failed"
		m.installation.Error = err.Error()
		m.status.LastError = err.Error()
	} else {
		m.installation.State, m.installation.Stage = "succeeded", "complete"
		m.installation.Error = ""
		m.status.LastError = ""
	}
	return err
}
