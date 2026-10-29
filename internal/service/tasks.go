package service

import (
	"bytes"
	"fmt"
	"os/user"
	"path/filepath"
	"strings"
	"unicode/utf16"
)

// On Windows a service is a Task Scheduler task in the berth folder, made
// from an XML file beside berth's state (unitPath). It runs at the user's
// logon, in their desktop session, and again every few minutes, so a
// program that stops comes back: what it runs must therefore be quick and
// do nothing when the work is already up (berth agent start). Task
// Scheduler sets no environment and keeps no log for a task: Env and
// LogPath are left to the program. It runs through conhost --headless, so
// a console program opens no window.

// currentUser is replaced in tests.
var currentUser = func() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	return u.Username, nil
}

func taskName(s Spec) string { return `\berth\` + s.Name }

// renderTask is the task's XML, UTF-16 with a byte order mark as
// schtasks /xml wants it.
func renderTask(s Spec) ([]byte, error) {
	who, err := currentUser()
	if err != nil {
		return nil, err
	}
	args := make([]string, 0, len(s.Args)+2)
	args = append(args, "--headless", winQuote(s.Program))
	for _, a := range s.Args {
		args = append(args, winQuote(a))
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
`)
	fmt.Fprintf(&b, "    <Description>%s</Description>\n", esc(s.Description))
	b.WriteString(`  </RegistrationInfo>
  <Triggers>
    <LogonTrigger>
      <Enabled>true</Enabled>
`)
	fmt.Fprintf(&b, "      <UserId>%s</UserId>\n", esc(who))
	b.WriteString(`      <Repetition>
        <Interval>PT5M</Interval>
        <StopAtDurationEnd>false</StopAtDurationEnd>
      </Repetition>
    </LogonTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
`)
	fmt.Fprintf(&b, "      <UserId>%s</UserId>\n", esc(who))
	b.WriteString(`      <LogonType>InteractiveToken</LogonType>
      <RunLevel>LeastPrivilege</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <StartWhenAvailable>true</StartWhenAvailable>
    <ExecutionTimeLimit>PT10M</ExecutionTimeLimit>
    <Hidden>true</Hidden>
    <Enabled>true</Enabled>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>conhost.exe</Command>
`)
	fmt.Fprintf(&b, "      <Arguments>%s</Arguments>\n", esc(strings.Join(args, " ")))
	b.WriteString(`    </Exec>
  </Actions>
</Task>
`)
	return utf16LE(strings.ReplaceAll(b.String(), "\n", "\r\n")), nil
}

// winQuote quotes an argument the way Windows programs split their command
// line: only when it has to.
func winQuote(a string) string {
	if a != "" && !strings.ContainsAny(a, " \t\"") {
		return a
	}
	return `"` + strings.ReplaceAll(a, `"`, `\"`) + `"`
}

func utf16LE(s string) []byte {
	u := utf16.Encode([]rune(s))
	var b bytes.Buffer
	b.Write([]byte{0xFF, 0xFE})
	for _, c := range u {
		b.WriteByte(byte(c))
		b.WriteByte(byte(c >> 8))
	}
	return b.Bytes()
}

// loadTask registers the task from its file, replacing one of the same name,
// and runs it once now.
func loadTask(s Spec, path string) error {
	if out, err := command("schtasks", "/create", "/tn", taskName(s), "/xml", filepath.Clean(path), "/f"); err != nil {
		return fmt.Errorf("registering %s: %v: %s", taskName(s), err, strings.TrimSpace(string(out)))
	}
	if out, err := command("schtasks", "/run", "/tn", taskName(s)); err != nil {
		return fmt.Errorf("starting %s: %v: %s", taskName(s), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func unloadTask(s Spec) {
	command("schtasks", "/end", "/tn", taskName(s))
	command("schtasks", "/delete", "/tn", taskName(s), "/f")
}

func taskRegistered(s Spec) bool {
	_, err := command("schtasks", "/query", "/tn", taskName(s))
	return err == nil
}
