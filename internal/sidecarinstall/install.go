// Package sidecarinstall installs the fixed, verified Xray sidecar release
// without executing it. Xray releases are multi-file ZIP archives, so unlike
// the single-binary Mihomo installer this package publishes a new directory
// and never overwrites an existing installation.
package sidecarinstall

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// PinnedVersion is the only Xray release this installer accepts.
const PinnedVersion = "26.10.31"

const (
	maxArchiveBytes int64 = 128 << 20
	maxFileBytes    int64 = 128 << 20
	maxTotalBytes   int64 = 256 << 20
	installTimeout        = 5 * time.Minute
)

// Options selects a supported target and an explicit, non-existing installation
// directory. Empty OS and Arch use runtime.GOOS and runtime.GOARCH.
type Options struct {
	OS, Arch, Dir string
}

// Progress describes real installation work. Stage is metadata, downloading,
// verifying, extracting, or installing. TotalBytes is zero until metadata is
// validated; both byte counts refer to the compressed archive.
type Progress struct {
	Stage                       string
	BytesDownloaded, TotalBytes int64
}

// Result describes a completed installation. Dir is the installed directory and
// Binary is the absolute path of the executable inside it.
type Result struct {
	Dir    string
	Binary string
}

// Supported reports whether the exact target has a built-in pinned digest.
func Supported(goos, arch string) bool {
	return (goos == "windows" || goos == "linux") && (arch == "amd64" || arch == "arm64")
}

// Install verifies and installs Xray and returns the published directory and
// binary path. It has a five-minute overall deadline, honors earlier caller
// cancellation, and never executes the binary. Publication creates a new
// directory with an atomic mkdir and hard-links verified staging files into it;
// there is no overwrite/rename fallback.
//
// A non-nil progress callback is called serially on the calling goroutine and
// must return promptly. Errors are safe Chinese user-facing text.
func Install(ctx context.Context, opts Options, progress func(Progress)) (Result, error) {
	return install(ctx, opts, progress, dependencies{})
}

// Test seams are private, per-call, and never global mutable overrides. The
// production entry point always uses the safe transport, built-in pins,
// os.Mkdir and os.Link. No caller can choose a version, URL, proxy, or force.
type dependencies struct {
	transport http.RoundTripper
	pin       func(string) ([sha256.Size]byte, bool)
	mkdir     func(string, os.FileMode) error
	link      func(string, string) error
}

func install(ctx context.Context, opts Options, progress func(Progress), deps dependencies) (Result, error) {
	if ctx == nil {
		return Result{}, errors.New("Xray 安装上下文无效；请重新发起安装")
	}
	ctx, cancel := context.WithTimeout(ctx, installTimeout)
	defer cancel()
	if err := checkContext(ctx); err != nil {
		return Result{}, err
	}
	if opts.OS == "" {
		opts.OS = runtime.GOOS
	}
	if opts.Arch == "" {
		opts.Arch = runtime.GOARCH
	}
	name, platform, err := assetName(opts.OS, opts.Arch)
	if err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(opts.Dir) == "" || strings.ContainsRune(opts.Dir, 0) {
		return Result{}, errors.New("未指定有效的 Xray 安装目录；请先设置 sidecar 安装目录")
	}
	destination, err := filepath.Abs(opts.Dir)
	if err != nil {
		return Result{}, errors.New("Xray 安装目录无效；请检查路径设置后重试")
	}
	if info, statErr := os.Lstat(destination); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return Result{}, &installError{text: "Xray 安装目录是符号链接，已拒绝安装；请使用不存在的普通目录", cause: os.ErrExist}
		}
		return Result{}, destinationExists()
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return Result{}, filesystemFailure("检查 Xray 安装目录", statErr)
	}
	if deps.pin == nil {
		deps.pin = builtinDigest
	}
	if deps.mkdir == nil {
		deps.mkdir = os.Mkdir
	}
	if deps.link == nil {
		deps.link = os.Link
	}
	client := newDownloadClient(deps.transport)
	defer client.CloseIdleConnections()
	report := func(stage string, downloaded, total int64) error {
		if err := checkContext(ctx); err != nil {
			return err
		}
		if progress != nil {
			progress(Progress{Stage: stage, BytesDownloaded: downloaded, TotalBytes: total})
		}
		return checkContext(ctx)
	}
	if err := report("metadata", 0, 0); err != nil {
		return Result{}, err
	}
	a, expected, err := findAsset(ctx, client, name, platform, deps.pin)
	if err != nil {
		return Result{}, err
	}
	if err := checkContext(ctx); err != nil {
		return Result{}, err
	}
	parent := filepath.Dir(destination)
	if err := os.MkdirAll(parent, 0700); err != nil {
		return Result{}, filesystemFailure("创建 Xray 安装父目录", err)
	}
	archive, err := os.CreateTemp(parent, ".xray-archive-*")
	if err != nil {
		return Result{}, filesystemFailure("创建临时下载文件", err)
	}
	defer cleanupTemp(archive)
	if err := report("downloading", 0, a.Size); err != nil {
		return Result{}, err
	}
	response, err := request(ctx, client, a.URL)
	if err != nil {
		return Result{}, err
	}
	if response.ContentLength >= 0 && response.ContentLength != a.Size {
		response.Body.Close()
		return Result{}, errDownloadSize
	}
	if encoding := strings.TrimSpace(response.Header.Get("Content-Encoding")); encoding != "" && !strings.EqualFold(encoding, "identity") {
		response.Body.Close()
		return Result{}, errors.New("GitHub 返回了非原始 Xray 安装包编码，已拒绝安装；请检查网络拦截后重试")
	}
	hash := sha256.New()
	writer := &downloadWriter{
		dst: io.MultiWriter(archive, hash),
		notify: func(n int64) error {
			if n > a.Size {
				return errDownloadSize
			}
			return report("downloading", n, a.Size)
		},
	}
	n, copyErr := io.Copy(writer, &contextReader{ctx: ctx, reader: io.LimitReader(response.Body, a.Size+1)})
	closeErr := response.Body.Close()
	if err := checkContext(ctx); err != nil {
		return Result{}, err
	}
	if writer.writeErr != nil {
		return Result{}, filesystemFailure("写入临时下载文件", writer.writeErr)
	}
	if n > a.Size || n > maxArchiveBytes {
		return Result{}, errDownloadSize
	}
	if copyErr != nil {
		return Result{}, networkFailure(ctx, copyErr)
	}
	if closeErr != nil {
		return Result{}, networkFailure(ctx, closeErr)
	}
	if n != a.Size {
		return Result{}, errDownloadSize
	}
	if err := report("verifying", n, a.Size); err != nil {
		return Result{}, err
	}
	if subtle.ConstantTimeCompare(hash.Sum(nil), expected[:]) != 1 {
		return Result{}, errChecksum
	}
	if err := archive.Sync(); err != nil {
		return Result{}, filesystemFailure("同步已校验 Xray 安装包", err)
	}
	if err := report("extracting", n, a.Size); err != nil {
		return Result{}, err
	}
	staging, err := os.MkdirTemp(parent, ".xray-install-*")
	if err != nil {
		return Result{}, filesystemFailure("创建 Xray 暂存目录", err)
	}
	defer os.RemoveAll(staging)
	files, err := extract(ctx, archive, n, name, opts.OS, staging)
	if err != nil {
		return Result{}, err
	}
	if err := archive.Close(); err != nil {
		return Result{}, filesystemFailure("关闭 Xray 安装包", err)
	}
	if err := report("installing", n, a.Size); err != nil {
		return Result{}, err
	}
	if err := deps.mkdir(destination, 0700); err != nil {
		return Result{}, publicationFailure(err)
	}
	published := make([]string, 0, len(files))
	for _, file := range files {
		if err := deps.link(filepath.Join(staging, file), filepath.Join(destination, file)); err != nil {
			for _, done := range published {
				_ = os.Remove(filepath.Join(destination, done))
			}
			_ = os.Remove(destination)
			return Result{}, publicationFailure(err)
		}
		published = append(published, file)
	}
	binaryName := "xray"
	if opts.OS == "windows" {
		binaryName = "xray.exe"
	}
	return Result{Dir: destination, Binary: filepath.Join(destination, binaryName)}, nil
}

func cleanupTemp(file *os.File) {
	_ = file.Close()
	_ = os.Remove(file.Name())
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.reader.Read(p)
	if ctxErr := r.ctx.Err(); ctxErr != nil {
		return n, ctxErr
	}
	return n, err
}

type downloadWriter struct {
	dst      io.Writer
	notify   func(int64) error
	n        int64
	writeErr error
}

func (w *downloadWriter) Write(p []byte) (int, error) {
	n, err := w.dst.Write(p)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	if err != nil {
		w.writeErr = err
		return n, err
	}
	w.n += int64(n)
	if err := w.notify(w.n); err != nil {
		return n, err
	}
	return n, nil
}
