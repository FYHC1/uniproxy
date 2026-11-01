//go:build !windows && !linux

package processguard

import "os/exec"

func enable() error { return nil }

func prepareCommand(*exec.Cmd) {}
