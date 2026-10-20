//go:build windows

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
	"unsafe"

	"github.com/uniproxy/uniproxy/internal/store"
	"golang.org/x/sys/windows"
)

// Unlike the original smoke test, this never API-stops the core before killing
// its owner. The next launch uses the same persisted synthetic configuration.
func TestBuiltWindowsAbruptExitAndRestart(t *testing.T) {
	appPath, corePath := os.Getenv("UNIPROXY_TEST_APP_PATH"), os.Getenv("UNIPROXY_TEST_CORE_PATH")
	if appPath == "" || corePath == "" {
		t.Skip("set UNIPROXY_TEST_APP_PATH and UNIPROXY_TEST_CORE_PATH")
	}
	root := t.TempDir()
	dir := filepath.Join(root, "data")
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	state := s.Snapshot()
	state.Revision = 61
	if err := s.Save(state); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	base := "http://127.0.0.1:" + strconv.Itoa(port)
	for round := 0; round < 3; round++ {
		t.Run(strconv.Itoa(round), func(t *testing.T) {
			parent := startAppInTestJob(t, appPath, []string{"-host", "127.0.0.1", "-port", strconv.Itoa(port), "-data", dir, "-core", corePath}, root)
			client := &http.Client{Timeout: time.Second, Transport: &http.Transport{Proxy: nil}}
			defer client.CloseIdleConnections()
			ready := false
			for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); {
				if status, err := windows.WaitForSingleObject(parent.Process, 0); err != nil || status != uint32(windows.WAIT_TIMEOUT) {
					t.Fatal("service exited before readiness")
				}
				res, err := client.Get(base + "/api/bootstrap")
				if err == nil {
					var boot struct {
						Revision int64
						Runtime  struct{ Running, Dirty bool }
					}
					decodeErr := json.NewDecoder(res.Body).Decode(&boot)
					res.Body.Close()
					if decodeErr == nil && boot.Revision == 61 && boot.Runtime.Running && !boot.Runtime.Dirty {
						ready = true
						break
					}
				}
				time.Sleep(50 * time.Millisecond)
			}
			if !ready {
				t.Fatal("service/core did not become ready with preserved revision")
			}
			children := ownedChildren(t, parent.Process, parent.ProcessId, corePath)
			if len(children) != 1 {
				t.Fatalf("expected one core child, got %d", len(children))
			}
			if err := windows.TerminateProcess(parent.Process, 73); err != nil {
				t.Fatal(err)
			}
			if status, err := windows.WaitForSingleObject(parent.Process, 5000); err != nil || status != windows.WAIT_OBJECT_0 {
				t.Fatal("owned parent did not exit")
			}
			for _, child := range children {
				state, err := windows.WaitForSingleObject(child, 5000)
				if err != nil || state != windows.WAIT_OBJECT_0 {
					t.Fatalf("core survived parent: wait=%d err=%v", state, err)
				}
			}
			// Read the database through the product store only after the entire
			// owned process tree exited; never delete WAL/SHM to make it pass.
			reopened, err := store.Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			got := reopened.Snapshot()
			reopened.Close()
			if got.Revision != 61 {
				t.Fatal("restart lost synthetic configuration")
			}
		})
	}
}

func ownedChildren(t *testing.T, parentHandle windows.Handle, parent uint32, corePath string) []windows.Handle {
	t.Helper()
	var err error
	var parentCreated, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(parentHandle, &parentCreated, &exited, &kernel, &user); err != nil {
		t.Fatal(err)
	}
	if state, err := windows.WaitForSingleObject(parentHandle, 0); err != nil || state != uint32(windows.WAIT_TIMEOUT) {
		t.Fatal("owned parent exited before child identification")
	}
	corePath, err = filepath.EvalSymlinks(corePath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(snapshot)
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	var result []windows.Handle
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		if entry.ParentProcessID != parent {
			continue
		}
		h, openErr := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_INFORMATION, false, entry.ProcessID)
		if openErr != nil {
			t.Fatal(openErr)
		}
		// A snapshot ParentProcessID can refer to an old PID generation. Query
		// the OPEN HANDLE's parent, creation time and image before any cleanup
		// can terminate it; this also rejects reuse between snapshot and open.
		var info windows.PROCESS_BASIC_INFORMATION
		var returned uint32
		queryErr := windows.NtQueryInformationProcess(h, windows.ProcessBasicInformation, unsafe.Pointer(&info), uint32(unsafe.Sizeof(info)), &returned)
		var childCreated windows.Filetime
		timeErr := windows.GetProcessTimes(h, &childCreated, &exited, &kernel, &user)
		var image [32768]uint16
		size := uint32(len(image))
		imageErr := windows.QueryFullProcessImageName(h, 0, &image[0], &size)
		if imageErr != nil || size > uint32(len(image)) {
			windows.CloseHandle(h)
			continue
		}
		imagePath, pathErr := filepath.EvalSymlinks(windows.UTF16ToString(image[:size]))
		parentState, parentErr := windows.WaitForSingleObject(parentHandle, 0)
		if queryErr != nil || timeErr != nil || imageErr != nil || pathErr != nil || parentErr != nil ||
			info.InheritedFromUniqueProcessId != uintptr(parent) || childCreated.Nanoseconds() < parentCreated.Nanoseconds() ||
			!strings.EqualFold(imagePath, corePath) || parentState != uint32(windows.WAIT_TIMEOUT) {
			windows.CloseHandle(h)
			continue // unproved ownership: never register termination cleanup
		}
		// Retaining the validated handle prevents any later PID reuse.

		t.Cleanup(func() { _ = windows.CloseHandle(h) }) // observation only; outer test job owns all cleanup
		result = append(result, h)
	}
	return result
}

// Retain this test-owned outer job throughout all assertions. If the production
// guard regresses, a readiness/assertion failure still cannot leak a core; the
// outer job is only closed AFTER assertions and cannot make them pass falsely.
func startAppInTestJob(t *testing.T, path string, args []string, dir string) windows.ProcessInformation {
	t.Helper()
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		windows.CloseHandle(job)
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = windows.CloseHandle(job) })
	application, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	command, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(append([]string{path}, args...)))
	if err != nil {
		t.Fatal(err)
	}
	cwd, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		t.Fatal(err)
	}
	var env []string
	for _, entry := range os.Environ() {
		upper := strings.ToUpper(entry)
		if !strings.HasPrefix(upper, "UNIPROXY_") && !strings.HasPrefix(upper, "LOCALAPPDATA=") {
			env = append(env, entry)
		}
	}
	env = append(env, "LOCALAPPDATA="+dir)
	block := utf16.Encode([]rune(strings.Join(env, "\x00") + "\x00\x00"))
	var startup windows.StartupInfo
	startup.Cb = uint32(unsafe.Sizeof(startup))
	var process windows.ProcessInformation
	if err := windows.CreateProcess(application, command, nil, nil, false, windows.CREATE_SUSPENDED|windows.CREATE_NO_WINDOW|windows.CREATE_UNICODE_ENVIRONMENT, &block[0], cwd, &startup, &process); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// Direct handle from CreateProcess, never a reopened snapshot PID.
		if status, _ := windows.WaitForSingleObject(process.Process, 0); status == uint32(windows.WAIT_TIMEOUT) {
			_ = windows.TerminateProcess(process.Process, 74)
			_, _ = windows.WaitForSingleObject(process.Process, 5000)
		}
		_ = windows.CloseHandle(process.Process)
	})
	defer windows.CloseHandle(process.Thread)
	if err := windows.AssignProcessToJobObject(job, process.Process); err != nil {
		t.Fatal(err)
	}
	if _, err := windows.ResumeThread(process.Thread); err != nil {
		t.Fatal(err)
	}
	return process
}

func TestBuiltWindowsStartupFailureWritesIsolatedReport(t *testing.T) {
	path := os.Getenv("UNIPROXY_TEST_APP_PATH")
	if path == "" {
		t.Skip("set UNIPROXY_TEST_APP_PATH")
	}
	root := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "-host", "0.0.0.0", "-data", filepath.Join(root, "data"), "-core", filepath.Join(root, "absent-core"))
	cmd.Dir = root
	cmd.WaitDelay = time.Second
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(entry), "UNIPROXY_") && !strings.HasPrefix(strings.ToUpper(entry), "LOCALAPPDATA=") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "LOCALAPPDATA="+root)
	cmd.Stdin = strings.NewReader("")
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err == nil {
		t.Fatal("invalid public launch succeeded")
	}
	if ctx.Err() != nil {
		t.Fatal("scripted startup blocked waiting for console input")
	}
	files, err := filepath.Glob(filepath.Join(root, "UniProxy", "logs", "startup-*.log"))
	if err != nil || len(files) != 1 {
		t.Fatalf("report missing: %v (%d)", err, len(files))
	}
	file, err := os.Open(files[0])
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(io.LimitReader(file, 20<<10))
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("requires UNIPROXY_ADMIN_TOKEN")) || !bytes.Contains(data, []byte("UniProxy")) {
		t.Fatal("report lacks actionable error/version")
	}
}

func TestBuiltWindowsHealthcheckFailuresDoNotAccumulateLogs(t *testing.T) {
	path := os.Getenv("UNIPROXY_TEST_APP_PATH")
	if path == "" {
		t.Skip("set UNIPROXY_TEST_APP_PATH")
	}
	root := t.TempDir()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		cmd := exec.CommandContext(ctx, path, "-healthcheck", "-port", strconv.Itoa(port))
		cmd.Dir = root
		for _, entry := range os.Environ() {
			upper := strings.ToUpper(entry)
			if !strings.HasPrefix(upper, "UNIPROXY_") && !strings.HasPrefix(upper, "LOCALAPPDATA=") {
				cmd.Env = append(cmd.Env, entry)
			}
		}
		cmd.Env = append(cmd.Env, "LOCALAPPDATA="+root)
		var output bytes.Buffer
		cmd.Stdout = &output
		cmd.Stderr = &output
		err := cmd.Run()
		timedOut := ctx.Err() != nil
		cancel()
		if err == nil || timedOut || output.Len() == 0 {
			t.Fatalf("failed probe behavior: err=%v timeout=%v", err, timedOut)
		}
	}
	files, err := filepath.Glob(filepath.Join(root, "UniProxy", "logs", "startup-*.log"))
	if err != nil || len(files) != 0 {
		t.Fatal("repeated health probes created durable startup reports")
	}
}
