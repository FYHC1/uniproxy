// Package processguard ties subprocess lifetime to the UniProxy executable.
// It is not a per-core manager and must not be used to stop or restart a core.
package processguard

import "os/exec"

// Enable contains future subprocesses for the lifetime of this process on
// Windows and is a no-op on other platforms.
//
// The executable must call Enable once, after CLI-only modes have returned and
// before starting any subprocess (including core version and validation probes).
// An error is fatal to startup: the caller must not continue without containment.
// Repeated calls return the first result; they do not create another job or retry
// a failed assignment.
//
// There is deliberately no Close or Stop operation. The Windows job handle stays
// open until the OS tears down this process, including on abrupt termination.
// Closing it from a defer or a core manager would kill this process as well as
// its descendants. Do not call Enable in a shared test runner; use a subprocess.
func Enable() error {
	return enable()
}

// PrepareCommand configures a child process before Start so an abrupt parent
// death does not leave it running. On Linux it sets the best-effort
// PR_SET_PDEATHSIG value to SIGTERM; on other platforms it is a no-op.
//
// Call this after creating every core subprocess command, including version
// probes and configuration validation. On Linux the signal is delivered when
// the thread that created the child exits, so this is a safety net rather than
// a replacement for the normal Stop path.
func PrepareCommand(cmd *exec.Cmd) {
	if cmd != nil {
		prepareCommand(cmd)
	}
}
