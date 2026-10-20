package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestDiagnosticBufferIsBoundedAndCopied(t *testing.T) {
	var b diagnosticBuffer
	for i := 0; i < 20; i++ {
		data := bytes.Repeat([]byte("x"), 2000)
		n, err := b.Write(data)
		if n != len(data) || err != nil {
			t.Fatal("invalid writer result")
		}
	}
	b.Write([]byte("final error"))
	got := b.Bytes()
	if len(got) != startupReportLimit || !bytes.HasSuffix(got, []byte("final error")) {
		t.Fatal("diagnostic bound lost most recent error")
	}
	got[0] = 'y'
	if b.Bytes()[0] == 'y' {
		t.Fatal("Bytes exposed shared storage")
	}
	b.Write(bytes.Repeat([]byte("z"), startupReportLimit*2))
	if len(b.Bytes()) != startupReportLimit {
		t.Fatal("single large write exceeded bound")
	}
}

func TestStartupReportFallsBackWithoutOverwriting(t *testing.T) {
	root := t.TempDir()
	blocked := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(blocked, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "logs")
	first, err := writeStartupReport([]byte("first"), []string{blocked, dir})
	if err != nil || filepath.Dir(first) != dir {
		t.Fatalf("fallback: %v %s", err, first)
	}
	second, err := writeStartupReport(bytes.Repeat([]byte("s"), startupReportLimit*2), []string{dir})
	if err != nil || first == second {
		t.Fatalf("report overwritten: %v", err)
	}
	data, err := os.ReadFile(first)
	if err != nil || string(data) != "first" {
		t.Fatal("first report changed")
	}
	info, err := os.Stat(second)
	if err != nil || info.Size() != startupReportLimit {
		t.Fatal("report unbounded")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		t.Fatal("report permissions are public")
	}
	if _, err := writeStartupReport(nil, []string{blocked}); err == nil {
		t.Fatal("unwritable logs reported success")
	}
	if _, err := writeStartupReport(nil, nil); err == nil {
		t.Fatal("missing log location reported success")
	}
}

func TestAcknowledgementStopsOnEnterEOFOrCancellation(t *testing.T) {
	for _, value := range []string{"\n", ""} {
		var output bytes.Buffer
		waitForAcknowledgement(context.Background(), strings.NewReader(value), &output)
		if !strings.Contains(output.String(), "Enter") {
			t.Fatal("prompt missing")
		}
	}
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		waitForAcknowledgement(ctx, reader, io.Discard)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("close signal blocked by console error prompt")
	}
}
