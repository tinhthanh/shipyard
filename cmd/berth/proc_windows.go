package main

import (
	"os"
	"syscall"
	"time"

	"github.com/cosscom/shipyard/internal/terminal"
	"github.com/cosscom/shipyard/internal/winsys"
)

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
