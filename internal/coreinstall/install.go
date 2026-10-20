// Package coreinstall installs the fixed, verified Mihomo release without running
// it. It never replaces an existing destination or changes any proxy settings.
package coreinstall

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

// PinnedVersion is the only Mihomo release this installer accepts.
const PinnedVersion = "1.19.32"

const (
	maxArchiveBytes int64 = 128 << 20
	maxBinaryBytes  int64 = 256 << 20
	installTimeout        = 5 * time.Minute
)

// Options selects a supported target and an explicit binary destination. Empty
// OS and Arch use runtime.GOOS and runtime.GOARCH; Output must not be empty.
type Options struct {
	OS, Arch, Output string
}

// Progress describes real installation work. Stage is metadata, downloading,
// verifying, extracting, or installing. TotalBytes is zero until metadata is
// validated; both byte counts refer to the compressed archive, not the binary.
type Progress struct {
	Stage                       string
	BytesDownloaded, TotalBytes int64
}

// Supported reports whether the exact target has a built-in pinned digest.
func Supported(goos, arch string) bool {
	return (goos == "windows" || goos == "linux") && (arch == "amd64" || arch == "arm64")
}

// Install verifies and installs Mihomo and returns the absolute destination.
// It has a five-minute overall deadline, honors earlier caller cancellation,
// and never executes the binary. Publication is an atomic, no-replace hard link
// from a closed staging file in the same directory. Filesystems without hard
// links fail safely; there is no overwrite/rename fallback.
//
// A non-nil progress callback is called serially on the calling goroutine and
// must return promptly. Completion is represented by this function returning,
// not by a separate progress stage. Errors are safe Chinese user-facing text.
func Install(ctx context.Context, opts Options, progress func(Progress)) (string, error) {
	return install(ctx, opts, progress, dependencies{})
}

// Test seams are private, per-call, and never global mutable overrides. The
// production entry point always uses the safe transport, built-in pins and
// os.Link. In particular no caller can choose a version, URL, proxy, or force.
type dependencies struct {
	transport http.RoundTripper
	pin       func(string) ([sha256.Size]byte, bool)
	link      func(string, string) error
}

func install(ctx context.Context, opts Options, progress func(Progress), deps dependencies) (string, error) {
	if ctx == nil {
		return "", errors.New("内核安装上下文无效；请重新发起安装")
	}
	ctx, cancel := context.WithTimeout(ctx, installTimeout)
	defer cancel()
	if err := checkContext(ctx); err != nil {
		return "", err
	}
	if opts.OS == "" {
		opts.OS = runtime.GOOS
	}
	if opts.Arch == "" {
		opts.Arch = runtime.GOARCH
	}
	name, platform, err := assetName(opts.OS, opts.Arch)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(opts.Output) == "" || strings.ContainsRune(opts.Output, 0) {
		return "", errors.New("未指定有效的内核安装路径；请先设置内核文件路径")
	}
	destination, err := filepath.Abs(opts.Output)
	if err != nil {
		return "", errors.New("内核安装路径无效；请检查路径设置后重试")
	}
	if info, statErr := os.Lstat(destination); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return "", &installError{text: "内核目标路径是符号链接，已拒绝安装；请使用不存在的普通文件路径", cause: os.ErrExist}
		}
		return "", destinationExists()
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return "", filesystemFailure("检查内核安装路径", statErr)
	}
	if deps.pin == nil {
		deps.pin = builtinDigest
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
		return "", err
	}
	a, expected, err := findAsset(ctx, client, name, platform, deps.pin)
	if err != nil {
		return "", err
	}
	if err := checkContext(ctx); err != nil {
		return "", err
	}
	directory := filepath.Dir(destination)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return "", filesystemFailure("创建内核安装目录", err)
	}
	archive, err := os.CreateTemp(directory, ".mihomo-archive-*")
	if err != nil {
		return "", filesystemFailure("创建临时下载文件", err)
	}
	defer cleanupTemp(archive)
	if err := report("downloading", 0, a.Size); err != nil {
		return "", err
	}
	response, err := request(ctx, client, a.URL)
	if err != nil {
		return "", err
	}
	// Hash the exact archive bytes. Never rely on automatic HTTP decompression.
	if response.ContentLength >= 0 && response.ContentLength != a.Size {
		response.Body.Close()
		return "", errDownloadSize
	}
	if encoding := strings.TrimSpace(response.Header.Get("Content-Encoding")); encoding != "" && !strings.EqualFold(encoding, "identity") {
		response.Body.Close()
		return "", errors.New("GitHub 返回了非原始安装包编码，已拒绝安装；请检查网络拦截后重试")
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
	// Size is already bounded by maxArchiveBytes. The extra byte detects both
	// overlong chunked responses and a body larger than the advertised asset.
	n, copyErr := io.Copy(writer, &contextReader{ctx: ctx, reader: io.LimitReader(response.Body, a.Size+1)})
	closeErr := response.Body.Close()
	if err := checkContext(ctx); err != nil {
		return "", err
	}
	if writer.writeErr != nil {
		return "", filesystemFailure("写入临时下载文件", writer.writeErr)
	}
	if n > a.Size || n > maxArchiveBytes {
		return "", errDownloadSize
	}
	if copyErr != nil {
		return "", networkFailure(ctx, copyErr)
	}
	if closeErr != nil {
		return "", networkFailure(ctx, closeErr)
	}
	if n != a.Size {
		return "", errDownloadSize
	}
	if err := report("verifying", n, a.Size); err != nil {
		return "", err
	}
	// Compare to the built-in digest, not merely to server-provided metadata.
	if subtle.ConstantTimeCompare(hash.Sum(nil), expected[:]) != 1 {
		return "", errChecksum
	}
	if err := archive.Sync(); err != nil {
		return "", filesystemFailure("同步已校验安装包", err)
	}
	if err := report("extracting", n, a.Size); err != nil {
		return "", err
	}
	binary, err := os.CreateTemp(directory, ".mihomo-install-*")
	if err != nil {
		return "", filesystemFailure("创建内核暂存文件", err)
	}
	defer cleanupTemp(binary)
	if err := extractBinary(ctx, archive, n, name, binary, maxBinaryBytes); err != nil {
		return "", err
	}
	if err := archive.Close(); err != nil {
		return "", filesystemFailure("关闭安装包", err)
	}
	if err := binary.Chmod(0700); err != nil {
		return "", filesystemFailure("设置内核文件权限", err)
	}
	if err := binary.Sync(); err != nil {
		return "", filesystemFailure("同步内核暂存文件", err)
	}
	if err := binary.Close(); err != nil {
		return "", filesystemFailure("关闭内核暂存文件", err)
	}
	if err := report("installing", n, a.Size); err != nil {
		return "", err
	}
	// Lstat+Rename is not no-replace on Unix: a competitor can create the
	// destination between those calls. Link itself provides the exclusion,
	// including for a dangling symlink or a simultaneously created file.
	if err := deps.link(binary.Name(), destination); err != nil {
		return "", publicationFailure(err)
	}
	// Publication is the commit point. Do not remove the destination or report
	// cancellation after it succeeds. Deferred cleanup unlinks only our staging
	// names, always after closing their handles (also on Windows).
	return destination, nil
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
