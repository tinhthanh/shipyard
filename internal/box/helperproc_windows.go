//go:build windows

package box

import (
	"os/exec"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sys/windows"

	"github.com/cosscom/shipyard/internal/winsys"
)

// helperExe is the suffix a helper's program needs to be started: Windows
// only runs a file by its extension.
const helperExe = ".exe"

// helperBinPerVersion: on Windows a helper's program keeps one path across
// updates (bin\<name>.exe), since Windows Firewall remembers a program's
// network permission by its path and would ask again after every update.
// Only one version is ever kept and the old one is stopped first, so the
// path is free when the new one moves in.
const helperBinPerVersion = false

// helperPlatformOK says this box can run helpers.
func helperPlatformOK() bool { return true }

// helperSysProcAttr starts a helper in a process group of its own, with no
// console window: berthd runs without one, and a helper is a console program.
func helperSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW,
		HideWindow:    true,
	}
}

// terminateHelper stops a helper and what it started. A console program
// without a console of berthd's can't be sent Ctrl-Break, so the tree is
// ended (taskkill /T) at once and then forced if it lingers past timeout.
// exited yields when the helper's own process has ended.
func terminateHelper(pid int, exited <-chan error, timeout time.Duration) {
	killTree(pid, false)
	select {
	case <-exited:
	case <-time.After(timeout):
		killTree(pid, true)
		<-exited
	}
}

// killTree ends pid and the processes it started; force adds /F.
func killTree(pid int, force bool) {
	args := []string{"/T", "/PID", strconv.Itoa(pid)}
	if force {
		args = append([]string{"/F"}, args...)
	}
	cmd := exec.Command("taskkill", args...)
	winsys.HideWindow(cmd)
	if cmd.Run() != nil && !force {
		// A console program ignores the polite close: force it.
		killTree(pid, true)
	}
}

// signalProcess ends a process: Windows has no signals to send it, so both
// SIGTERM and SIGKILL end it (the first politely, then by force).
func signalProcess(pid int, sig syscall.Signal) {
	killTree(pid, sig == syscall.SIGKILL)
}

// processAlive reports whether a process is still there.
func processAlive(pid int) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	if windows.GetExitCodeProcess(h, &code) != nil {
		return false
	}
	return code == 259 // STILL_ACTIVE
}
