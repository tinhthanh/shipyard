//go:build !windows

package agentpath

import (
	"os"
	"syscall"
)

// ownSession starts a command in a session of its own, so killSession
// reaches everything it started.
func ownSession() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setsid: true} }

func killSession(p *os.Process) error { return syscall.Kill(-p.Pid, syscall.SIGKILL) }
