package coreinstall

import (
	"archive/zip"
	"bufio"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"
)

// extractBinary copies only into the already-open staging file. No archive path
// is ever used as an OS path. Its private limit permits small deterministic
// expansion-limit tests; production always supplies maxBinaryBytes.
func extractBinary(ctx context.Context, archive *os.File, size int64, name string, output io.Writer, limit int64) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	if limit <= 0 || limit > maxBinaryBytes || size <= 0 || size > maxArchiveBytes {
		return errArchive
	}
	switch {
	case strings.HasSuffix(name, ".zip"):
		return extractZIP(ctx, archive, size, output, limit)
	case strings.HasSuffix(name, ".gz"):
		return extractGZIP(ctx, archive, size, output, limit)
	default:
		return errArchive
	}
}

func extractZIP(ctx context.Context, archive io.ReaderAt, size int64, output io.Writer, limit int64) error {
	reader, err := zip.NewReader(&contextReaderAt{ctx: ctx, reader: archive}, size)
	if failure := checkContext(ctx); failure != nil {
		return failure
	}
	if err != nil || len(reader.File) > 64 {
		return errArchive
	}
	var entry *zip.File
	seen := make(map[string]bool, len(reader.File))
	for _, file := range reader.File {
		if err := checkContext(ctx); err != nil {
			return err
		}
		// Reject traversal, links/devices, duplicates and all non-flat paths,
		// including unrelated entries. Official ZIP assets are flat archives.
		name := strings.ToLower(file.Name)
		if !safeFlatName(file.Name) || !file.Mode().IsRegular() || file.Flags&1 != 0 || seen[name] {
			return errArchive
		}
		seen[name] = true
		if !strings.HasSuffix(name, ".exe") {
			continue
		}
		if !strings.HasPrefix(file.Name, "mihomo") || entry != nil {
			return errArchive
		}
		if file.UncompressedSize64 == 0 || file.UncompressedSize64 > uint64(limit) {
			return errBinarySize
		}
		entry = file
	}
	if entry == nil {
		return errArchive
	}
	input, err := entry.Open()
	if err != nil {
		if failure := checkContext(ctx); failure != nil {
			return failure
		}
		return errArchive
	}
	defer input.Close()
	return copyBinary(ctx, output, input, limit)
}

func extractGZIP(ctx context.Context, archive io.ReaderAt, size int64, output io.Writer, limit int64) error {
	// ByteReader + Multistream(false) leaves the reader exactly after the first
	// stream, allowing rejection of appended payloads and concatenated members.
	source := bufio.NewReader(&contextReader{ctx: ctx, reader: io.NewSectionReader(archive, 0, size)})
	reader, err := gzip.NewReader(source)
	if err != nil {
		if failure := checkContext(ctx); failure != nil {
			return failure
		}
		return errArchive
	}
	defer reader.Close()
	reader.Multistream(false)
	// Never use a gzip filename for extraction. Still refuse path-like names
	// so a future refactor cannot turn a previously accepted header into a path.
	if reader.Name != "" && !safeFlatName(reader.Name) {
		return errArchive
	}
	if err := copyBinary(ctx, output, reader, limit); err != nil {
		return err
	}
	if _, err := source.ReadByte(); !errors.Is(err, io.EOF) {
		if failure := checkContext(ctx); failure != nil {
			return failure
		}
		return errArchive
	}
	return checkContext(ctx)
}

func safeFlatName(name string) bool {
	if name == "" || !utf8.ValidString(name) || len(name) > 255 || name == "." || name == ".." || strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") || strings.ContainsAny(name, "/\\:") {
		return false
	}
	return strings.IndexFunc(name, unicode.IsControl) < 0
}

func copyBinary(ctx context.Context, output io.Writer, input io.Reader, limit int64) error {
	writer := &binaryWriter{dst: output}
	n, err := io.Copy(writer, &contextReader{ctx: ctx, reader: io.LimitReader(input, limit+1)})
	if failure := checkContext(ctx); failure != nil {
		return failure
	}
	if writer.err != nil {
		return filesystemFailure("写入内核暂存文件", writer.err)
	}
	if err != nil {
		return errArchive // Includes ZIP CRC, gzip trailer and truncated streams.
	}
	if n == 0 || n > limit {
		return errBinarySize
	}
	return nil
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

type binaryWriter struct {
	dst io.Writer
	err error
}

func (w *binaryWriter) Write(p []byte) (int, error) {
	n, err := w.dst.Write(p)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	w.err = err
	return n, err
}
