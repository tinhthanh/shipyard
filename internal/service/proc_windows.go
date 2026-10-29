package service

import (
	"syscall"

	"golang.org/x/sys/windows"
)

// ownSession starts a command without a console window of its own.
func ownSession() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW}
}
