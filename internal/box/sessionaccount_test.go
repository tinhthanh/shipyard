package box

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// claudeAccountsHome makes a home with the default Claude folder and two
// signed-in accounts, work and other.
func claudeAccountsHome(t *testing.T) (home, work, other string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	work = filepath.Join(home, ".berth", "accounts", "claude", "work")
	other = filepath.Join(home, ".berth", "accounts", "claude", "other")
	for _, d := range []string{filepath.Join(home, ".claude"), work, other} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, ".claude.json"), []byte(`{"oauthAccount":{}}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return home, work, other
}

func TestAnAccountIsAskedForByFolderOrName(t *testing.T) {
	home, work, _ := claudeAccountsHome(t)
	for asked, want := range map[string]string{"work": work, "~/.berth/accounts/claude/work": work, work: work} {
		got, def, err := accountDirFor(home, asked)
		if err != nil || got != want || def {
			t.Errorf("%q = %q %v %v", asked, got, def, err)
		}
	}
	if got, def, err := accountDirFor(home, "default"); err != nil || !def || got != filepath.Join(home, ".claude") {
		t.Errorf("default = %q %v %v", got, def, err)
	}
	never := filepath.Join(home, ".berth", "accounts", "claude", "never")
	os.MkdirAll(never, 0o700)
	for _, bad := range []string{"nope", "/etc", "never"} {
		if _, _, err := accountDirFor(home, bad); err == nil {
			t.Errorf("%q was taken", bad)
		}
	}
}

func TestTheDefaultAccountIsTheVariableUnset(t *testing.T) {
	cmd, env := onAccount("claude --resume x", []string{"A=1"}, "/h/.claude", true)
	if cmd != "env -u CLAUDE_CONFIG_DIR claude --resume x" || !slices.Equal(env, []string{"A=1"}) {
		t.Fatalf("default: %q %v", cmd, env)
	}
	cmd, env = onAccount("claude --resume x", []string{"A=1"}, "/h/work", false)
	if cmd != "claude --resume x" || !slices.Equal(env, []string{"A=1", "CLAUDE_CONFIG_DIR=/h/work"}) {
		t.Fatalf("work: %q %v", cmd, env)
	}
}

func TestAConversationIsCopiedToTheOtherAccount(t *testing.T) {
	_, work, other := claudeAccountsHome(t)
	dir := t.TempDir()
	folder := claudeFolders(work, dir)
	src := filepath.Join(folder[len(folder)-1], "sid-1.jsonl")
	os.MkdirAll(filepath.Join(strings.TrimSuffix(src, ".jsonl"), "subagents"), 0o700)
	os.WriteFile(src, []byte("{\"type\":\"user\"}\n"), 0o600)
	os.WriteFile(filepath.Join(strings.TrimSuffix(src, ".jsonl"), "subagents", "a.jsonl"), []byte("{}\n"), 0o600)
	if err := copyClaudeConversation(work, other, dir, "sid-1"); err != nil {
		t.Fatal(err)
	}
	got := claudeFile(other, dir, "sid-1")
	if b, _ := os.ReadFile(got); string(b) != "{\"type\":\"user\"}\n" {
		t.Fatalf("copied %q: %q", got, b)
	}
	if _, err := os.Stat(filepath.Join(strings.TrimSuffix(got, ".jsonl"), "subagents", "a.jsonl")); err != nil {
		t.Fatal("its subagents weren't copied:", err)
	}
	// The old account keeps its own.
	if claudeFile(work, dir, "sid-1") == "" {
		t.Fatal("the source went")
	}
	if err := copyClaudeConversation(work, other, dir, "nope"); err == nil {
		t.Fatal("a conversation never saved was copied")
	}
}

// A Claude Code session in its terminal names its account, and moves to
// another one: its conversation copied there and Claude Code started again
// in the pane with that account and --resume; to the default, with the
// variable unset.
func TestATerminalSessionMovesToAnotherAccount(t *testing.T) {
	_, work, other := claudeAccountsHome(t)
	var bx *Box
	states := &Turns{}
	c, bus := servedBox(t, func(b *Box) { bx = b; b.Turns = states })
	states.Attach(bus)
	if caps := bx.Capabilities(); !slices.Contains(caps, "session.account.switch") {
		t.Fatalf("capabilities = %v", caps)
	}
	repo := gitRepo(t)
	call(t, c, "POST", "/v1/locations", "", map[string]string{"name": "acme", "path": repo}, nil)
	// A stand-in for Claude Code that says how it was started.
	seen := filepath.Join(t.TempDir(), "seen")
	fake := filepath.Join(t.TempDir(), "claude")
	os.WriteFile(fake, []byte("#!/bin/sh\necho \"${CLAUDE_CONFIG_DIR-unset}|$*\" >> "+seen+"\nexec sleep 60\n"), 0o755)
	if status := call(t, c, "PUT", "/v1/locations/acme/config", "", map[string]any{"local": map[string]any{"agents": []map[string]string{{"id": "claude", "name": "Claude Code", "command": fake}}}}, nil); status != 200 {
		t.Fatalf("config: %d", status)
	}
	var sess Session
	if status := call(t, c, "POST", "/v1/sessions", "", map[string]any{"location": "acme", "agent": "claude", "accounts": map[string]string{"claude": work}}, &sess); status != 200 {
		t.Fatalf("start: %d", status)
	}
	t.Cleanup(func() { call(t, c, "DELETE", "/v1/sessions/"+sess.Name, "", nil, nil) })
	lines := func(n int) []string {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for {
			b, _ := os.ReadFile(seen)
			if l := strings.Split(strings.TrimSpace(string(b)), "\n"); len(b) > 0 && len(l) >= n {
				return l
			}
			if time.Now().After(deadline) {
				t.Fatalf("Claude Code started %d times, not %d", strings.Count(string(b), "\n"), n)
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	if l := lines(1); !strings.HasPrefix(l[0], work+"|") {
		t.Fatalf("started with %q", l[0])
	}
	account := func() string {
		t.Helper()
		var all []Session
		call(t, c, "GET", "/v1/sessions", "", nil, &all)
		for _, s := range all {
			if s.Name == sess.Name {
				return s.Account
			}
		}
		return "?"
	}
	if got := account(); got != "work" {
		t.Fatalf("account = %q", got)
	}

	// Nothing to carry on before Claude Code names its conversation.
	if status := call(t, c, "POST", "/v1/sessions/"+sess.Name+"/account", "", map[string]string{"account": "other"}, nil); status != 409 {
		t.Fatalf("before its conversation: %d", status)
	}
	// Its hooks name the conversation; its transcript is in work's folder.
	call(t, c, "POST", "/v1/events", "", map[string]any{"type": "agent.finished", "data": map[string]any{"session": sess.Name, "agent": "claude", "agent_session_id": "sid-1", "path": sess.Dir}}, nil)
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		if st, _ := states.State(sess.Name); st.AgentSessionID == "sid-1" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the hook's conversation was never seen")
		}
	}
	folders := claudeFolders(work, sess.Dir)
	os.MkdirAll(folders[len(folders)-1], 0o700)
	os.WriteFile(filepath.Join(folders[len(folders)-1], "sid-1.jsonl"), []byte("{\"type\":\"user\"}\n"), 0o600)

	var moved Session
	if status := call(t, c, "POST", "/v1/sessions/"+sess.Name+"/account", "", map[string]string{"account": "other"}, &moved); status != 200 {
		var why map[string]any
		call(t, c, "POST", "/v1/sessions/"+sess.Name+"/account", "", map[string]string{"account": "other"}, &why)
		t.Fatalf("move: %d %v", status, why)
	}
	if moved.Account != "other" {
		t.Fatalf("moved to %q", moved.Account)
	}
	if l := lines(2); l[1] != other+"|--resume sid-1" {
		t.Fatalf("started again with %q", l[1])
	}
	if claudeFile(other, sess.Dir, "sid-1") == "" {
		t.Fatal("the conversation wasn't copied to other")
	}
	if status := call(t, c, "POST", "/v1/sessions/"+sess.Name+"/account", "", map[string]string{"account": "other"}, nil); status != 409 {
		t.Fatalf("to the account it is on: %d", status)
	}

	// To the default: the variable unset, not ~/.claude.
	if status := call(t, c, "POST", "/v1/sessions/"+sess.Name+"/account", "", map[string]string{"account": "default"}, &moved); status != 200 {
		t.Fatalf("to default: %d", status)
	}
	if l := lines(3); l[2] != "unset|--resume sid-1" {
		t.Fatalf("on default, started with %q", l[2])
	}
	if got := account(); got != "default" {
		t.Fatalf("account = %q", got)
	}
}
