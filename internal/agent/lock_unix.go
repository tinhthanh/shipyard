//go:build !windows

package agent

import (
	"os"
	"syscall"
)

// tryLock takes an exclusive lock on f without waiting for it.
func tryLock(f *os.File) error { return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) }

func unlock(f *os.File) { syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }
