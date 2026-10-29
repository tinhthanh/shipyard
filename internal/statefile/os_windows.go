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

// private makes f readable and writable by its owner (and SYSTEM) only. A
// file's mode bits mean nothing on Windows: it takes its folder's access
// list, which can let every user of the machine read it (a BERTH_HOME of
// C:\berth). The list is replaced by one that grants the current user and
// SYSTEM, and inherits nothing.
func private(f *os.File) error {
	tok := windows.GetCurrentProcessToken()
	u, err := tok.GetTokenUser()
	if err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;SY)(A;;FA;;;" + u.User.Sid.String() + ")")
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetSecurityInfo(windows.Handle(f.Fd()), windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}
