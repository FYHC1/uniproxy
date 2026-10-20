//go:build windows

package processguard

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// lifetime owns the only handle to an unnamed job. A raw handle has no Go
// finalizer: it is intentionally retained until process teardown, not closed by
// ordinary shutdown, a core Stop, garbage collection, or a deferred cleanup.
var lifetime lifetimeGuard

type lifetimeGuard struct {
	once sync.Once
	job  windows.Handle
	err  error
}

// jobAPI permits deterministic pre-assignment failure tests without assigning
// the test runner itself to a job. It is private; no PID/termination API escapes
// this package.
type jobAPI struct {
	create         func(*windows.SecurityAttributes, *uint16) (windows.Handle, error)
	setHandleFlags func(windows.Handle, uint32, uint32) error
	setLimits      func(windows.Handle, *windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION) error
	assign         func(windows.Handle, windows.Handle) error
	close          func(windows.Handle) error
}

var systemJobAPI = jobAPI{
	create:         windows.CreateJobObject,
	setHandleFlags: windows.SetHandleInformation,
	setLimits: func(job windows.Handle, limits *windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION) error {
		_, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(limits)), uint32(unsafe.Sizeof(*limits)))
		return err
	},
	assign: windows.AssignProcessToJobObject,
	close:  windows.CloseHandle,
}

func enable() error { return lifetime.enable(systemJobAPI) }

func (g *lifetimeGuard) enable(api jobAPI) error {
	g.once.Do(func() {
		// Nil attributes make the handle non-inheritable at creation, so there
		// is no interval in which a concurrent child could inherit it. Nil
		// name prevents descendants from reopening the job by a shared name.
		job, err := api.create(nil, nil)
		if err != nil {
			g.err = fmt.Errorf("processguard: create Windows lifetime job: %w", err)
			return
		}
		if err := api.setHandleFlags(job, windows.HANDLE_FLAG_INHERIT, 0); err != nil {
			g.err = releaseUnassigned(api, job, "make Windows lifetime job non-inheritable", err)
			return
		}
		var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
		// Do not allow explicit or silent breakaway. Job membership, unlike
		// handle inheritance, is inherited atomically by future children.
		limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
		if err := api.setLimits(job, &limits); err != nil {
			g.err = releaseUnassigned(api, job, "set Windows lifetime job kill-on-close", err)
			return
		}
		if err := api.assign(job, windows.CurrentProcess()); err != nil {
			// A restrictive existing job may reject nested assignment. Never
			// silently run uncontained or try to escape the existing job.
			g.err = releaseUnassigned(api, job, "assign current process to Windows lifetime job (subprocess containment is required)", err)
			return
		}
		// No fallible operations or CloseHandle after successful assignment:
		// closing the last handle now would terminate the current process.
		g.job = job
	})
	return g.err
}

// Only failure paths before successful current-process assignment call this.
// In particular, this must never be deferred across AssignProcessToJobObject.
func releaseUnassigned(api jobAPI, job windows.Handle, operation string, cause error) error {
	err := fmt.Errorf("processguard: %s: %w", operation, cause)
	if closeErr := api.close(job); closeErr != nil {
		return errors.Join(err, fmt.Errorf("processguard: release unassigned Windows lifetime job: %w", closeErr))
	}
	return err
}
