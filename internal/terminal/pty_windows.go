package terminal

import (
	"errors"
	"os"
	"os/exec"

	"golang.org/x/term"
)

// ErrNoPTY: this build has no pseudo-terminal for local commands yet
// (Windows needs ConPTY). Terminals on boxes don't use it.
var ErrNoPTY = errors.New("local terminals are not supported on Windows yet")

// Start would run cmd in a new pseudo-terminal of cols×rows.
func Start(cmd *exec.Cmd, cols, rows int) (*os.File, error) { return nil, ErrNoPTY }

// Resize would set a pseudo-terminal's size.
func Resize(f *os.File, cols, rows int) error { return ErrNoPTY }

// Size is the console's size in cells.
func Size(fd uintptr) (cols, rows int, err error) { return term.GetSize(int(fd)) }

// MakeRaw puts the console in raw mode and returns how to restore it.
func MakeRaw(fd uintptr) (restore func(), err error) {
	st, err := term.MakeRaw(int(fd))
	if err != nil {
		return nil, err
	}
	return func() { term.Restore(int(fd), st) }, nil
}

// IsTerminal reports whether fd is a console.
func IsTerminal(fd uintptr) bool { return term.IsTerminal(int(fd)) }
