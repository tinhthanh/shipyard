//go:build !windows

package main

import (
	"os"
	"os/signal"
	"syscall"
)

// detached starts a program in a session of its own, so it outlives the
// terminal that started it.
func detached() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setsid: true} }

// resizes sends when the terminal on fd changes size.
func resizes(fd uintptr) (<-chan os.Signal, func()) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	return ch, func() { signal.Stop(ch) }
}

// consoleBerth is Windows' (proc_windows.go).
func consoleBerth(exe string) string { return exe }
