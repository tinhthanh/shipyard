package winsys

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConsoleTwinIsTheConsoleBuildBesideIt(t *testing.T) {
	dir := t.TempDir()
	agent := filepath.Join(dir, "berth-agent.exe")
	os.WriteFile(agent, nil, 0o755)
	if got := ConsoleTwin(agent); got != agent {
		t.Fatalf("without berth-cli.exe: %s", got)
	}
	cli := filepath.Join(dir, "berth-cli.exe")
	os.WriteFile(cli, nil, 0o755)
	if got := ConsoleTwin(agent); got != cli {
		t.Fatalf("ConsoleTwin(%s) = %s, want %s", agent, got, cli)
	}
	if got := ConsoleTwin(cli); got != cli {
		t.Fatalf("a console build stays itself: %s", got)
	}
}
