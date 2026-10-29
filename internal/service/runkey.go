package service

import (
	"fmt"
	"strings"
)

// On Windows a service is a value under the user's Run key
// (HKCU\Software\Microsoft\Windows\CurrentVersion\Run), the way desktop apps
// start at login. Windows runs it once at each logon; it doesn't restart a
// program that stops, so what it runs must be safe to run again (berth
// agent start does nothing while an agent runs, and the app starts one when
// it finds none). The file at unitPath holds the command line the value was
// set to, so Installed can compare it as on the other platforms. Run starts
// no environment or log of its own: Env and LogPath are the program's to
// handle. The program should be a GUI-subsystem build (berth-agent.exe),
// or Windows opens a console window for it at every logon.

const runKey = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`

// runValueName is the Run key's value for s.
func runValueName(s Spec) string { return s.Name }

// renderRun is the command line the Run value holds.
func renderRun(s Spec) ([]byte, error) {
	parts := make([]string, 0, len(s.Args)+1)
	parts = append(parts, winQuote(s.Program))
	for _, a := range s.Args {
		parts = append(parts, winQuote(a))
	}
	return []byte(strings.Join(parts, " ") + "\n"), nil
}

// winQuote quotes an argument the way Windows programs split their command
// line: only when it has to.
func winQuote(a string) string {
	if a != "" && !strings.ContainsAny(a, " \t\"") {
		return a
	}
	return `"` + strings.ReplaceAll(a, `"`, `\"`) + `"`
}

// loadRun sets the Run value and runs the program once now.
func loadRun(s Spec, data []byte) error {
	line := strings.TrimSpace(string(data))
	if out, err := command("reg", "add", runKey, "/v", runValueName(s), "/t", "REG_SZ", "/d", line, "/f"); err != nil {
		return fmt.Errorf("setting %s\\%s: %v: %s", runKey, runValueName(s), err, strings.TrimSpace(string(out)))
	}
	return startRun(s)
}

func unloadRun(s Spec) {
	command("reg", "delete", runKey, "/v", runValueName(s), "/f")
}

func runRegistered(s Spec) bool {
	_, err := command("reg", "query", runKey, "/v", runValueName(s))
	return err == nil
}

// startRun runs the program as the Run value would.
func startRun(s Spec) error {
	if out, err := command(s.Program, s.Args...); err != nil {
		return fmt.Errorf("starting %s: %v: %s", s.Name, err, strings.TrimSpace(string(out)))
	}
	return nil
}
