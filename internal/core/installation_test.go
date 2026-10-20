package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uniproxy/uniproxy/internal/coreinstall"
	"github.com/uniproxy/uniproxy/internal/model"
)

func missingInstallManager(t *testing.T) (*Manager, *atomic.Int32, *atomic.Int32) {
	t.Helper()
	m, starts, versions := mockManager(t)
	m.options.BinaryPath = filepath.Join(t.TempDir(), "core", binaryFilename())
	return m, starts, versions
}

func TestCoreInstallationMissingAndExplicitDestination(t *testing.T) {
	m, starts, versions := missingInstallManager(t)
	s := m.InstallationStatus()
	if s.State != "idle" || s.Installed || !s.CanInstall || !s.Supported || s.Path != m.options.BinaryPath || s.Version != PinnedVersion || s.OS != runtime.GOOS || s.Arch != runtime.GOARCH {
		t.Fatalf("unexpected missing core status: %+v", s)
	}
	if starts.Load() != 0 || versions.Load() != 0 {
		t.Fatal("status must not execute a subprocess")
	}
	m.options.BinaryPath = "uniproxy-nonexistent-core-command"
	s = m.InstallationStatus()
	if s.CanInstall || s.Path != "" || !strings.Contains(s.Reason, "PATH") {
		t.Fatalf("bare PATH override became an implicit output file: %+v", s)
	}
	m.options.BinaryPath = filepath.Join("relative-core", binaryFilename())
	s = m.InstallationStatus()
	want, _ := filepath.Abs(m.options.BinaryPath)
	if !s.CanInstall || s.Path != want {
		t.Fatalf("explicit relative file not resolved: %+v", s)
	}
}

func TestCoreInstallationDiscoveryOrderAndNoInvalidFallback(t *testing.T) {
	// Every candidate is a synthetic, never-executed file under this temp tree.
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("PATH", filepath.Join(dir, "path-bin"))
	for _, folder := range []string{"work", "bin", "path-bin"} {
		if err := os.MkdirAll(filepath.Join(dir, folder), 0700); err != nil {
			t.Fatal(err)
		}
	}
	dataPath := filepath.Join(dir, "work", binaryFilename())
	legacyPath := filepath.Join(dir, "bin", binaryFilename())
	pathPath := filepath.Join(dir, "path-bin", binaryFilename())
	for _, path := range []string{dataPath, legacyPath, pathPath} {
		if err := os.WriteFile(path, []byte("discovery fixture, never executed"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	m := New(Options{DataDir: filepath.Join(dir, "work")})
	for _, want := range []string{dataPath, legacyPath, pathPath} {
		got, exists, err := m.discoverBinary()
		if err != nil || !exists || got != want {
			t.Fatalf("discovery got %q exists=%v err=%v; want %q", got, exists, err, want)
		}
		if s := m.InstallationStatus(); !s.Installed || s.CanInstall || s.Path != want {
			t.Fatalf("existing file replaceable: %+v", s)
		}
		if err = os.Remove(want); err != nil {
			t.Fatal(err)
		}
	}
	if s := m.InstallationStatus(); s.Installed || !s.CanInstall || s.Path != dataPath {
		t.Fatalf("missing standard destination: %+v", s)
	}
	// A wrong-version/invalid first candidate must not silently fall through to
	// another binary and change which core is used.
	if err := os.Mkdir(dataPath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPath, []byte("other fixture"), 0700); err != nil {
		t.Fatal(err)
	}
	got, exists, err := m.discoverBinary()
	if err != nil || !exists || got != dataPath {
		t.Fatalf("invalid primary candidate skipped: %q %v %v", got, exists, err)
	}
	if s := m.InstallationStatus(); s.CanInstall || !s.Installed {
		t.Fatalf("directory overwrite allowed: %+v", s)
	}
}

func TestCoreInstallationRefusesExistingRunningBusyAndCanceled(t *testing.T) {
	m, _, _ := mockManager(t)
	if _, err := m.PrepareInstall(context.Background()); err == nil {
		t.Fatal("existing core replaceable")
	}
	m, _, _ = missingInstallManager(t)
	m.mu.Lock()
	m.status.Running = true
	m.mu.Unlock()
	if _, err := m.PrepareInstall(context.Background()); err == nil {
		t.Fatal("running core install permitted")
	}
	m.mu.Lock()
	m.status.Running = false
	m.mu.Unlock()
	m.op.Lock()
	if _, err := m.PrepareInstall(context.Background()); !errors.Is(err, ErrCoreBusy) {
		t.Fatalf("busy lifecycle accepted: %v", err)
	}
	m.op.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.PrepareInstall(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled reservation accepted: %v", err)
	}
	if m.installing() {
		t.Fatal("canceled request reserved install")
	}
}

func TestCoreInstallationReservationProgressAndExplicitApply(t *testing.T) {
	m, starts, versions := missingInstallManager(t)
	entered, release := make(chan struct{}), make(chan struct{})
	m.install = func(ctx context.Context, opts coreinstall.Options, report func(coreinstall.Progress)) (string, error) {
		report(coreinstall.Progress{Stage: "downloading", BytesDownloaded: 17, TotalBytes: 100})
		close(entered)
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-release:
		}
		if err := os.MkdirAll(filepath.Dir(opts.Output), 0700); err != nil {
			return "", err
		}
		if err := os.WriteFile(opts.Output, []byte("only the injected helper may execute"), 0700); err != nil {
			return "", err
		}
		return opts.Output, nil
	}
	s, err := m.PrepareInstall(context.Background())
	if err != nil || s.State != "installing" || s.StartedAt == "" {
		t.Fatalf("reserve: %+v %v", s, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- m.RunInstall(ctx) }()
	<-entered
	if s = m.InstallationStatus(); s.Stage != "downloading" || s.BytesDownloaded != 17 || s.TotalBytes != 100 || s.CanInstall {
		t.Fatalf("progress: %+v", s)
	}
	if _, err = m.PrepareInstall(ctx); err == nil {
		t.Fatal("second reservation accepted")
	}
	if err = m.RunInstall(ctx); !errors.Is(err, ErrCoreBusy) {
		t.Fatalf("second worker accepted: %v", err)
	}
	if err = m.Apply(ctx, model.DefaultState()); !errors.Is(err, ErrInstalling) {
		t.Fatalf("apply during install: %v", err)
	}
	if err = m.Validate(ctx, model.DefaultState()); !errors.Is(err, ErrInstalling) {
		t.Fatalf("validate during install: %v", err)
	}
	_ = m.Inspect(ctx)
	if starts.Load() != 0 || versions.Load() != 0 {
		t.Fatal("install reservation did not exclude version/start subprocess")
	}
	close(release)
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	s = m.InstallationStatus()
	r := m.Status()
	if s.State != "succeeded" || s.Stage != "complete" || !s.Installed || s.CanInstall || s.Error != "" || s.FinishedAt == "" {
		t.Fatalf("finished status: %+v", s)
	}
	if !r.Available || r.Running || !r.Dirty || r.Version != PinnedVersion || starts.Load() != 0 || versions.Load() != 1 {
		t.Fatalf("install misreported running core: %+v starts=%d probes=%d", r, starts.Load(), versions.Load())
	}
	if err = m.Apply(ctx, model.DefaultState()); err != nil {
		t.Fatal("explicit apply failed:", err)
	}
	if !m.Status().Running || starts.Load() != 1 {
		t.Fatal("explicit apply did not start")
	}
}

func TestCoreInstallationFailureRetryAndCancellation(t *testing.T) {
	m, starts, _ := missingInstallManager(t)
	m.install = func(ctx context.Context, opts coreinstall.Options, report func(coreinstall.Progress)) (string, error) {
		report(coreinstall.Progress{Stage: "downloading", BytesDownloaded: 5, TotalBytes: 40})
		return "", errors.New("受控网络失败")
	}
	if _, err := m.PrepareInstall(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := m.RunInstall(context.Background()); err == nil {
		t.Fatal("failed network reported success")
	}
	if s := m.InstallationStatus(); s.State != "failed" || s.Error != "受控网络失败" || !s.CanInstall || s.Installed || s.BytesDownloaded != 5 {
		t.Fatalf("failure: %+v", s)
	}
	s, err := m.PrepareInstall(context.Background())
	if err != nil || s.BytesDownloaded != 0 || s.TotalBytes != 0 || s.Error != "" || s.FinishedAt != "" {
		t.Fatalf("retry failed to reset: %+v %v", s, err)
	}
	m.install = func(ctx context.Context, _ coreinstall.Options, _ func(coreinstall.Progress)) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = m.RunInstall(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	if s = m.InstallationStatus(); s.State != "failed" || !s.CanInstall || starts.Load() != 0 {
		t.Fatalf("canceled install remained busy: %+v", s)
	}
}

func TestCoreInstallationWrongVersionNeverReportsSuccess(t *testing.T) {
	m, starts, _ := missingInstallManager(t)
	t.Setenv("UNIPROXY_MOCK_VERSION", "1.20.0")
	m.install = func(_ context.Context, opts coreinstall.Options, _ func(coreinstall.Progress)) (string, error) {
		if err := os.MkdirAll(filepath.Dir(opts.Output), 0700); err != nil {
			return "", err
		}
		return opts.Output, os.WriteFile(opts.Output, []byte("helper version mismatch"), 0700)
	}
	if _, err := m.PrepareInstall(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := m.RunInstall(context.Background()); !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatalf("wrong version accepted: %v", err)
	}
	s := m.InstallationStatus()
	if s.State != "failed" || !s.Installed || s.CanInstall || m.Status().Available || starts.Load() != 0 {
		t.Fatalf("wrong version install status: %+v", s)
	}
}

func TestCoreInstallationReinspectRecoversWithoutRedownloadOrStart(t *testing.T) {
	m, starts, versions := missingInstallManager(t)
	m.install = func(_ context.Context, opts coreinstall.Options, _ func(coreinstall.Progress)) (string, error) {
		if err := os.MkdirAll(filepath.Dir(opts.Output), 0700); err != nil {
			return "", err
		}
		return opts.Output, os.WriteFile(opts.Output, []byte("mock probe recovery fixture"), 0700)
	}
	if _, err := m.PrepareInstall(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := m.RunInstall(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Model a cold-start timeout after publication, with the verified file kept.
	m.mu.Lock()
	m.installation.State, m.installation.Stage = "failed", "inspecting"
	m.installation.Error = "Mihomo version inspection timed out or was canceled"
	m.status.Available = false
	m.status.LastError = m.installation.Error
	m.mu.Unlock()
	m.binaryVersion = ""
	status := m.Inspect(context.Background())
	install := m.InstallationStatus()
	if !status.Available || status.Running || status.LastError != "" || install.State != "succeeded" || install.Stage != "complete" || install.Error != "" || starts.Load() != 0 || versions.Load() != 2 {
		t.Fatalf("reinspect did not recover safely: runtime=%+v install=%+v", status, install)
	}
}

func TestCoreInstallationManualRepairRetiresFailedDownload(t *testing.T) {
	m, starts, _ := missingInstallManager(t)
	m.install = func(context.Context, coreinstall.Options, func(coreinstall.Progress)) (string, error) {
		return "", errors.New("受控网络失败")
	}
	if _, err := m.PrepareInstall(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := m.RunInstall(context.Background()); err == nil {
		t.Fatal("expected download failure")
	}
	if err := os.MkdirAll(filepath.Dir(m.options.BinaryPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(m.options.BinaryPath, []byte("manual fixture verified by helper"), 0700); err != nil {
		t.Fatal(err)
	}
	r := m.Inspect(context.Background())
	s := m.InstallationStatus()
	if !r.Available || r.Running || r.LastError != "" || s.State != "idle" || !s.Installed || s.CanInstall || s.Error != "" || s.TotalBytes != 0 || starts.Load() != 0 {
		t.Fatalf("manual repair left failed task or claimed download success: runtime=%+v install=%+v", r, s)
	}
}

func TestCoreInstallationLifecycleCallsDoNotWaitForProbe(t *testing.T) {
	m, _, _ := missingInstallManager(t)
	if _, err := m.PrepareInstall(context.Background()); err != nil {
		t.Fatal(err)
	}
	// A slow post-install version process holds op. Read-only Inspect and
	// rejected Apply/Validate must observe the reservation before taking op.
	m.op.Lock()
	done := make(chan error, 1)
	go func() {
		if err := m.Apply(context.Background(), model.DefaultState()); !errors.Is(err, ErrInstalling) {
			done <- err
			return
		}
		if err := m.Validate(context.Background(), model.DefaultState()); !errors.Is(err, ErrInstalling) {
			done <- err
			return
		}
		_ = m.Inspect(context.Background())
		done <- nil
	}()
	select {
	case err := <-done:
		m.op.Unlock()
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		m.op.Unlock()
		<-done
		t.Fatal("installation lifecycle calls waited for version probe")
	}
}

func TestCoreInstallationConcurrentReservations(t *testing.T) {
	m, _, _ := missingInstallManager(t)
	var success atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := m.PrepareInstall(context.Background()); err == nil {
				success.Add(1)
			}
		}()
	}
	wg.Wait()
	if success.Load() != 1 {
		t.Fatalf("got %d winning reservations", success.Load())
	}
	m.install = func(context.Context, coreinstall.Options, func(coreinstall.Progress)) (string, error) {
		return "", context.Canceled
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = m.RunInstall(ctx)
}
