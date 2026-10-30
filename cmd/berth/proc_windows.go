package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/cosscom/shipyard/internal/terminal"
	"golang.org/x/sys/windows"
)

// detached starts a program in a process group of its own, so it outlives
// the console that started it, with a console that is never shown: the
// programs it starts (ssh, git) share that one rather than each opening a
// window, as they would from a program with no console at all.
func detached() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW | windows.CREATE_NEW_PROCESS_GROUP}
}

// consoleBerth is the berth that runs the agent: berth-cli.exe rather than
// berth-agent.exe, which has no console, and so would give each console
// program the agent starts a window of its own.
func consoleBerth(exe string) string {
	if strings.EqualFold(filepath.Base(exe), "berth-agent.exe") {
		if cli := filepath.Join(filepath.Dir(exe), "berth-cli.exe"); fileExists(cli) {
			return cli
		}
	}
	return exe
}

// resizes sends when the console on fd changes size. Windows has no
// SIGWINCH: the size is looked at a few times a second.
func resizes(fd uintptr) (<-chan os.Signal, func()) {
	ch := make(chan os.Signal, 1)
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(300 * time.Millisecond)
		defer t.Stop()
		lc, lr, _ := terminal.Size(fd)
		for {
			select {
			case <-done:
				return
			case <-t.C:
				if c, r, err := terminal.Size(fd); err == nil && (c != lc || r != lr) {
					lc, lr = c, r
					select {
					case ch <- syscall.Signal(0):
					default:
					}
				}
			}
		}
	}()
	return ch, func() { close(done) }
}
