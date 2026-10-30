package agentpath

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"

	"github.com/cosscom/shipyard/internal/winsys"
)

// ownSession starts a command in a process group of its own and without a
// console window of its own (an agent's CLI is a console program).
func ownSession() *syscall.SysProcAttr { return winsys.Detached() }

// killSession ends the process and everything it started: Windows has no
// process groups to signal, so taskkill walks the tree.
func killSession(p *os.Process) error {
	if err := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(p.Pid)).Run(); err != nil {
		return p.Kill()
	}
	return nil
}
