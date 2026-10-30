//go:build !windows

package main

import (
	"os"
	"os/signal"
	"syscall"
)

// The agent's service is a launchd agent or a systemd unit, which starts
// `berth agent` at login and restarts it after a crash.
const (
	serviceSupervises  = true
	agentInstalledNote = "The agent now starts at login and restarts after a crash; berth stop still stops it."
)

// serviceProgram is what the agent's service runs.
func serviceProgram(exe string) (string, []string) { return exe, []string{"agent"} }

// detached starts a program in a session of its own, so it outlives the
// terminal that started it.
func detached() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setsid: true} }

// resizes sends when the terminal on fd changes size.
func resizes(fd uintptr) (<-chan os.Signal, func()) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	return ch, func() { signal.Stop(ch) }
}
