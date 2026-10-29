//go:build !windows

package statefile

import (
	"os"
	"syscall"
)

func lockFile(f *os.File) error { return syscall.Flock(int(f.Fd()), syscall.LOCK_EX) }

func unlockFile(f *os.File) { syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }

// private makes f readable and writable by its owner only.
func private(f *os.File) error { return f.Chmod(0o600) }
