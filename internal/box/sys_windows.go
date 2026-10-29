package box

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"

	"golang.org/x/sys/windows"
)

// Windows versions of sys_unix.go. Windows has no signals to send a
// process: anything but a liveness check ends it, and stop/continue (which
// freeze a worktree's processes) do nothing.

const (
	sigStop = syscall.Signal(0x13)
	sigCont = syscall.Signal(0x12)
	sigTTIN = syscall.Signal(0x15)
)

func ownGroup() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW}
}

func ownSession() *syscall.SysProcAttr { return ownGroup() }

func signalPID(pid int, sig syscall.Signal) error {
	switch sig {
	case 0:
		if processExists(pid) {
			return nil
		}
		return syscall.ESRCH
	case sigStop, sigCont, sigTTIN:
		return nil
	}
	return exec.Command("taskkill", "/F", "/PID", strconv.Itoa(pid)).Run()
}

// signalGroup ends the process tree under pid.
func signalGroup(pid int, sig syscall.Signal) error {
	return exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()
}

func processExists(pid int) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return err == windows.ERROR_ACCESS_DENIED
	}
	defer windows.CloseHandle(h)
	var code uint32
	return windows.GetExitCodeProcess(h, &code) == nil && code == 259 // STILL_ACTIVE
}

// fileOwner: a Windows FileInfo has no owner; the callers treat that as
// not known.
func fileOwner(fi os.FileInfo) (uid int, ok bool) { return 0, false }

func diskSpace(path string) (total, avail, id uint64, ok bool) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, 0, false
	}
	var free, size, totalFree uint64
	if windows.GetDiskFreeSpaceEx(p, &free, &size, &totalFree) != nil {
		return 0, 0, 0, false
	}
	// The volume (C:\) tells one filesystem from another.
	vol := filepath.VolumeName(path)
	for _, c := range vol {
		id = id*31 + uint64(c)
	}
	return size, free, id, true
}
