//go:build windows

package processguard

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

const helperMarker = "uniproxy-processguard-helper"
const helperWaitMilliseconds = 20000

type helperIdentity struct {
	PID     uint32
	Created windows.Filetime
}

type helperReport struct {
	Processes           []helperIdentity
	InheritedJobHandle  bool
	InheritedEventWorks bool
	GuardHandleFlags    uint32
}

type helperArguments struct {
	role    string
	dir     string
	ready   windows.Handle
	release windows.Handle
	cleanup windows.Handle
	job     windows.Handle
}

// These tests never Enable in the main test runner. Only disposable helpers
// attach themselves to jobs, and every process handle/PID comes from those
// helpers. No system-wide process enumeration or termination is used.
func TestEnableWindowsParentLifetime(t *testing.T) {
	for _, tc := range []struct {
		name   string
		abrupt bool
		nested bool
	}{
		{"abrupt-parent-death", true, false},
		{"normal-parent-exit", false, false},
		{"already-in-existing-job", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			ready := mustEvent(t)
			release := mustEvent(t)
			cleanup := mustEvent(t)
			// Children block on this event for up to two minutes. It remains
			// unsignaled until cleanup, so it cannot satisfy the death assertion.
			// It also rescues owned descendants if setup/assertions fail.
			t.Cleanup(func() { _ = windows.SetEvent(cleanup) })

			args := helperArguments{role: "parent", dir: dir, ready: ready, release: release, cleanup: cleanup}
			parent, err := startNativeHelper(args, windows.CREATE_SUSPENDED)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { cleanupOwnedProcess(t, parent.Process) })
			t.Cleanup(func() { _ = windows.CloseHandle(parent.Thread) })

			var outer windows.Handle
			if tc.nested {
				outer, err = windows.CreateJobObject(nil, nil)
				if err != nil {
					t.Fatalf("create test outer job: %v", err)
				}
				// The runner is NOT assigned. Hold this handle open throughout
				// the assertions: only the inner guard can cause descendant exit.
				t.Cleanup(func() { _ = windows.CloseHandle(outer) })
				var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
				limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
				if err := systemJobAPI.setLimits(outer, &limits); err != nil {
					t.Fatal(err)
				}
				if err := windows.AssignProcessToJobObject(outer, parent.Process); err != nil {
					t.Fatalf("assign owned suspended helper to existing job: %v", err)
				}
			}
			if _, err := windows.ResumeThread(parent.Thread); err != nil {
				t.Fatalf("resume helper: %v", err)
			}
			if err := waitHelperReady(parent.Process, ready); err != nil {
				t.Fatalf("parent startup: %v; %s", err, helperFailure(dir, "parent"))
			}
			report, err := readHelperReport(dir, "parent")
			if err != nil {
				t.Fatal(err)
			}
			if len(report.Processes) != 2 {
				t.Fatalf("expected child and grandchild, got %+v", report.Processes)
			}
			if report.InheritedJobHandle || report.GuardHandleFlags&windows.HANDLE_FLAG_INHERIT != 0 {
				t.Fatal("a descendant could inherit and retain the guard job handle")
			}
			if !report.InheritedEventWorks {
				t.Fatal("inherit-all CreateProcess did not actually inherit the canary event")
			}

			descendants := make([]windows.Handle, 0, len(report.Processes))
			for _, identity := range report.Processes {
				h := openOwnedDescendant(t, identity)
				descendants = append(descendants, h)
				if state, err := windows.WaitForSingleObject(h, 100); err != nil || state != uint32(windows.WAIT_TIMEOUT) {
					t.Fatalf("descendant %d was not long-running before parent exit: state=%d err=%v", identity.PID, state, err)
				}
			}
			if state, err := windows.WaitForSingleObject(parent.Process, 0); err != nil || state != uint32(windows.WAIT_TIMEOUT) {
				t.Fatalf("parent exited before test action: state=%d err=%v", state, err)
			}
			if tc.abrupt {
				// Exactly the handle returned by our CreateProcess, never a
				// process name, a guessed PID, or another application instance.
				if err := windows.TerminateProcess(parent.Process, 73); err != nil {
					t.Fatalf("terminate owned helper parent: %v", err)
				}
			} else if err := windows.SetEvent(release); err != nil {
				t.Fatal(err)
			}
			mustExit(t, parent.Process, "parent")
			if !tc.abrupt {
				var code uint32
				if err := windows.GetExitCodeProcess(parent.Process, &code); err != nil || code != 0 {
					t.Fatalf("normal parent exit: code=%d err=%v; %s", code, err, helperFailure(dir, "parent"))
				}
			}
			for i, process := range descendants {
				mustExit(t, process, fmt.Sprintf("descendant %d", report.Processes[i].PID))
			}
			if state, err := windows.WaitForSingleObject(cleanup, 0); err != nil || state != uint32(windows.WAIT_TIMEOUT) {
				t.Fatalf("cleanup event unexpectedly signaled during containment test: %d %v", state, err)
			}
			if outer != 0 {
				var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
				if err := queryJobLimits(outer, &limits); err != nil {
					t.Fatalf("outer job must still be open during death assertion: %v", err)
				}
			}
		})
	}
}

func mustEvent(t *testing.T) windows.Handle {
	t.Helper()
	h, err := newInheritedEvent()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = windows.CloseHandle(h) })
	return h
}

func newInheritedEvent() (windows.Handle, error) {
	attrs := windows.SecurityAttributes{InheritHandle: 1}
	attrs.Length = uint32(unsafe.Sizeof(attrs))
	return windows.CreateEvent(&attrs, 1, 0, nil)
}

func startNativeHelper(args helperArguments, flags uint32) (*windows.ProcessInformation, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	argv := []string{exe, "-test.run=^TestProcessGuardHelper$", "--", helperMarker, args.role, args.dir}
	for _, h := range []windows.Handle{args.ready, args.release, args.cleanup, args.job} {
		argv = append(argv, strconv.FormatUint(uint64(h), 10))
	}
	command, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(argv))
	if err != nil {
		return nil, err
	}
	application, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return nil, err
	}
	dir, err := windows.UTF16PtrFromString(args.dir)
	if err != nil {
		return nil, err
	}
	var startup windows.StartupInfo
	startup.Cb = uint32(unsafe.Sizeof(startup))
	var process windows.ProcessInformation
	// Intentionally use inheritHandles=true WITHOUT an allowlist. Go's exec
	// normally allowlists handles and would hide a job-handle inheritance bug.
	err = windows.CreateProcess(application, command, nil, nil, true,
		flags|windows.CREATE_NO_WINDOW, nil, dir, &startup, &process)
	if err != nil {
		return nil, fmt.Errorf("create %s helper: %w", args.role, err)
	}
	return &process, nil
}

func waitHelperReady(process, ready windows.Handle) error {
	state, err := windows.WaitForMultipleObjects([]windows.Handle{ready, process}, false, helperWaitMilliseconds)
	if err != nil {
		return err
	}
	if state != windows.WAIT_OBJECT_0 {
		return fmt.Errorf("helper did not become ready (wait state %d)", state)
	}
	return nil
}

func mustExit(t *testing.T, process windows.Handle, label string) {
	t.Helper()
	state, err := windows.WaitForSingleObject(process, 5000)
	if err != nil || state != windows.WAIT_OBJECT_0 {
		t.Fatalf("%s survived parent exit (state %d, error %v)", label, state, err)
	}
}

func cleanupOwnedProcess(t *testing.T, process windows.Handle) {
	t.Helper()
	defer windows.CloseHandle(process)
	state, err := windows.WaitForSingleObject(process, 0)
	if err != nil {
		t.Errorf("wait for owned process during cleanup: %v", err)
		return
	}
	if state == uint32(windows.WAIT_TIMEOUT) {
		if err := windows.TerminateProcess(process, 74); err != nil {
			t.Errorf("terminate owned test process during cleanup: %v", err)
			return
		}
		if state, err := windows.WaitForSingleObject(process, 5000); err != nil || state != windows.WAIT_OBJECT_0 {
			t.Errorf("owned process did not exit during cleanup: %d %v", state, err)
		}
	}
}

func processIdentity(process windows.Handle, pid uint32) (helperIdentity, error) {
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(process, &created, &exited, &kernel, &user); err != nil {
		return helperIdentity{}, err
	}
	return helperIdentity{PID: pid, Created: created}, nil
}

func openOwnedDescendant(t *testing.T, identity helperIdentity) windows.Handle {
	t.Helper()
	if identity.PID == 0 || identity.PID == windows.GetCurrentProcessId() {
		t.Fatalf("invalid owned descendant identity: %+v", identity)
	}
	h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE, false, identity.PID)
	if err != nil {
		t.Fatalf("open owned descendant %d: %v", identity.PID, err)
	}
	actual, err := processIdentity(h, identity.PID)
	if err != nil || actual != identity {
		_ = windows.CloseHandle(h)
		t.Fatalf("refusing cleanup of reused/unverified PID %d: expected=%+v actual=%+v err=%v", identity.PID, identity, actual, err)
	}
	// From now on cleanup uses this retained handle, never reopens the PID.
	t.Cleanup(func() { cleanupOwnedProcess(t, h) })
	return h
}

func writeHelperReport(args helperArguments, report helperReport) error {
	data, err := json.Marshal(report)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(args.dir, args.role+".json"), data, 0600); err != nil {
		return err
	}
	return windows.SetEvent(args.ready)
}

func readHelperReport(dir, role string) (helperReport, error) {
	var report helperReport
	data, err := os.ReadFile(filepath.Join(dir, role+".json"))
	if err != nil {
		return report, fmt.Errorf("read %s report: %w; %s", role, err, helperFailure(dir, role))
	}
	err = json.Unmarshal(data, &report)
	return report, err
}

func helperFailure(dir, role string) string {
	data, err := os.ReadFile(filepath.Join(dir, role+".error"))
	if err != nil {
		return "no helper error report"
	}
	return string(data)
}

func queryJobLimits(job windows.Handle, limits *windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION) error {
	return windows.QueryInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(limits)), uint32(unsafe.Sizeof(*limits)), nil)
}

var getHandleInformation = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetHandleInformation")

func jobHandleFlags(job windows.Handle) (uint32, error) {
	var flags uint32
	ok, _, err := getHandleInformation.Call(uintptr(job), uintptr(unsafe.Pointer(&flags)))
	if ok == 0 {
		return 0, err
	}
	return flags, nil
}

func TestProcessGuardHelper(t *testing.T) {
	var tail []string
	for i, arg := range os.Args {
		if arg == helperMarker {
			tail = os.Args[i+1:]
			break
		}
	}
	if len(tail) == 0 {
		return // ordinary runner: absolutely no job assignment
	}
	if len(tail) != 6 {
		os.Exit(70)
	}
	args := helperArguments{role: tail[0], dir: tail[1]}
	for i, target := range []*windows.Handle{&args.ready, &args.release, &args.cleanup, &args.job} {
		value, err := strconv.ParseUint(tail[i+2], 10, 64)
		if err != nil {
			os.Exit(71)
		}
		*target = windows.Handle(value)
	}
	var err error
	if args.role == "parent" {
		err = runHelperParent(args)
	} else if args.role == "child" || args.role == "grandchild" {
		err = runHelperDescendant(args)
	} else {
		err = fmt.Errorf("unknown test helper role %q", args.role)
	}
	if err != nil {
		_ = os.WriteFile(filepath.Join(args.dir, args.role+".error"), []byte(err.Error()), 0600)
		_ = windows.SetEvent(args.ready)
		os.Exit(72)
	}
	// A normal OS process exit; there is no job CloseHandle, defer, or explicit
	// descendant termination on the success path.
	os.Exit(0)
}

func runHelperParent(args helperArguments) (err error) {
	if err := Enable(); err != nil {
		return err
	}
	job := lifetime.job
	if job == 0 {
		return fmt.Errorf("Enable did not retain the lifetime job")
	}
	for range 3 {
		runtime.GC()
		if err := Enable(); err != nil || lifetime.job != job {
			return fmt.Errorf("repeated Enable changed lifetime job: %v", err)
		}
	}
	flags, err := jobHandleFlags(job)
	if err != nil {
		return err
	}
	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	if err := queryJobLimits(job, &limits); err != nil {
		return err
	}
	if limits.BasicLimitInformation.LimitFlags != windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE {
		return fmt.Errorf("unexpected lifetime job limits: %#x", limits.BasicLimitInformation.LimitFlags)
	}
	childArgs := args
	childArgs.role, childArgs.job = "child", job
	childArgs.ready, err = newInheritedEvent()
	if err != nil {
		return err
	}
	defer windows.CloseHandle(childArgs.ready)
	child, err := startNativeHelper(childArgs, 0)
	if err != nil {
		return err
	}
	defer closeHelperChild(child, &err)
	if err := waitHelperReady(child.Process, childArgs.ready); err != nil {
		return fmt.Errorf("child startup: %w; %s", err, helperFailure(args.dir, "child"))
	}
	report, err := readHelperReport(args.dir, "child")
	if err != nil {
		return err
	}
	identity, err := processIdentity(child.Process, child.ProcessId)
	if err != nil {
		return err
	}
	if len(report.Processes) != 2 || report.Processes[0] != identity {
		return fmt.Errorf("child report does not match the created process")
	}
	report.GuardHandleFlags = flags
	if err := writeHelperReport(args, report); err != nil {
		return err
	}
	state, err := windows.WaitForSingleObject(args.release, 120000)
	if err != nil || state != windows.WAIT_OBJECT_0 {
		return fmt.Errorf("parent release wait: %d %v", state, err)
	}
	return nil
}

func runHelperDescendant(args helperArguments) (err error) {
	state, err := windows.WaitForSingleObject(args.cleanup, 0)
	if err != nil || state != uint32(windows.WAIT_TIMEOUT) {
		return fmt.Errorf("inherited cleanup canary: %d %v", state, err)
	}
	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	// The parent's numerical handle must not be a usable job handle here,
	// even though CreateProcess inherited every inheritable handle.
	inherited := queryJobLimits(args.job, &limits) == nil
	identity, err := processIdentity(windows.CurrentProcess(), windows.GetCurrentProcessId())
	if err != nil {
		return err
	}
	report := helperReport{Processes: []helperIdentity{identity}, InheritedJobHandle: inherited, InheritedEventWorks: true}
	if args.role == "child" {
		leafArgs := args
		leafArgs.role = "grandchild"
		leafArgs.ready, err = newInheritedEvent()
		if err != nil {
			return err
		}
		defer windows.CloseHandle(leafArgs.ready)
		var leaf *windows.ProcessInformation
		leaf, err = startNativeHelper(leafArgs, 0)
		if err != nil {
			return err
		}
		defer closeHelperChild(leaf, &err)
		if err := waitHelperReady(leaf.Process, leafArgs.ready); err != nil {
			return fmt.Errorf("grandchild startup: %w; %s", err, helperFailure(args.dir, "grandchild"))
		}
		leafReport, err := readHelperReport(args.dir, "grandchild")
		if err != nil {
			return err
		}
		leafIdentity, err := processIdentity(leaf.Process, leaf.ProcessId)
		if err != nil {
			return err
		}
		if len(leafReport.Processes) != 1 || leafReport.Processes[0] != leafIdentity {
			return fmt.Errorf("grandchild report does not match the created process")
		}
		report.Processes = append(report.Processes, leafIdentity)
		report.InheritedJobHandle = report.InheritedJobHandle || leafReport.InheritedJobHandle
		report.InheritedEventWorks = report.InheritedEventWorks && leafReport.InheritedEventWorks
	}
	if err := writeHelperReport(args, report); err != nil {
		return err
	}
	state, err = windows.WaitForSingleObject(args.cleanup, 120000)
	if err != nil || state != windows.WAIT_OBJECT_0 {
		return fmt.Errorf("descendant cleanup wait: %d %v", state, err)
	}
	return nil
}

func closeHelperChild(child *windows.ProcessInformation, result *error) {
	// This closes PROCESS/THREAD handles, never a job handle. Success leaves
	// descendants running so the test observes OS job teardown, not a defer.
	if *result != nil {
		_ = windows.TerminateProcess(child.Process, 75)
		_, _ = windows.WaitForSingleObject(child.Process, 5000)
	}
	_ = windows.CloseHandle(child.Thread)
	_ = windows.CloseHandle(child.Process)
}
