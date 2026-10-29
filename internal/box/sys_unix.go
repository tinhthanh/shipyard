//go:build !windows

package box

import (
	"errors"
	"os"
	"syscall"
)

// What berthd asks of the operating system about processes and files,
// kept here so the rest of the package builds on Windows too
// (sys_windows.go).

const (
	sigStop = syscall.SIGSTOP
	sigCont = syscall.SIGCONT
	sigTTIN = syscall.SIGTTIN
)

// ownGroup starts a command in a process group of its own, so
// signalGroup reaches what it starts.
func ownGroup() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setpgid: true} }

// ownSession starts a command in a session of its own, with no
// controlling terminal.
func ownSession() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setsid: true} }

func signalPID(pid int, sig syscall.Signal) error { return syscall.Kill(pid, sig) }

// signalGroup signals the process group pid leads.
func signalGroup(pid int, sig syscall.Signal) error { return syscall.Kill(-pid, sig) }

// processExists reports whether pid runs, even as another user's.
func processExists(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// fileOwner is the user id that owns a file, when the system says.
func fileOwner(fi os.FileInfo) (uid int, ok bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return int(st.Uid), true
}

// diskSpace is the size and the space available on the filesystem at path,
// and an id that tells one filesystem from another.
func diskSpace(path string) (total, avail, id uint64, ok bool) {
	var st syscall.Statfs_t
	if syscall.Statfs(path, &st) != nil {
		return 0, 0, 0, false
	}
	bs := uint64(st.Bsize)
	return uint64(st.Blocks) * bs, uint64(st.Bavail) * bs, uint64(st.Blocks)<<16 ^ uint64(st.Bsize), true
}
