//go:build !windows

package main

func standaloneConsole() bool { return false }
func prepareConsole()         {}
