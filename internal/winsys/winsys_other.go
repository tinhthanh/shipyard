//go:build !windows

package winsys

import (
	"errors"
	"os/exec"
	"syscall"
)

// Detached is Windows'; elsewhere a caller sets its own (Setsid).
func Detached() *syscall.SysProcAttr { return nil }

// HideWindow: no program opens a window of its own here.
func HideWindow(*exec.Cmd) {}

// ConsoleTwin: every program has its console here.
func ConsoleTwin(exe string) string { return exe }

// WindowlessTwin: every program has its console here.
func WindowlessTwin(exe string) string { return exe }

// RelaunchHidden: nothing to relaunch here.
func RelaunchHidden([]string) bool { return false }

// StopIfIn is Windows': a service manager restarts berthd elsewhere.
func StopIfIn(int, string, string) {}

// Elevate is Windows' administrator prompt.
func Elevate(string, string) error { return errors.New("only on Windows") }

// PrivateFile: mode bits do this elsewhere.
func PrivateFile(string) error { return nil }

// PrivateDir: mode bits do this elsewhere.
func PrivateDir(string) error { return nil }
