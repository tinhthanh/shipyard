package service

import (
	"syscall"

	"github.com/cosscom/shipyard/internal/winsys"
)

// ownSession starts a command without a console window of its own.
func ownSession() *syscall.SysProcAttr { return winsys.Detached() }
