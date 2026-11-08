//go:build !linux && !darwin && !windows

package box

import (
	"syscall"
	"time"
)

// Helpers run on Linux, macOS and Windows boxes.
const helperExe = ""

// helperBinPerVersion: the program sits in bin/<version>/.
const helperBinPerVersion = true

func helperPlatformOK() bool { return false }

func helperSysProcAttr() *syscall.SysProcAttr { return nil }

func terminateHelper(pid int, exited <-chan error, timeout time.Duration) {}

func signalProcess(pid int, sig syscall.Signal) {}

func processAlive(pid int) bool { return false }
