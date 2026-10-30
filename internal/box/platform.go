package box

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// What a box does differently on Windows, where berthd runs on Windows
// itself (Labs): sessions in PowerShell, under psmux, the tmux for Windows
// that comes beside berthd. Each function answers "" or false elsewhere,
// and the shared code goes on its usual way.

// platformShell is the shell a session runs in when the system decides it:
// on Windows, PowerShell (pwsh, else Windows PowerShell), which has no
// SHELL to name it.
func platformShell() string {
	if runtime.GOOS != "windows" {
		return ""
	}
	if p, err := exec.LookPath("pwsh.exe"); err == nil {
		return p
	}
	return "powershell.exe"
}

// tmuxConfigFor is berth's tmux configuration for this system: on Windows,
// programs (Git's less, say) know no tmux-256color.
func tmuxConfigFor(config string) string {
	if runtime.GOOS != "windows" {
		return config
	}
	return strings.Replace(config, `"tmux-256color"`, `"xterm-256color"`, 1)
}

// bundledTmux is the tmux beside berthd on Windows (psmux), which comes
// before PATH: a service started at sign-in has no PATH of its own, and a
// tmux on PATH (MSYS2's, say) can't drive a Windows console.
func bundledTmux() string {
	if runtime.GOOS != "windows" {
		return ""
	}
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	p := filepath.Join(filepath.Dir(exe), "tmux.exe")
	if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() {
		return p
	}
	return ""
}

// platformHostSocket is where an agent's host listens on Windows: the
// user's own temporary folder, which only they can open, as a Windows file
// has no Unix owner or mode to check. ok is false elsewhere.
func platformHostSocket(id string) (path string, ok bool, err error) {
	if runtime.GOOS != "windows" {
		return "", false, nil
	}
	dir := filepath.Join(os.TempDir(), "berth")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", true, err
	}
	return filepath.Join(dir, id+".sock"), true, nil
}
