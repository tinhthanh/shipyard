//go:build !windows

package agenthost

import "syscall"

// ownGroup starts the agent in a process group of its own, so a stop
// reaches what it starts.
func ownGroup() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setpgid: true} }

// signalTree sends sig to the process tree under pid (its descendants as
// they are now, then its group, then pid itself).
func signalTree(pid int, sig syscall.Signal) {
	kids := descendants(pid)
	for i := len(kids) - 1; i >= 0; i-- {
		syscall.Kill(kids[i], sig)
	}
	syscall.Kill(-pid, sig)
	syscall.Kill(pid, sig)
}

func killPID(pid int) { syscall.Kill(pid, syscall.SIGKILL) }

func alive(pid int) bool { return syscall.Kill(pid, 0) == nil }
