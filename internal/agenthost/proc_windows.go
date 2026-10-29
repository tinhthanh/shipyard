package agenthost

import (
	"os/exec"
	"strconv"
	"syscall"

	"golang.org/x/sys/windows"
)

// ownGroup starts the agent in a process group of its own, without a
// console window of its own.
func ownGroup() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW}
}

// signalTree ends the process tree under pid. Windows has no signals to
// send a tree: a SIGTERM asks taskkill to close it, anything else forces it.
func signalTree(pid int, sig syscall.Signal) {
	args := []string{"/T", "/PID", strconv.Itoa(pid)}
	if sig != syscall.SIGTERM {
		args = append([]string{"/F"}, args...)
	}
	exec.Command("taskkill", args...).Run()
}

func killPID(pid int) { exec.Command("taskkill", "/F", "/PID", strconv.Itoa(pid)).Run() }

func alive(pid int) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	return windows.GetExitCodeProcess(h, &code) == nil && code == 259 // STILL_ACTIVE
}
