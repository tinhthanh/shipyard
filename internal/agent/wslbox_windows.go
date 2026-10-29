package agent

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// noConsole keeps a console program the agent runs from opening a window:
// the agent itself has none (berth-agent.exe).
func noConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
}

// elevate runs a program as an administrator, through Windows' own prompt.
func elevate(program, args string) error {
	verb, _ := windows.UTF16PtrFromString("runas")
	exe, _ := windows.UTF16PtrFromString(program)
	params, _ := windows.UTF16PtrFromString(args)
	return windows.ShellExecute(0, verb, exe, params, nil, windows.SW_SHOWNORMAL)
}
