package main

import (
	"os"
	"syscall"
	"time"

	"github.com/cosscom/shipyard/internal/terminal"
	"github.com/cosscom/shipyard/internal/winsys"
)

// Windows runs the agent's service, a Run value (service, runkey.go), at
// sign-in only: no supervisor restarts it, or is there to ask to start it.
const (
	serviceSupervises  = false
	agentInstalledNote = "The agent now starts when you sign in to Windows; berth stop still stops it."
)

// serviceProgram is what the Run value runs: `agent start`, which starts the
// agent, detached, with its log, and does nothing while one runs, from
// berth-agent.exe, the build without a console, so none opens at sign-in.
func serviceProgram(exe string) (string, []string) {
	return winsys.WindowlessTwin(exe), []string{"agent", "start"}
}

// detached starts the agent in a process group of its own, with a console
// that is never shown (winsys.Detached).
func detached() *syscall.SysProcAttr { return winsys.Detached() }

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
