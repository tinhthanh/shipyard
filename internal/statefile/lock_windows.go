package statefile

import (
	"os"

	"golang.org/x/sys/windows"
)

// The whole file, as far as a lock can reach: LockFileEx locks a byte
// range, and a range past the end is allowed.
const lockLow, lockHigh = ^uint32(0), ^uint32(0)

func lockFile(f *os.File) error {
	return windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, lockLow, lockHigh, new(windows.Overlapped))
}

func unlockFile(f *os.File) {
	windows.UnlockFileEx(windows.Handle(f.Fd()), 0, lockLow, lockHigh, new(windows.Overlapped))
}
