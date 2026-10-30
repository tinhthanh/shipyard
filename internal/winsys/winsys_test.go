package winsys

import "testing"

func TestTheConsoleBuildOfAProgramWithoutOne(t *testing.T) {
	for in, want := range map[string]string{
		"berth-agent.exe": "berth-cli.exe",
		"BERTHD-BG.EXE":   "berthd.exe",
		"berth-cli.exe":   "",
		"berthd.exe":      "",
		"berth":           "",
	} {
		if got := twinName(in); got != want {
			t.Errorf("twinName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTheBuildWithoutAConsoleOfAConsoleProgram(t *testing.T) {
	for in, want := range map[string]string{
		"berth-cli.exe":   "berth-agent.exe",
		"BERTHD.EXE":      "berthd-bg.exe",
		"berth-agent.exe": "",
		"berth":           "",
	} {
		if got := windowlessName(in); got != want {
			t.Errorf("windowlessName(%q) = %q, want %q", in, got, want)
		}
	}
}
