//go:build windows

package main

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

var getConsoleProcessList = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleProcessList")

func standaloneConsole() bool {
	var mode uint32
	if windows.GetConsoleMode(windows.Handle(os.Stdin.Fd()), &mode) != nil || windows.GetConsoleMode(windows.Handle(os.Stderr.Fd()), &mode) != nil {
		return false
	}
	if err := getConsoleProcessList.Find(); err != nil {
		return false
	}
	var processes [2]uint32
	count, _, _ := getConsoleProcessList.Call(uintptr(unsafe.Pointer(&processes[0])), uintptr(len(processes)))
	// A PowerShell/cmd parent shares this console and must get its prompt back.
	// More than the buffer capacity still returns the required process count.
	return count == 1 && processes[0] == uint32(os.Getpid())
}

func prepareConsole() {
	// Only a standalone console: never mutate an existing shell's code page.
	_ = windows.SetConsoleOutputCP(65001)
}
