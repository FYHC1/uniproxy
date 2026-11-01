//go:build linux

package processguard

import (
	"os/exec"
	"syscall"
)

func enable() error { return nil }

// Pdeathsig is best-effort: Linux delivers it when the thread that created the
// child exits, not necessarily when an arbitrary goroutine's parent process
// exits. It is still the earliest kernel-level containment available without a
// supervisor process, and it covers the normal kill -9 parent case.
func prepareCommand(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Pdeathsig = syscall.SIGTERM
}
