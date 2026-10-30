package main

import (
	"os"
	"path/filepath"
	"testing"
)

// The agent runs as the console berth-cli.exe beside berth-agent.exe, so
// the programs it starts share its hidden console rather than each opening
// a window.
func TestTheAgentRunsAsTheConsoleBerth(t *testing.T) {
	dir := t.TempDir()
	agent := filepath.Join(dir, "berth-agent.exe")
	os.WriteFile(agent, nil, 0o755)
	if got := consoleBerth(agent); got != agent {
		t.Fatalf("without berth-cli.exe: %s", got)
	}
	cli := filepath.Join(dir, "berth-cli.exe")
	os.WriteFile(cli, nil, 0o755)
	if got := consoleBerth(agent); got != cli {
		t.Fatalf("consoleBerth(%s) = %s, want %s", agent, got, cli)
	}
	if got := consoleBerth(cli); got != cli {
		t.Fatalf("a console berth stays itself: %s", got)
	}
}
