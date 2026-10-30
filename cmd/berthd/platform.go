package main

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/cosscom/shipyard/internal/statefile"
	"github.com/cosscom/shipyard/internal/winsys"
)

// What berthd's service is on Windows, where a value under the user's Run
// key starts it at sign-in (service, runkey.go): no service manager, no
// console, no environment and nowhere for its output.

// daemonProgram is the program and arguments the service runs. On Windows:
// berthd-bg.exe, the build without a console, beside this one; its home as
// --home, as the Run key has no environment; and its own log (--log).
func daemonProgram(b boxHome, exe string, args []string) (string, []string) {
	if runtime.GOOS != "windows" {
		return exe, args
	}
	home := filepath.Dir(b.dir)
	args = append(append([]string{"--home", home}, args...), "--log", filepath.Join(b.dir, "berthd.log"))
	return winsys.WindowlessTwin(exe), args
}

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

// takeHome takes a leading --home DIR as BERTH_HOME, for where no
// environment can be given: a Windows service is only a command line
// (install.go).
func takeHome(args []string) []string {
	if len(args) > 2 && args[0] == "--home" {
		os.Setenv("BERTH_HOME", args[1])
		return args[2:]
	}
	return args
}

// serveLog sends serve's log to path when one is given (serve --log): a
// Windows service has no output of its own.
func serveLog(path string) (func(), error) {
	if path == "" {
		return func() {}, nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	log.SetOutput(f)
	os.Stderr = f
	return func() { f.Close() }, nil
}

// notePID writes serve's pid to box/berthd.pid: install on Windows stops the
// berthd it replaces by it (stopServing).
func notePID(b boxHome) {
	statefile.Write(filepath.Join(b.dir, "berthd.pid"), []byte(strconv.Itoa(os.Getpid())))
}
