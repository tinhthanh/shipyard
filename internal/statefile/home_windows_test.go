package statefile

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheStateLivesInLocalAppDataOnWindows(t *testing.T) {
	local, roaming := t.TempDir(), t.TempDir()
	t.Setenv("BERTH_HOME", "")
	t.Setenv("LOCALAPPDATA", local)
	t.Setenv("APPDATA", roaming)
	got, err := Home()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(local, "berth"); got != want {
		t.Fatalf("Home() = %s, want %s", got, want)
	}
}

func TestAStateInAppDataMovesToLocalAppData(t *testing.T) {
	local, roaming := t.TempDir(), t.TempDir()
	t.Setenv("BERTH_HOME", "")
	t.Setenv("LOCALAPPDATA", local)
	t.Setenv("APPDATA", roaming)
	old := filepath.Join(roaming, "berth")
	os.MkdirAll(filepath.Join(old, "client"), 0o700)
	os.WriteFile(filepath.Join(old, "client", "ui-token"), []byte("t"), 0o600)
	// The service file the agent installed already lives in the new place.
	os.MkdirAll(filepath.Join(local, "berth", "services"), 0o700)

	got, err := Home()
	if err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(got, "client", "ui-token")); err != nil || string(b) != "t" {
		t.Fatalf("token after the move: %q, %v (home %s)", b, err, got)
	}
	if _, err := os.Stat(filepath.Join(got, "services")); err != nil {
		t.Fatalf("services went: %v", err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("the old folder is still there: %v", err)
	}
}

func TestAStateHeldOpenStaysWhereItIs(t *testing.T) {
	local, roaming := t.TempDir(), t.TempDir()
	t.Setenv("BERTH_HOME", "")
	t.Setenv("LOCALAPPDATA", local)
	t.Setenv("APPDATA", roaming)
	old := filepath.Join(roaming, "berth")
	os.MkdirAll(filepath.Join(old, "client"), 0o700)
	// A running agent from before the move holds its files open, which
	// Windows won't let a folder move under.
	f, err := os.Create(filepath.Join(old, "client", "agent.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err := Home()
	if err != nil {
		t.Fatal(err)
	}
	if got != old {
		t.Fatalf("Home() = %s while the old state is in use, want %s", got, old)
	}
}

func TestFilesWrittenBeforeTheStateWasPrivateBecomeOwnerOnly(t *testing.T) {
	local, roaming := t.TempDir(), t.TempDir()
	t.Setenv("BERTH_HOME", "")
	t.Setenv("LOCALAPPDATA", local)
	t.Setenv("APPDATA", roaming)
	old := filepath.Join(roaming, "berth", "client")
	os.MkdirAll(old, 0o700)
	// An identity key from an older berth: its folder's access list, no own.
	if err := os.WriteFile(filepath.Join(old, "identity.pem"), []byte("k"), 0o600); err != nil {
		t.Fatal(err)
	}
	home, err := Home()
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("icacls", filepath.Join(home, "client", "identity.pem")).CombinedOutput()
	if err != nil {
		t.Fatalf("icacls: %v %s", err, out)
	}
	// Only the access entries: the path itself has "Users" in it.
	got := strings.ReplaceAll(string(out), filepath.Join(home, "client", "identity.pem"), "")
	if strings.Contains(got, "Administrators") || strings.Contains(got, `BUILTIN\Users`) || strings.Contains(got, "Everyone") {
		t.Fatalf("the key is open to more than its owner:\n%s", got)
	}
	if !strings.Contains(got, "SYSTEM") {
		t.Fatalf("unexpected access list:\n%s", got)
	}
}
