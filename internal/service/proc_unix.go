//go:build !windows

package service

import "syscall"

// ownSession starts a command in a session of its own, so an interactive
// shell never takes a terminal over.
func ownSession() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setsid: true} }
