package sidecar

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

	"github.com/uniproxy/uniproxy/internal/model"
	"github.com/uniproxy/uniproxy/internal/sidecarinstall"
)

// ErrSidecarInstalling blocks configuration application while the fixed Xray
// release is being downloaded and verified. Installation never starts proxy
// traffic; the administrator must apply configuration explicitly afterwards.
var ErrSidecarInstalling = errors.New("Xray sidecar 正在安装，请等待完成后再应用配置")

// ErrSidecarBusy means another lifecycle operation owns the manager.
var ErrSidecarBusy = errors.New("Xray sidecar 正在执行其他操作，请稍后重试")

type xrayInstallFunc func(context.Context, sidecarinstall.Options, func(sidecarinstall.Progress)) (sidecarinstall.Result, error)
type xrayVersionFunc func(context.Context, string) (string, error)

func xrayBinaryName() string {
	if runtime.GOOS == "windows" {
		return "xray.exe"
	}
	return "xray"
}

// installDestination returns the versioned directory and binary path managed by
// the online installer. The version segment makes replacement explicit: an
// existing directory is never overwritten.
func (m *XrayManager) installDestination() (string, string, error) {
	if strings.TrimSpace(m.opts.WorkDir) == "" {
		return "", "", errors.New("未配置 sidecar 工作目录，无法在线安装 Xray")
	}
	dir := filepath.Join(m.opts.WorkDir, "xray", sidecarinstall.PinnedVersion)
	return dir, filepath.Join(dir, xrayBinaryName()), nil
}

func (m *XrayManager) inspectBinary(name string) (string, bool, error) {
	path, err := filepath.Abs(name)
	if err != nil {
		return "", false, errors.New("无法确定 Xray 可执行文件路径")
	}
	if _, err := os.Lstat(path); err == nil {
		return path, true, nil
	} else if !os.IsNotExist(err) {
		return path, false, errors.New("无法检查 Xray 可执行文件，请确认目录访问权限")
	}
	return path, false, nil
}

func lookupXrayBinary(name string) (string, bool, error) {
	path, err := exec.LookPath(name)
	if errors.Is(err, exec.ErrDot) {
		return path, true, errors.New("PATH 指向当前目录中的 Xray；请用 -xray 指定完整路径")
	}
	if err != nil {
		return "", false, nil
	}
	return path, true, nil
}

// discoverBinary resolves the configured override first, then the installer's
// versioned directory, then PATH. A missing binary is a normal state before an
// online installation.
func (m *XrayManager) discoverBinary() (string, bool, error) {
	if override := strings.TrimSpace(m.opts.BinaryPath); override != "" {
		if filepath.IsAbs(override) || strings.ContainsAny(override, "/\\") {
			return m.inspectBinary(override)
		}
		return lookupXrayBinary(override)
	}
	if _, binary, err := m.installDestination(); err == nil {
		if path, exists, inspectErr := m.inspectBinary(binary); exists || inspectErr != nil {
			return path, exists, inspectErr
		}
	}
	return lookupXrayBinary(xrayBinaryName())
}

func (m *XrayManager) executable() (string, error) {
	path, exists, err := m.discoverBinary()
	if err != nil {
		return "", err
	}
	if !exists {
		return "", errors.New("未找到 Xray 可执行文件；请先在设置中在线安装，或用 -xray 指定完整路径")
	}
	return path, nil
}

// InstallationStatus performs only bounded filesystem discovery. It never
// downloads, executes the binary, or reports success merely because a previous
// installation record exists.
func (m *XrayManager) InstallationStatus() model.CoreInstallation {
	m.mu.Lock()
	s := m.installation
	running := m.status.Running || m.cmd != nil
	m.mu.Unlock()
	if s.State == "" {
		s.State = "idle"
	}
	s.Version, s.OS, s.Arch = sidecarinstall.PinnedVersion, runtime.GOOS, runtime.GOARCH
	s.Supported = sidecarinstall.Supported(s.OS, s.Arch)

	destination, destinationBinary, destinationErr := m.installDestination()
	if destination == "" {
		s.Path = ""
	} else {
		s.Path = destinationBinary
	}
	path, exists, discoveryErr := m.discoverBinary()
	if exists {
		s.Path = path
	}
	s.Installed = exists
	destinationPresent := false
	if destination != "" {
		if _, err := os.Lstat(destination); err == nil {
			destinationPresent = true
		} else if !os.IsNotExist(err) {
			discoveryErr = errors.New("无法检查 Xray 安装目录，请确认目录访问权限")
		}
	}
	s.CanInstall, s.Reason = false, ""
	switch {
	case s.State == "installing":
		s.Reason = "Xray 正在后台安装，关闭窗口不会中断安装"
	case running:
		s.Reason = "Xray 正在运行，不能在线安装或替换"
	case exists:
		s.Reason = "已检测到 Xray 可执行文件，无需重复安装"
	case discoveryErr != nil:
		s.Reason = discoveryErr.Error()
	case !s.Supported:
		s.Reason = "在线安装仅支持 Windows/Linux 的 amd64、arm64；当前平台请手动配置固定版本 Xray"
	case destinationErr != nil:
		s.Reason = destinationErr.Error()
	case destinationPresent:
		s.Reason = "Xray 安装目录已存在，不会覆盖；请检查文件完整性或手动处理"
	default:
		s.CanInstall = true
	}
	return s
}

// PrepareInstall reserves the missing versioned destination before a background
// worker is launched. opMu closes the race with Apply without holding it during
// the download.
func (m *XrayManager) PrepareInstall(ctx context.Context) (model.CoreInstallation, error) {
	if !m.opMu.TryLock() {
		return m.InstallationStatus(), ErrSidecarBusy
	}
	defer m.opMu.Unlock()
	if ctx == nil || ctx.Err() != nil {
		return m.InstallationStatus(), errors.New("Xray 安装请求已取消，请重新发起")
	}
	s := m.InstallationStatus()
	if !s.CanInstall {
		return s, errors.New(s.Reason)
	}
	dir, binary, err := m.installDestination()
	if err != nil {
		return s, err
	}
	if dir == "" {
		return s, errors.New("未配置 sidecar 工作目录，无法在线安装 Xray")
	}
	s.State, s.Stage, s.Error = "installing", "metadata", ""
	s.Path = binary
	s.BytesDownloaded, s.TotalBytes = 0, 0
	s.StartedAt, s.FinishedAt = model.Now(), ""
	s.CanInstall = false
	s.Reason = "Xray 正在后台安装，关闭窗口不会中断安装"
	m.mu.Lock()
	if m.installation.State == "installing" {
		m.mu.Unlock()
		return m.InstallationStatus(), ErrSidecarBusy
	}
	m.installation = s
	m.mu.Unlock()
	return s, nil
}

// RunInstall must follow a successful reservation. It verifies the pinned
// version after publication and never applies configuration or starts a proxy.
func (m *XrayManager) RunInstall(ctx context.Context) error {
	m.opMu.Lock()
	m.mu.Lock()
	if m.installation.State != "installing" || m.installWorker {
		m.mu.Unlock()
		m.opMu.Unlock()
		return ErrSidecarBusy
	}
	m.installWorker = true
	binaryPath := m.installation.Path
	m.mu.Unlock()
	m.opMu.Unlock()

	defer func() {
		m.mu.Lock()
		m.installWorker = false
		m.mu.Unlock()
	}()

	installer := m.installer
	if installer == nil {
		installer = sidecarinstall.Install
	}
	result, err := installer(ctx, sidecarinstall.Options{OS: runtime.GOOS, Arch: runtime.GOARCH, Dir: filepath.Dir(binaryPath)}, func(p sidecarinstall.Progress) {
		m.mu.Lock()
		m.installation.Stage = p.Stage
		m.installation.BytesDownloaded = p.BytesDownloaded
		m.installation.TotalBytes = p.TotalBytes
		m.mu.Unlock()
	})
	version := ""
	if err == nil {
		m.mu.Lock()
		m.installation.Stage = "inspecting"
		m.mu.Unlock()
		probe := m.probeVersion
		if probe == nil {
			probe = probeXrayVersion
		}
		probeCtx, cancel := context.WithTimeout(ctx, time.Minute)
		version, err = probe(probeCtx, result.Binary)
		cancel()
		if err == nil && version != sidecarinstall.PinnedVersion {
			err = fmt.Errorf("Xray 版本不匹配：需要 %s，实际 %s", sidecarinstall.PinnedVersion, version)
		}
		if err != nil {
			err = fmt.Errorf("Xray 文件已安装，但版本检测未通过；请检查文件后重试：%w", err)
			// The installer publishes a new directory before returning. A failed
			// post-publication probe must not leave that directory behind, or
			// InstallationStatus would report CanInstall=false forever.
			if expectedDir, _, destinationErr := m.installDestination(); destinationErr == nil && filepath.Clean(result.Dir) == filepath.Clean(expectedDir) {
				if cleanupErr := os.RemoveAll(expectedDir); cleanupErr != nil {
					err = fmt.Errorf("%w；清理未通过版本检测的安装目录失败：%v", err, cleanupErr)
				}
			}
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.installation.FinishedAt = model.Now()
	if err != nil {
		m.installation.State = "failed"
		m.installation.Error = err.Error()
		return err
	}
	m.installation.State, m.installation.Stage = "succeeded", "complete"
	m.installation.Path, m.installation.Installed, m.installation.Error = result.Binary, true, ""
	m.status.Version = version
	return nil
}

func probeXrayVersion(ctx context.Context, binary string) (string, error) {
	cmd := exec.CommandContext(ctx, binary, "version")
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return "", errors.New("Xray 版本检测超时")
	}
	if err != nil {
		return "", errors.New("无法执行 Xray 版本检测")
	}
	fields := strings.Fields(string(out))
	for i, field := range fields {
		if strings.EqualFold(field, "Xray") && i+1 < len(fields) {
			return fields[i+1], nil
		}
	}
	return "", errors.New("无法识别 Xray 版本")
}
