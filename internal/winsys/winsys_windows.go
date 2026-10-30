package winsys

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

// SharedSSH is whether ssh can share one connection between commands
// (ControlMaster): Windows' OpenSSH can't, so each logs in itself.
const SharedSSH = false

// Detached starts a program in a process group of its own, so it outlives
// the console that started it, with a console that is never shown: the
// programs it starts share that one rather than each opening a window.
func Detached() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW}
}

// HideWindow keeps a console program cmd starts from opening a window.
func HideWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NO_WINDOW
}

// ConsoleTwin is exe's console build when exe is one of Shipyard's programs
// without a console and the console build is beside it; otherwise exe.
func ConsoleTwin(exe string) string {
	name := twinName(filepath.Base(exe))
	if name == "" {
		return exe
	}
	twin := filepath.Join(filepath.Dir(exe), name)
	if st, err := os.Stat(twin); err != nil || !st.Mode().IsRegular() {
		return exe
	}
	return twin
}

// WindowlessTwin is the build of exe without a console, when exe is one of
// Shipyard's console programs and that build is beside it; otherwise exe:
// what Windows should start at sign-in, so no console window opens.
func WindowlessTwin(exe string) string {
	name := windowlessName(filepath.Base(exe))
	if name == "" {
		return exe
	}
	twin := filepath.Join(filepath.Dir(exe), name)
	if st, err := os.Stat(twin); err != nil || !st.Mode().IsRegular() {
		return exe
	}
	return twin
}

// RelaunchHidden starts this program's console build with args and a
// console that is never shown, when this is a program without one, and
// says whether it did: the caller is then done. A program with no console
// gives every console program it starts (tmux, git, PowerShell) a window
// of its own, which Windows Terminal opens: dozens.
func RelaunchHidden(args []string) bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	twin := ConsoleTwin(exe)
	if twin == exe {
		return false
	}
	cmd := exec.Command(twin, args...)
	cmd.SysProcAttr = Detached()
	if err := cmd.Start(); err != nil {
		return false
	}
	cmd.Process.Release()
	return true
}

// StopIfIn ends process pid and waits, briefly, for it to be gone, but only
// when its program is in dir and its file name starts with prefix: a pid
// from a file may since belong to anything. Windows names a running program
// by its file's name now, so one moved aside (berthd.exe.old) still counts.
func StopIfIn(pid int, dir, prefix string) {
	if pid <= 0 || pid == os.Getpid() {
		return
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	if windows.QueryFullProcessImageName(h, 0, &buf[0], &n) != nil {
		return
	}
	image := windows.UTF16ToString(buf[:n])
	if !strings.EqualFold(filepath.Dir(image), filepath.Clean(dir)) || !strings.HasPrefix(strings.ToLower(filepath.Base(image)), strings.ToLower(prefix)) {
		return
	}
	if windows.TerminateProcess(h, 0) == nil {
		windows.WaitForSingleObject(h, 10_000)
	}
}

// Elevate runs a program as an administrator, through Windows' own prompt.
func Elevate(program, args string) error {
	verb, _ := windows.UTF16PtrFromString("runas")
	exe, _ := windows.UTF16PtrFromString(program)
	params, _ := windows.UTF16PtrFromString(args)
	return windows.ShellExecute(0, verb, exe, params, nil, windows.SW_SHOWNORMAL)
}

// ownerOnly is an access list for the current user and SYSTEM alone,
// inherited by what a folder holds when inherit is set.
func ownerOnly(inherit bool) (*windows.ACL, error) {
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	flags := ""
	if inherit {
		flags = "OICI"
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;" + flags + ";FA;;;SY)(A;" + flags + ";FA;;;" + u.User.Sid.String() + ")")
	if err != nil {
		return nil, err
	}
	dacl, _, err := sd.DACL()
	return dacl, err
}

func setOwnerOnly(path string, inherit bool) error {
	dacl, err := ownerOnly(inherit)
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}

// PrivateFile lets only the current user (and SYSTEM) open path. A file's
// mode bits mean nothing on Windows: it takes its folder's access list,
// which can let every user of the computer read it. By name, as a handle
// opened to read and write has no right to change it.
func PrivateFile(path string) error { return setOwnerOnly(path, false) }

// PrivateDir lets only the current user (and SYSTEM) into dir and what it
// holds, now and later: files without a list of their own take its, and
// change with it. Once the folder's list is protected, it is left be.
func PrivateDir(dir string) error {
	if sd, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION); err == nil {
		if ctl, _, err := sd.Control(); err == nil && ctl&windows.SE_DACL_PROTECTED != 0 {
			return nil
		}
	}
	return setOwnerOnly(dir, true)
}
