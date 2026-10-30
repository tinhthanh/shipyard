package statefile

import (
	"os"

	"github.com/cosscom/shipyard/internal/winsys"
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

// private makes f readable and writable by its owner (and SYSTEM) only. A
// file's mode bits mean nothing on Windows: it takes its folder's access
// list, which can let every user of the machine read it (a BERTH_HOME of
// C:\berth). The list is replaced by one that grants the current user and
// SYSTEM, and inherits nothing.
func private(f *os.File) error { return winsys.PrivateFile(f.Name()) }
