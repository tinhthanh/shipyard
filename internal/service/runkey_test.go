package service

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// On Windows the service is a value under the user's Run key: Install sets
// it to the program's command line and runs the program once; Uninstall
// deletes it. No scheduled task, nothing hidden, no proxy program.
func TestAWindowsServiceIsARunValue(t *testing.T) {
	home := t.TempDir()
	saved := []any{goos, homeDir, command, spawn}
	defer func() {
		goos, homeDir, command = saved[0].(string), saved[1].(func() (string, error)), saved[2].(func(string, ...string) ([]byte, error))
		spawn = saved[3].(func(string, ...string) error)
	}()
	goos = "windows"
	homeDir = func() (string, error) { return home, nil }
	var ran [][]string
	command = func(name string, args ...string) ([]byte, error) {
		ran = append(ran, append([]string{name}, args...))
		return nil, nil
	}
	// Started and left running, not waited for: berthd serve never ends.
	spawn = func(name string, args ...string) error {
		ran = append(ran, append([]string{"spawn", name}, args...))
		return nil
	}
	s := Spec{Name: "berth-agent", Description: "berth agent", Program: "/opt/Vetgo App/berth-agent.exe", Args: []string{"agent", "start"}}
	data, err := Render(s)
	if err != nil || string(data) != "\"/opt/Vetgo App/berth-agent.exe\" agent start\n" {
		t.Fatalf("command line %q, %v", data, err)
	}
	path, err := Install(s)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != filepath.Join(home, "AppData", "Local", "berth", "services") || !Installed(s) {
		t.Fatalf("installed at %s, Installed=%v", path, Installed(s))
	}
	want := [][]string{
		{"reg", "add", runKey, "/v", "berth-agent", "/t", "REG_SZ", "/d", "\"/opt/Vetgo App/berth-agent.exe\" agent start", "/f"},
		{"spawn", "/opt/Vetgo App/berth-agent.exe", "agent", "start"},
	}
	if fmt.Sprint(ran) != fmt.Sprint(want) {
		t.Fatalf("ran %q, want %q", ran, want)
	}
	for _, c := range ran {
		for _, a := range c {
			if strings.Contains(strings.ToLower(a), "schtasks") || strings.Contains(strings.ToLower(a), "conhost") {
				t.Fatalf("a Windows service must not go through %q", a)
			}
		}
	}
	ran = nil
	if _, err := Uninstall(s); err != nil {
		t.Fatal(err)
	}
	if len(ran) != 1 || ran[0][1] != "delete" {
		t.Fatalf("uninstall ran %q", ran)
	}
}
