package main

import (
	"os"
	"syscall"
	"time"

	"github.com/cosscom/shipyard/internal/terminal"
	"golang.org/x/sys/windows"
)

// detached starts a program with no console, in a process group of its
// own, so it outlives the console that started it.
func detached() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP}
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
