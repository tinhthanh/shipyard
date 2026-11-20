package box

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestAPickedAccountMustBeOneOfTheBoxs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	work := filepath.Join(home, ".berth", "accounts", "claude", "work")
	if err := os.MkdirAll(work, 0o700); err != nil {
		t.Fatal(err)
	}
	b := &Box{}
	r := httptest.NewRequest("POST", "/v1/sessions", nil)

	same, err := b.withAccounts(r, nil)
	if err != nil || same != r || accountEnv(same.Context()) != nil {
		t.Fatalf("no pick: %v %v", err, accountEnv(same.Context()))
	}
	got, err := b.withAccounts(r, map[string]string{"claude": "~/.berth/accounts/claude/work"})
	if err != nil {
		t.Fatal(err)
	}
	if env := accountEnv(got.Context()); len(env) != 1 || env[0] != "CLAUDE_CONFIG_DIR="+work {
		t.Fatalf("env = %v", env)
	}
	for _, bad := range []map[string]string{
		{"claude": "~/.berth/accounts/claude/nope"},
		{"claude": "/etc"},
		{"gemini": "~/.gemini"},
	} {
		if _, err := b.withAccounts(r, bad); err == nil {
			t.Errorf("%v was taken", bad)
		}
	}
}

// A task started with an account of its own runs with it, over the one
// its project picks for new sessions.
func TestATaskRunsWithTheAccountItPicks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	work := filepath.Join(home, ".berth", "accounts", "claude", "work")
	other := filepath.Join(home, ".berth", "accounts", "claude", "other")
	for _, d := range []string{work, other} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	var bx *Box
	c, _ := servedBox(t, func(b *Box) { bx = b })
	// The app offers the picker only to a box that says it takes accounts.
	if caps := bx.Capabilities(); !slices.Contains(caps, "task.accounts") {
		t.Fatalf("capabilities = %v", caps)
	}
	repo := gitRepo(t)
	call(t, c, "POST", "/v1/locations", "", map[string]string{"name": "acme", "path": repo}, nil)
	// The project's own pick, which the task's overrides.
	var cfg map[string]any
	if status := call(t, c, "PUT", "/v1/locations/acme/config", "", map[string]any{"local": map[string]any{"env": map[string]string{"CLAUDE_CONFIG_DIR": other}}}, &cfg); status != 200 {
		t.Fatalf("config: %d", status)
	}
	// seen starts a session that writes the account it sees, and reads it.
	seen := func(name string, accounts map[string]string) string {
		t.Helper()
		out := filepath.Join(t.TempDir(), "seen")
		req := map[string]any{"name": name, "location": "acme", "command": "printenv CLAUDE_CONFIG_DIR > " + out + "; sleep 30"}
		if accounts != nil {
			req["accounts"] = accounts
		}
		var sess Session
		if status := call(t, c, "POST", "/v1/sessions", "", req, &sess); status != 200 {
			t.Fatalf("start %s: %d", name, status)
		}
		t.Cleanup(func() { call(t, c, "DELETE", "/v1/sessions/"+sess.Name, "", nil, nil) })
		deadline := time.Now().Add(10 * time.Second)
		for {
			b, _ := os.ReadFile(out)
			if got := strings.TrimSpace(string(b)); got != "" {
				return got
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s never ran", name)
			}
			time.Sleep(50 * time.Millisecond)
		}
	}

	if status := call(t, c, "POST", "/v1/sessions", "", map[string]any{"location": "acme", "command": "true", "accounts": map[string]string{"claude": "~/.berth/accounts/claude/nope"}}, nil); status != 400 {
		t.Fatalf("an unknown account: %d", status)
	}
	if got := seen("follows", nil); got != other {
		t.Fatalf("without a pick, CLAUDE_CONFIG_DIR = %q, want the project's %q", got, other)
	}
	if got := seen("picks", map[string]string{"claude": "~/.berth/accounts/claude/work"}); got != work {
		t.Fatalf("with a pick, CLAUDE_CONFIG_DIR = %q, want %q", got, work)
	}
}
