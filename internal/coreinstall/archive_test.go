package coreinstall

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func extractFixture(t *testing.T, ctx context.Context, data []byte, name string, limit int64, output io.Writer) error {
	t.Helper()
	path := filepath.Join(t.TempDir(), "archive")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	return extractBinary(ctx, file, int64(len(data)), name, output, limit)
}

func TestZIPExtractionAcceptsOnlySafeSoleMihomoExecutable(t *testing.T) {
	for _, tc := range []struct {
		name    string
		entries []zipEntry
		want    bool
	}{
		{"single", []zipEntry{{name: "mihomo.exe", body: "binary"}}, true},
		{"asset-name", []zipEntry{{name: "mihomo-windows-amd64-compatible.exe", body: "binary"}}, true},
		{"readme", []zipEntry{{name: "README.txt", body: "notice"}, {name: "mihomo.exe", body: "binary"}}, true},
		{"traversal", []zipEntry{{name: "../mihomo.exe", body: "binary"}}, false},
		{"nested", []zipEntry{{name: "nested/mihomo.exe", body: "binary"}}, false},
		{"backslash", []zipEntry{{name: "nested\\mihomo.exe", body: "binary"}}, false},
		{"absolute", []zipEntry{{name: "/mihomo.exe", body: "binary"}}, false},
		{"drive", []zipEntry{{name: "C:mihomo.exe", body: "binary"}}, false},
		{"ads", []zipEntry{{name: "mihomo:other.exe", body: "binary"}}, false},
		{"nul", []zipEntry{{name: "mihomo\x00.exe", body: "binary"}}, false},
		{"control", []zipEntry{{name: "mihomo\n.exe", body: "binary"}}, false},
		{"invalid-utf8", []zipEntry{{name: "mihomo\xff.exe", body: "binary"}}, false},
		{"not-mihomo", []zipEntry{{name: "attacker.exe", body: "binary"}}, false},
		{"none", []zipEntry{{name: "README.txt", body: "notice"}}, false},
		{"two-executables", []zipEntry{{name: "mihomo.exe", body: "binary"}, {name: "mihomo-other.exe", body: "binary"}}, false},
		{"second-nonmihomo", []zipEntry{{name: "mihomo.exe", body: "binary"}, {name: "other.exe", body: "binary"}}, false},
		{"duplicate", []zipEntry{{name: "mihomo.exe", body: "binary"}, {name: "MIHOMO.EXE", body: "binary"}}, false},
		{"symlink", []zipEntry{{name: "mihomo.exe", body: "target", mode: os.ModeSymlink | 0777}}, false},
		{"device", []zipEntry{{name: "mihomo.exe", body: "binary", mode: os.ModeDevice | 0600}}, false},
		{"directory-mode", []zipEntry{{name: "mihomo.exe", mode: os.ModeDir | 0700}}, false},
		{"unsafe-unrelated", []zipEntry{{name: "../README.txt", body: "notice"}, {name: "mihomo.exe", body: "binary"}}, false},
		{"symlink-unrelated", []zipEntry{{name: "README.txt", body: "target", mode: os.ModeSymlink | 0777}, {name: "mihomo.exe", body: "binary"}}, false},
		{"trailing-dot", []zipEntry{{name: "README.", body: "notice"}, {name: "mihomo.exe", body: "binary"}}, false},
		{"trailing-space", []zipEntry{{name: "README ", body: "notice"}, {name: "mihomo.exe", body: "binary"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := zipBytes(t, tc.entries...)
			var output bytes.Buffer
			err := extractFixture(t, context.Background(), data, "archive.zip", 1024, &output)
			if tc.want {
				if err != nil || output.String() != "binary" {
					t.Fatalf("valid ZIP rejected: %q %v", output.String(), err)
				}
			} else if err == nil {
				t.Fatal("unsafe ZIP accepted")
			}
		})
	}
}

func TestZIPExtractionCRCTruncationEncryptionAndEntryBounds(t *testing.T) {
	valid := zipBytes(t, zipEntry{name: "mihomo.exe", body: "binary"})
	for _, bad := range []string{"crc", "truncated", "encrypted", "method", "too-many", "empty", "oversize-header", "oversize-stream"} {
		t.Run(bad, func(t *testing.T) {
			data := append([]byte(nil), valid...)
			limit := int64(64)
			central := bytes.Index(data, []byte{'P', 'K', 1, 2})
			if central < 0 {
				t.Fatal("fixture missing ZIP directory")
			}
			switch bad {
			case "crc":
				data[central+16] ^= 0xff
			case "truncated":
				data = data[:len(data)-8]
			case "encrypted":
				data[central+8] |= 1
			case "method":
				binary.LittleEndian.PutUint16(data[central+10:], 99)
			case "too-many":
				entries := []zipEntry{{name: "mihomo.exe", body: "binary"}}
				for i := 0; i < 64; i++ {
					entries = append(entries, zipEntry{name: strings.Repeat("x", i+1) + ".txt", body: "x"})
				}
				data = zipBytes(t, entries...)
			case "empty":
				data = zipBytes(t, zipEntry{name: "mihomo.exe"})
			case "oversize-header":
				binary.LittleEndian.PutUint32(data[central+24:], uint32(maxBinaryBytes+1))
			case "oversize-stream":
				data = zipBytes(t, zipEntry{name: "mihomo.exe", body: strings.Repeat("x", int(limit)+1)})
			}
			var output bytes.Buffer
			if err := extractFixture(t, context.Background(), data, "archive.zip", limit, &output); err == nil {
				t.Fatal("unsafe ZIP accepted")
			}
		})
	}
}

func TestGZIPExtractionRejectsInvalidCRCTruncationAndAppendedMembers(t *testing.T) {
	valid := gzipBytes(t, []byte("binary"), "mihomo")
	for _, tc := range []struct {
		name string
		data []byte
		want bool
	}{
		{"valid", valid, true},
		{"nameless", gzipBytes(t, []byte("binary"), ""), true},
		{"truncated", valid[:len(valid)-4], false},
		{"plain", []byte("SECRET not gzip"), false},
		{"empty", gzipBytes(t, nil, "mihomo"), false},
		{"appended", append(append([]byte(nil), valid...), []byte("SECRET appended")...), false},
		{"second-member", append(append([]byte(nil), valid...), valid...), false},
		{"parent-header", gzipBytes(t, []byte("binary"), "../mihomo"), false},
		{"backslash-header", gzipBytes(t, []byte("binary"), "nested\\mihomo"), false},
		{"absolute-header", gzipBytes(t, []byte("binary"), "/mihomo"), false},
		{"drive-header", gzipBytes(t, []byte("binary"), "C:mihomo"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			err := extractFixture(t, context.Background(), tc.data, "archive.gz", 1024, &output)
			if tc.want {
				if err != nil || output.String() != "binary" {
					t.Fatalf("valid gzip rejected: %q %v", output.String(), err)
				}
			} else if err == nil {
				t.Fatal("unsafe gzip accepted")
			}
			if err != nil && strings.Contains(err.Error(), "SECRET") {
				t.Fatal("archive details leaked", err)
			}
		})
	}
	crc := append([]byte(nil), valid...)
	crc[len(crc)-8] ^= 1
	if err := extractFixture(t, context.Background(), crc, "archive.gz", 1024, io.Discard); !errors.Is(err, errArchive) {
		t.Fatal("gzip CRC ignored", err)
	}
}

func TestBinaryExpansionLimitAtExactBoundary(t *testing.T) {
	const limit int64 = 1024
	for _, size := range []int{0, int(limit) - 1, int(limit), int(limit) + 1} {
		for _, suffix := range []string{".gz", ".zip"} {
			body := bytes.Repeat([]byte("x"), size)
			data := gzipBytes(t, body, "")
			if suffix == ".zip" {
				data = zipBytes(t, zipEntry{name: "mihomo.exe", body: string(body)})
			}
			var output bytes.Buffer
			err := extractFixture(t, context.Background(), data, "archive"+suffix, limit, &output)
			if size == 0 || size > int(limit) {
				if err == nil {
					t.Fatal("empty/oversize binary accepted", suffix, size)
				}
			} else if err != nil || !bytes.Equal(output.Bytes(), body) {
				t.Fatal("bounded valid binary failed", suffix, size, err)
			}
			if output.Len() > int(limit)+1 {
				t.Fatal("unbounded expansion")
			}
		}
	}
}

type cancelWriter struct {
	cancel context.CancelFunc
	n      int
}

func (w *cancelWriter) Write(p []byte) (int, error) { w.n += len(p); w.cancel(); return len(p), nil }

type errorWriter struct {
	err   error
	short bool
}

func (w errorWriter) Write(p []byte) (int, error) {
	if w.short {
		return len(p) / 2, nil
	}
	return 0, w.err
}

func TestExtractionCancellationAndWriterErrorsAreSafe(t *testing.T) {
	for _, suffix := range []string{".gz", ".zip"} {
		payload := strings.Repeat("fixture", 20000)
		data := gzipBytes(t, []byte(payload), "")
		if suffix == ".zip" {
			data = zipBytes(t, zipEntry{name: "mihomo.exe", body: payload})
		}
		ctx, cancel := context.WithCancel(context.Background())
		writer := &cancelWriter{cancel: cancel}
		err := extractFixture(t, ctx, data, "archive"+suffix, maxBinaryBytes, writer)
		cancel()
		if !errors.Is(err, context.Canceled) || writer.n == 0 || writer.n >= len(payload) {
			t.Fatal("extraction failed to cancel promptly", err, writer.n)
		}
		for _, writer := range []io.Writer{errorWriter{err: &os.PathError{Op: "write", Path: "SECRET-path", Err: os.ErrPermission}}, errorWriter{short: true}} {
			err := extractFixture(t, context.Background(), data, "archive"+suffix, maxBinaryBytes, writer)
			if err == nil || strings.Contains(err.Error(), "SECRET") || !strings.Contains(err.Error(), "权限") {
				t.Fatal("unsafe write error", err)
			}
		}
	}
}

func TestExtractionUnknownFormatAndLimits(t *testing.T) {
	data := gzipBytes(t, []byte("fixture"), "")
	for _, tc := range []struct {
		name  string
		limit int64
	}{{"archive.tar", 100}, {"archive.gz", 0}, {"archive.gz", maxBinaryBytes + 1}} {
		if err := extractFixture(t, context.Background(), data, tc.name, tc.limit, io.Discard); err == nil {
			t.Fatal("unknown format / invalid limits accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := extractFixture(t, ctx, data, "archive.gz", maxBinaryBytes, io.Discard); !errors.Is(err, context.Canceled) {
		t.Fatal("pre-canceled extraction accepted", err)
	}
}
