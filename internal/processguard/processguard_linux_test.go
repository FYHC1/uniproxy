//go:build linux

package processguard

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const linuxHelperEnv = "UNIPROXY_PROCESSGUARD_LINUX_HELPER"

func TestPrepareCommandSetsPdeathsig(t *testing.T) {
	cmd := exec.Command("true")
	PrepareCommand(cmd)
	if cmd.SysProcAttr == nil || cmd.SysProcAttr.Pdeathsig != syscall.SIGTERM {
		t.Fatalf("Pdeathsig = %#v, want SIGTERM", cmd.SysProcAttr)
	}
	if err := cmd.Run(); err != nil {
		t.Fatalf("run prepared command: %v", err)
	}
}

// TestChildDiesWhenParentDies exercises the real kernel behavior in a
// disposable helper. The test runner itself is never killed or reconfigured.
func TestChildDiesWhenParentDies(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestProcessGuardLinuxHelper$")
	cmd.Env = append(os.Environ(), linuxHelperEnv+"=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var childPID int
	cleanup := func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		if childPID > 0 {
			_ = syscall.Kill(childPID, syscall.SIGKILL)
		}
	}
	defer cleanup()

	lineCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "CHILD_PID=") {
				lineCh <- line
				return
			}
		}
		if err := scanner.Err(); err != nil {
			errCh <- err
			return
		}
		errCh <- fmt.Errorf("helper exited without reporting a child PID")
	}()
	select {
	case line := <-lineCh:
		childPID, err = strconv.Atoi(strings.TrimPrefix(line, "CHILD_PID="))
		if err != nil || childPID <= 0 {
			t.Fatalf("invalid child PID %q: %v", line, err)
		}
	case err := <-errCh:
		t.Fatalf("helper startup: %v; stderr: %s", err, stderr.String())
	case <-time.After(10 * time.Second):
		t.Fatalf("helper startup timed out; stderr: %s", stderr.String())
	}

	if err := cmd.Process.Kill(); err != nil {
		t.Fatalf("kill owned helper parent: %v", err)
	}
	_ = cmd.Wait()
	deadline := time.Now().Add(5 * time.Second)
	for !linuxProcessGone(childPID) {
		if time.Now().After(deadline) {
			t.Fatalf("child %d survived parent death; stderr: %s", childPID, stderr.String())
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestProcessGuardLinuxHelper(t *testing.T) {
	if os.Getenv(linuxHelperEnv) != "1" {
		return
	}
	child := exec.Command("sleep", "120")
	PrepareCommand(child)
	if err := child.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "start helper child:", err)
		os.Exit(2)
	}
	fmt.Printf("CHILD_PID=%d\n", child.Process.Pid)
	time.Sleep(2 * time.Minute)
	_ = child.Process.Kill()
	os.Exit(0)
}

func linuxProcessGone(pid int) bool {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return true
	}
	fields := strings.Fields(string(data))
	return len(fields) >= 3 && fields[2] == "Z"
}
