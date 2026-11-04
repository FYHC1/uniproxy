package sidecarinstall

import (
	"archive/zip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// extract reads only an already verified ZIP archive and writes an allowlist of
// flat runtime files into staging. No archive path is ever used as an OS path.
func extract(ctx context.Context, archive *os.File, size int64, name, goos, staging string) ([]string, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	if !strings.HasSuffix(name, ".zip") || size <= 0 || size > maxArchiveBytes {
		return nil, errArchive
	}
	reader, err := zip.NewReader(&contextReaderAt{ctx: ctx, reader: archive}, size)
	if failure := checkContext(ctx); failure != nil {
		return nil, failure
	}
	if err != nil || len(reader.File) > 64 {
		return nil, errArchive
	}
	allowed, required, order := archiveAllowlist(goos)
	if len(required) == 0 {
		return nil, errArchive
	}
	seen := make(map[string]bool, len(reader.File))
	extracted := make(map[string]bool, len(order))
	var total int64
	for _, file := range reader.File {
		if err := checkContext(ctx); err != nil {
			return nil, err
		}
		lower := strings.ToLower(file.Name)
		if !safeFlatName(file.Name) || !file.Mode().IsRegular() || file.Flags&1 != 0 || seen[lower] {
			return nil, errArchive
		}
		seen[lower] = true
		if !allowed[file.Name] {
			continue
		}
		if file.UncompressedSize64 == 0 || file.UncompressedSize64 > uint64(maxFileBytes) {
			return nil, errArchive
		}
		total += int64(file.UncompressedSize64)
		if total > maxTotalBytes {
			return nil, errArchive
		}
		if err := extractEntry(ctx, file, staging); err != nil {
			return nil, err
		}
		extracted[file.Name] = true
	}
	for _, name := range required {
		if !extracted[name] {
			return nil, errArchive
		}
	}
	files := make([]string, 0, len(order))
	for _, name := range order {
		if extracted[name] {
			files = append(files, name)
		}
	}
	if len(files) == 0 {
		return nil, errArchive
	}
	return files, nil
}

func archiveAllowlist(goos string) (map[string]bool, []string, []string) {
	var required []string
	var order []string
	if goos == "windows" {
		required = []string{"xray.exe", "geoip.dat", "geosite.dat", "wintun.dll"}
		order = []string{"xray.exe", "geoip.dat", "geosite.dat", "wintun.dll", "LICENSE", "LICENSE-Wintun"}
	} else if goos == "linux" {
		required = []string{"xray", "geoip.dat", "geosite.dat"}
		order = []string{"xray", "geoip.dat", "geosite.dat", "LICENSE", "LICENSE-Wintun"}
	}
	allowed := make(map[string]bool, len(order))
	for _, name := range order {
		allowed[name] = true
	}
	return allowed, required, order
}

func extractEntry(ctx context.Context, file *zip.File, staging string) error {
	input, err := file.Open()
	if err != nil {
		if failure := checkContext(ctx); failure != nil {
			return failure
		}
		return errArchive
	}
	defer input.Close()
	path := filepath.Join(staging, file.Name)
	mode := os.FileMode(0600)
	if file.Name == "xray" || file.Name == "xray.exe" {
		mode = 0700
	}
	output, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return filesystemFailure("创建 Xray 暂存文件", err)
	}
	written, copyErr := io.Copy(output, &contextReader{ctx: ctx, reader: io.LimitReader(input, maxFileBytes+1)})
	syncErr := output.Sync()
	closeErr := output.Close()
	if failure := checkContext(ctx); failure != nil {
		return failure
	}
	if copyErr != nil {
		return errArchive // Includes ZIP CRC and truncated streams.
	}
	if written != int64(file.UncompressedSize64) || written == 0 || written > maxFileBytes {
		return errArchive
	}
	if syncErr != nil {
		return filesystemFailure("同步 Xray 暂存文件", syncErr)
	}
	if closeErr != nil {
		return filesystemFailure("关闭 Xray 暂存文件", closeErr)
	}
	return nil
}

func safeFlatName(name string) bool {
	if name == "" || !utf8.ValidString(name) || len(name) > 255 || name == "." || name == ".." || strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") || strings.ContainsAny(name, "/\\:") {
		return false
	}
	return strings.IndexFunc(name, unicode.IsControl) < 0
}

// ZIP reads its central directory through ReaderAt before it opens an entry.
// Carry the same cancellation/deadline into those reads as the body copy.
type contextReaderAt struct {
	ctx    context.Context
	reader io.ReaderAt
}

func (r *contextReaderAt) ReadAt(p []byte, offset int64) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.reader.ReadAt(p, offset)
	if ctxErr := r.ctx.Err(); ctxErr != nil {
		return n, ctxErr
	}
	return n, err
}
