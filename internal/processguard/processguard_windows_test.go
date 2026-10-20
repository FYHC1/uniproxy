//go:build windows

package processguard

import (
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"golang.org/x/sys/windows"
)

type fakeJob struct {
	calls    []string
	failAt   string
	closeErr error
}

const fakeJobHandle windows.Handle = 1234

func (f *fakeJob) api(t *testing.T) jobAPI {
	t.Helper()
	call := func(step string) error {
		f.calls = append(f.calls, step)
		if f.failAt == step {
			return windows.ERROR_ACCESS_DENIED
		}
		return nil
	}
	checkJob := func(job windows.Handle) {
		if job != fakeJobHandle {
			t.Errorf("job = %v, want owned handle %v", job, fakeJobHandle)
		}
	}
	return jobAPI{
		create: func(attrs *windows.SecurityAttributes, name *uint16) (windows.Handle, error) {
			if attrs != nil || name != nil {
				t.Error("job must be unnamed and non-inheritable from creation")
			}
			if err := call("create"); err != nil {
				return 0, err
			}
			return fakeJobHandle, nil
		},
		setHandleFlags: func(job windows.Handle, mask, flags uint32) error {
			checkJob(job)
			if mask != windows.HANDLE_FLAG_INHERIT || flags != 0 {
				t.Errorf("SetHandleInformation mask=%#x flags=%#x", mask, flags)
			}
			return call("non-inheritable")
		},
		setLimits: func(job windows.Handle, limits *windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION) error {
			checkJob(job)
			var want windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
			want.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
			if !reflect.DeepEqual(*limits, want) {
				t.Errorf("job limits = %+v, want only KILL_ON_JOB_CLOSE", *limits)
			}
			return call("kill-on-close")
		},
		assign: func(job, process windows.Handle) error {
			checkJob(job)
			if process != windows.CurrentProcess() {
				t.Errorf("assignment targets %v, not the current process", process)
			}
			return call("assign")
		},
		close: func(job windows.Handle) error {
			checkJob(job)
			f.calls = append(f.calls, "close")
			return f.closeErr
		},
	}
}

func TestEnableWindowsSetupOnce(t *testing.T) {
	// This local guard uses only fakes. Never attach the main test runner.
	var guard lifetimeGuard
	fake := new(fakeJob)
	api := fake.api(t)
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := guard.enable(api); err != nil {
				t.Errorf("Enable: %v", err)
			}
		}()
	}
	wg.Wait()
	if guard.job != fakeJobHandle {
		t.Errorf("retained handle = %v, want %v", guard.job, fakeJobHandle)
	}
	want := []string{"create", "non-inheritable", "kill-on-close", "assign"}
	if !reflect.DeepEqual(fake.calls, want) {
		t.Errorf("calls = %v, want %v (no close after assignment)", fake.calls, want)
	}
}

func TestEnableWindowsFailuresAreFatalAndSticky(t *testing.T) {
	cases := []struct {
		step    string
		message string
		calls   []string
	}{
		{"create", "create Windows lifetime job", []string{"create"}},
		{"non-inheritable", "non-inheritable", []string{"create", "non-inheritable", "close"}},
		{"kill-on-close", "kill-on-close", []string{"create", "non-inheritable", "kill-on-close", "close"}},
		{"assign", "subprocess containment is required", []string{"create", "non-inheritable", "kill-on-close", "assign", "close"}},
	}
	for _, tc := range cases {
		t.Run(tc.step, func(t *testing.T) {
			var guard lifetimeGuard
			fake := &fakeJob{failAt: tc.step}
			err := guard.enable(fake.api(t))
			if !errors.Is(err, windows.ERROR_ACCESS_DENIED) || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("error = %v, want stage-specific wrapped access-denied error", err)
			}
			// Nil functions would panic if failure were retried or silently ignored.
			if second := guard.enable(jobAPI{}); second != err {
				t.Errorf("second Enable = %v, want original failure %v", second, err)
			}
			if guard.job != 0 {
				t.Errorf("failed guard retained handle %v", guard.job)
			}
			if !reflect.DeepEqual(fake.calls, tc.calls) {
				t.Errorf("calls = %v, want %v", fake.calls, tc.calls)
			}
		})
	}
}

func TestEnableWindowsReportsUnassignedHandleReleaseFailure(t *testing.T) {
	var guard lifetimeGuard
	fake := &fakeJob{failAt: "assign", closeErr: windows.ERROR_INVALID_HANDLE}
	err := guard.enable(fake.api(t))
	if !errors.Is(err, windows.ERROR_ACCESS_DENIED) || !errors.Is(err, windows.ERROR_INVALID_HANDLE) {
		t.Fatalf("error = %v, want both assignment and release errors", err)
	}
	if !strings.Contains(err.Error(), "release unassigned") {
		t.Fatalf("error does not identify failed cleanup: %v", err)
	}
	if second := guard.enable(jobAPI{}); second != err {
		t.Fatalf("failed release must not retry setup: %v", second)
	}
}
