// Package appdata resolves the persistent data directory using platform
// conventions instead of a working-directory-relative default.
//
// A CWD-relative "./data" is a common source of silent data loss: starting the
// same binary from another directory creates a second empty database and the
// original configuration appears to vanish. The resolver therefore prefers a
// stable per-machine location, and adopts an existing legacy directory when one
// is present so an upgrade never points at an empty store.
package appdata

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Source explains why a directory was chosen. It is reported at startup and in
// the UI-visible log so an operator can tell a migrated install from a fresh one.
type Source string

const (
	// SourceConfigured means the operator set -data or UNIPROXY_DATA_DIR.
	SourceConfigured Source = "configured"
	// SourceStandard means the platform default was used.
	SourceStandard Source = "standard"
	// SourceFallback means the platform default was unusable and a per-user
	// directory was used instead.
	SourceFallback Source = "fallback"
	// SourceLegacy means an existing CWD-relative or executable-relative
	// directory was adopted to avoid orphaning real data.
	SourceLegacy Source = "legacy"
)

// Result is the resolved directory plus an operator-facing explanation.
type Result struct {
	Dir    string
	Source Source
	Note   string
}

// Deps lets tests describe a platform without touching the real filesystem.
type Deps struct {
	GOOS        string
	Getenv      func(string) string
	UserHomeDir func() (string, error)
	Executable  func() (string, error)
	WorkingDir  func() (string, error)
	HasDatabase func(string) bool
	MkdirAll    func(string, os.FileMode) error
	Abs         func(string) (string, error)
}

// Default returns the real process dependencies.
func Default() Deps {
	return Deps{
		GOOS:        runtime.GOOS,
		Getenv:      os.Getenv,
		UserHomeDir: os.UserHomeDir,
		Executable:  os.Executable,
		WorkingDir:  os.Getwd,
		HasDatabase: hasDatabase,
		MkdirAll:    os.MkdirAll,
		Abs:         filepath.Abs,
	}
}

// hasDatabase reports whether dir already holds a non-empty configuration
// database. A zero-length file is a failed creation, not data worth adopting.
func hasDatabase(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "uniproxy.db"))
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}

// Resolve returns the directory to use for the SQLite database, master key and
// core working files.
//
// Precedence:
//  1. explicit flag/environment value (created if missing);
//  2. an existing platform-standard directory;
//  3. an existing legacy directory (CWD, then executable directory);
//  4. the platform-standard directory, falling back per-user if unwritable.
func Resolve(explicit string, d Deps) (Result, error) {
	if explicit != "" {
		dir, err := d.Abs(explicit)
		if err != nil {
			return Result{}, err
		}
		if err := d.MkdirAll(dir, 0o700); err != nil {
			return Result{}, fmt.Errorf("数据目录不可用: %w", err)
		}
		return Result{Dir: dir, Source: SourceConfigured}, nil
	}
	standard := standardDir(d)
	if standard != "" && d.HasDatabase(standard) {
		return Result{Dir: standard, Source: SourceStandard}, nil
	}
	if legacy, origin := legacyDir(d); legacy != "" {
		return Result{
			Dir:    legacy,
			Source: SourceLegacy,
			Note:   fmt.Sprintf("检测到旧版 %s 数据目录并继续使用；建议迁移到 %s 后删除旧目录", origin, displayPath(standard)),
		}, nil
	}
	if standard == "" {
		return Result{}, fmt.Errorf("无法确定平台数据目录，请使用 -data 或 UNIPROXY_DATA_DIR 指定")
	}
	if err := d.MkdirAll(standard, 0o700); err == nil {
		return Result{Dir: standard, Source: SourceStandard}, nil
	}
	// A machine-wide directory can be read-only for an unprivileged account.
	// Falling back per-user keeps the service usable; the note makes the split
	// visible instead of silently creating a second store.
	fallback := userDir(d)
	if fallback == "" {
		return Result{}, fmt.Errorf("数据目录 %s 不可写，且无法确定用户数据目录", standard)
	}
	if err := d.MkdirAll(fallback, 0o700); err != nil {
		return Result{}, fmt.Errorf("数据目录 %s 与用户目录 %s 均不可写: %w", standard, fallback, err)
	}
	return Result{
		Dir:    fallback,
		Source: SourceFallback,
		Note:   fmt.Sprintf("系统数据目录 %s 不可写，已使用用户数据目录", standard),
	}, nil
}

// joinTarget joins path elements with the separator of the target platform,
// not the host's. Without this a Windows-hosted unit test for Linux would emit
// backslashes and a Linux-hosted test for Windows would emit slashes.
func joinTarget(goos string, elem ...string) string {
	separator := "/"
	if goos == "windows" {
		separator = `\`
	}
	return strings.Join(elem, separator)
}

// parentTarget returns the parent of a path using the separators of the target
// platform. filepath.Dir would rewrite the fixture to the host's separators,
// which makes cross-platform unit tests assert the wrong thing.
func parentTarget(value string) string {
	index := strings.LastIndexAny(value, `/\`)
	if index <= 0 {
		return value
	}
	// Preserve a Windows drive root such as C:\ .
	if index == 2 && len(value) > 2 && value[1] == ':' {
		return value[:3]
	}
	return value[:index]
}

// standardDir is the machine-wide location for the platform. Windows uses
// ProgramData (the documented location for service data), Linux follows FHS,
// and macOS uses the Application Support convention.
func standardDir(d Deps) string {
	switch d.GOOS {
	case "windows":
		if base := d.Getenv("ProgramData"); base != "" {
			return joinTarget(d.GOOS, base, "UniProxy")
		}
		if base := d.Getenv("SystemDrive"); base != "" {
			return joinTarget(d.GOOS, base, "ProgramData", "UniProxy")
		}
		return ""
	case "darwin":
		home, err := d.UserHomeDir()
		if err != nil || home == "" {
			return ""
		}
		return joinTarget(d.GOOS, home, "Library", "Application Support", "UniProxy")
	default:
		return "/var/lib/uniproxy"
	}
}

// userDir is the per-user location used when the machine-wide directory is not
// writable, and as the XDG fallback on Linux.
func userDir(d Deps) string {
	switch d.GOOS {
	case "windows":
		if base := d.Getenv("LOCALAPPDATA"); base != "" {
			return joinTarget(d.GOOS, base, "UniProxy")
		}
	case "darwin":
		if home, err := d.UserHomeDir(); err == nil && home != "" {
			return joinTarget(d.GOOS, home, "Library", "Application Support", "UniProxy")
		}
	default:
		if base := d.Getenv("XDG_DATA_HOME"); base != "" {
			return joinTarget(d.GOOS, base, "uniproxy")
		}
		if home, err := d.UserHomeDir(); err == nil && home != "" {
			return joinTarget(d.GOOS, home, ".local", "share", "uniproxy")
		}
	}
	return ""
}

// legacyDir finds a pre-existing CWD-relative or executable-relative "./data".
// The working directory wins because that is how earlier builds resolved it.
func legacyDir(d Deps) (string, string) {
	if cwd, err := d.WorkingDir(); err == nil && cwd != "" {
		candidate := joinTarget(d.GOOS, cwd, "data")
		if d.HasDatabase(candidate) {
			return candidate, "工作目录"
		}
	}
	if exe, err := d.Executable(); err == nil && exe != "" {
		candidate := joinTarget(d.GOOS, parentTarget(exe), "data")
		if d.HasDatabase(candidate) {
			return candidate, "程序目录"
		}
	}
	return "", ""
}

func displayPath(path string) string {
	if path == "" {
		return "平台标准目录"
	}
	return path
}
