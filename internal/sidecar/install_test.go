package sidecar

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/uniproxy/uniproxy/internal/sidecarinstall"
)

func TestXrayInstallationStatusAndReservation(t *testing.T) {
	workDir := t.TempDir()
	manager, err := NewXrayManager(XrayOptions{WorkDir: workDir, BinaryPath: filepath.Join(workDir, "missing", "xray.exe")})
	if err != nil {
		t.Fatal(err)
	}
	status := manager.InstallationStatus()
	if status.State != "idle" || status.Version != sidecarinstall.PinnedVersion || status.Installed {
		t.Fatalf("unexpected initial installation status: %#v", status)
	}
	if !status.CanInstall || status.Reason != "" {
		t.Fatalf("missing binary should be installable: %#v", status)
	}
	reserved, err := manager.PrepareInstall(context.Background())
	if err != nil {
		t.Fatalf("prepare install: %v", err)
	}
	if reserved.State != "installing" || reserved.Stage != "metadata" || reserved.Path == "" {
		t.Fatalf("unexpected reservation: %#v", reserved)
	}
	installing := manager.InstallationStatus()
	if installing.CanInstall || installing.State != "installing" {
		t.Fatalf("installing status must block another install: %#v", installing)
	}
	if _, err := manager.Apply(context.Background(), nil); !errors.Is(err, ErrSidecarInstalling) {
		t.Fatalf("apply during install = %v, want ErrSidecarInstalling", err)
	}
}

func TestXrayManagerRunsReservedInstallAndVerifiesVersion(t *testing.T) {
	workDir := t.TempDir()
	manager, err := NewXrayManager(XrayOptions{WorkDir: workDir})
	if err != nil {
		t.Fatal(err)
	}
	manager.installer = func(_ context.Context, opts sidecarinstall.Options, progress func(sidecarinstall.Progress)) (sidecarinstall.Result, error) {
		progress(sidecarinstall.Progress{Stage: "downloading", BytesDownloaded: 10, TotalBytes: 100})
		if err := os.MkdirAll(opts.Dir, 0o700); err != nil {
			return sidecarinstall.Result{}, err
		}
		binary := filepath.Join(opts.Dir, xrayBinaryName())
		if err := os.WriteFile(binary, []byte("fixture"), 0o700); err != nil {
			return sidecarinstall.Result{}, err
		}
		return sidecarinstall.Result{Dir: opts.Dir, Binary: binary}, nil
	}
	manager.probeVersion = func(_ context.Context, binary string) (string, error) {
		if _, err := os.Stat(binary); err != nil {
			return "", err
		}
		return sidecarinstall.PinnedVersion, nil
	}
	if _, err := manager.PrepareInstall(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := manager.RunInstall(context.Background()); err != nil {
		t.Fatalf("run install: %v", err)
	}
	status := manager.InstallationStatus()
	if status.State != "succeeded" || status.Stage != "complete" || !status.Installed || status.Error != "" {
		t.Fatalf("unexpected success status: %#v", status)
	}
	if status.Path == "" {
		t.Fatal("successful install did not record binary path")
	}
	executable, err := manager.executable()
	if err != nil || executable != status.Path {
		t.Fatalf("executable = %q, %v; want %q", executable, err, status.Path)
	}
	if err := manager.RunInstall(context.Background()); !errors.Is(err, ErrSidecarBusy) {
		t.Fatalf("second run install = %v, want ErrSidecarBusy", err)
	}
}

func TestXrayManagerInstallFailureIsRetryable(t *testing.T) {
	workDir := t.TempDir()
	manager, err := NewXrayManager(XrayOptions{WorkDir: workDir, BinaryPath: filepath.Join(workDir, "missing", "xray.exe")})
	if err != nil {
		t.Fatal(err)
	}
	manager.installer = func(context.Context, sidecarinstall.Options, func(sidecarinstall.Progress)) (sidecarinstall.Result, error) {
		return sidecarinstall.Result{}, errors.New("下载失败")
	}
	if _, err := manager.PrepareInstall(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := manager.RunInstall(context.Background()); err == nil {
		t.Fatal("failed installer returned nil")
	}
	status := manager.InstallationStatus()
	if status.State != "failed" || status.Error == "" || !status.CanInstall {
		t.Fatalf("failed install should be retryable: %#v", status)
	}
}

func TestXrayManagerVersionProbeFailureIsRetryable(t *testing.T) {
	workDir := t.TempDir()
	manager, err := NewXrayManager(XrayOptions{WorkDir: workDir, BinaryPath: filepath.Join(workDir, "missing", "xray.exe")})
	if err != nil {
		t.Fatal(err)
	}
	manager.installer = func(_ context.Context, opts sidecarinstall.Options, _ func(sidecarinstall.Progress)) (sidecarinstall.Result, error) {
		if err := os.MkdirAll(opts.Dir, 0o700); err != nil {
			return sidecarinstall.Result{}, err
		}
		binary := filepath.Join(opts.Dir, xrayBinaryName())
		if err := os.WriteFile(binary, []byte("fixture"), 0o700); err != nil {
			return sidecarinstall.Result{}, err
		}
		return sidecarinstall.Result{Dir: opts.Dir, Binary: binary}, nil
	}
	manager.probeVersion = func(context.Context, string) (string, error) {
		return "0.0.0", nil
	}
	if _, err := manager.PrepareInstall(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := manager.RunInstall(context.Background()); err == nil {
		t.Fatal("version mismatch returned nil")
	}
	destination, _, err := manager.installDestination()
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Lstat(destination); !os.IsNotExist(statErr) {
		t.Fatalf("failed installation directory still exists: %v", statErr)
	}
	status := manager.InstallationStatus()
	if status.State != "failed" || !status.CanInstall {
		t.Fatalf("version probe failure should be retryable: %#v", status)
	}
}

func TestXrayInstallationStatusExternalPathAllowsManagedInstall(t *testing.T) {
	externalDir := t.TempDir()
	binary := filepath.Join(externalDir, xrayBinaryName())
	if err := os.WriteFile(binary, []byte("fixture"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", externalDir)
	manager, err := NewXrayManager(XrayOptions{WorkDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	status := manager.InstallationStatus()
	if !status.Installed || status.Version != "未知（外部路径）" {
		t.Fatalf("external binary should be reported without a pinned version: %#v", status)
	}
	if !status.CanInstall {
		t.Fatalf("external PATH binary should not block managed install: %#v", status)
	}
	if status.Path != binary {
		t.Fatalf("external path = %q, want %q", status.Path, binary)
	}
}

func TestXrayManagerRequiresConfiguredWorkDirForInstall(t *testing.T) {
	manager, err := NewXrayManager(XrayOptions{BinaryPath: filepath.Join(t.TempDir(), "missing", "xray.exe")})
	if err != nil {
		t.Fatal(err)
	}
	status := manager.InstallationStatus()
	if status.CanInstall || status.Reason == "" {
		t.Fatalf("install without workdir must be unavailable: %#v", status)
	}
	if _, err := manager.PrepareInstall(context.Background()); err == nil {
		t.Fatal("PrepareInstall without workdir returned nil")
	}
}
