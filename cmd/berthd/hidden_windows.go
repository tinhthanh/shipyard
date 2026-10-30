package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

// runHidden: Windows starts berthd-bg.exe at sign-in, built without a
// console so none shows. But a program without a console gives every
// console program it starts (tmux, git, PowerShell) a console window of its
// own, which Windows Terminal opens: dozens. So berthd-bg.exe only starts
// berthd.exe, beside it, with the same arguments and a console that is never
// shown, which the programs it starts share, and is done.
func runHidden(args []string) bool {
	exe, err := os.Executable()
	if err != nil || !strings.EqualFold(filepath.Base(exe), "berthd-bg.exe") {
		return false
	}
	console := filepath.Join(filepath.Dir(exe), "berthd.exe")
	if st, err := os.Stat(console); err != nil || !st.Mode().IsRegular() {
		return false
	}
	cmd := exec.Command(console, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW | windows.CREATE_NEW_PROCESS_GROUP}
	if err := cmd.Start(); err != nil {
		return false
	}
	cmd.Process.Release()
	return true
}
