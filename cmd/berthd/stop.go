package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cosscom/shipyard/internal/winsys"
)

// stopServing ends the berthd this box's service runs now, found by the pid
// serve wrote, when it is a berthd in program's folder: Windows has no
// service manager to restart it, and the berthd replacing it could neither
// listen nor replace the program while it runs. Elsewhere it does nothing.
func stopServing(b boxHome, program string) {
	raw, err := os.ReadFile(filepath.Join(b.dir, "berthd.pid"))
	if err != nil {
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		return
	}
	winsys.StopIfIn(pid, filepath.Dir(program), "berthd")
}
