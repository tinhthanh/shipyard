//go:build !windows

package agent

import (
	"errors"
	"os/exec"
)

func noConsole(*exec.Cmd) {}

func elevate(string, string) error { return errors.New("only on Windows") }
