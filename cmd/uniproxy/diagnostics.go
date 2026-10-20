package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const startupReportLimit = 16 << 10

// Only this executable's safe lifecycle messages enter the buffer. Core output,
// configuration payloads, environment variables and command-line arguments do not.
type diagnosticBuffer struct {
	mu   sync.Mutex
	data []byte
}

func (b *diagnosticBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	if len(p) >= startupReportLimit {
		b.data = append(b.data[:0], p[len(p)-startupReportLimit:]...)
	} else {
		b.data = append(b.data, p...)
		if len(b.data) > startupReportLimit {
			b.data = append(b.data[:0], b.data[len(b.data)-startupReportLimit:]...)
		}
	}
	return n, nil
}

func (b *diagnosticBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.data...)
}

func saveStartupReport(data []byte) (string, error) {
	var dirs []string
	if cache, err := os.UserCacheDir(); err == nil && cache != "" {
		dirs = append(dirs, filepath.Join(cache, "UniProxy", "logs"))
	}
	dirs = append(dirs, filepath.Join(os.TempDir(), "UniProxy-logs"))
	return writeStartupReport(data, dirs)
}

func writeStartupReport(data []byte, dirs []string) (string, error) {
	if len(data) > startupReportLimit {
		data = data[len(data)-startupReportLimit:]
	}
	var lastErr error
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0700); err != nil {
			lastErr = err
			continue
		}
		// Unique O_EXCL files avoid overwriting either another failure report or
		// a pre-planted file/link. Nothing is written alongside the executable,
		// and reports still work when the selected database directory is broken.
		f, err := os.CreateTemp(dir, "startup-"+time.Now().Format("20060102-150405")+"-*.log")
		if err != nil {
			lastErr = err
			continue
		}
		path := f.Name()
		_, writeErr := f.Write(data)
		syncErr := f.Sync()
		closeErr := f.Close()
		if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
			lastErr = err
			continue
		}
		return path, nil
	}
	if lastErr == nil {
		lastErr = errors.New("no diagnostic directory available")
	}
	return "", lastErr
}

// The caller must establish this is an Explorer-style interactive launch. A
// redirected stdin, healthcheck, shell command or service must never wait here.
func waitForAcknowledgement(ctx context.Context, input io.Reader, output io.Writer) {
	if ctx.Err() != nil {
		return
	}
	fmt.Fprintln(output, "启动失败，服务未运行。请保留上方错误信息；按 Enter 关闭此窗口。")
	done := make(chan struct{})
	go func() {
		_, _ = bufio.NewReader(input).ReadString('\n')
		close(done)
	}()
	select {
	case <-ctx.Done():
	case <-done:
	}
}
