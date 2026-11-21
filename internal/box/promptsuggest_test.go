package box

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// The stand-in Claudes of the other tests know no --help: asking them
// would start one. Tests that want the real check swap it back.
var realClaudeSuggests = claudeSuggests

func init() { claudeSuggests = func(string) bool { return false } }

func TestBerthdAsksForSuggestionsOnlyFromAClaudeThatTakesTheFlag(t *testing.T) {
	write := func(help string) string {
		dir := t.TempDir()
		p := filepath.Join(dir, "claude")
		os.WriteFile(p, []byte("#!/bin/sh\necho 'Usage: claude [options]'\necho '"+help+"'\n"), 0o755)
		return p
	}
	newer := write("  --prompt-suggestions [value]   Turn on prompt suggestions.")
	older := write("  --permission-mode <mode>        Permission mode.")
	if !realClaudeSuggests(newer + " --model opus") {
		t.Error("a Claude Code that takes --prompt-suggestions wasn't asked")
	}
	if realClaudeSuggests(older) {
		t.Error("a Claude Code too old for --prompt-suggestions was asked: it would refuse to start")
	}
	if realClaudeSuggests("codex") {
		t.Error("only Claude Code is asked")
	}
}

// suggestingClaude stands in for `claude -p` speaking stream-json: it
// echoes each prompt and, asked with --prompt-suggestions, guesses the
// next one after the turn's result. "slow" works a while first.
const suggestingClaude = `#!/usr/bin/env python3
import json, sys, time
S = "00000000-0000-4000-8000-0000000000d1"
suggest = "--prompt-suggestions" in sys.argv
n = [0]
def out(d):
    d.setdefault("session_id", S)
    print(json.dumps(d), flush=True)
def uid():
    n[0] += 1
    return "00000000-0000-4000-8000-%012d" % n[0]
for line in sys.stdin:
    m = json.loads(line)
    if m["type"] == "control_request":
        out({"type": "control_response", "response": {"subtype": "success", "request_id": m["request_id"], "response": {}}})
        continue
    text = m["message"]["content"]
    out({"type": "user", "message": m["message"], "isReplay": True, "parent_tool_use_id": None, "uuid": uid()})
    if text == "slow":
        time.sleep(2)
    out({"type": "assistant", "message": {"id": "msg_" + uid(), "role": "assistant", "content": [{"type": "text", "text": "echo: " + text}]}, "parent_tool_use_id": None, "uuid": uid()})
    out({"type": "result", "subtype": "success", "is_error": False, "result": ""})
    if suggest:
        time.sleep(0.5)
        out({"type": "prompt_suggestion", "suggestion": "  after " + text + "  ", "uuid": uid()})
`

func TestAStructuredSessionOffersTheNextPromptClaudeGuesses(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("the stand-in Claude needs python3")
	}
	claudeSuggests = func(string) bool { return true }
	t.Cleanup(func() { claudeSuggests = func(string) bool { return false } })
	bin := t.TempDir()
	os.WriteFile(filepath.Join(bin, "claude"), []byte(suggestingClaude), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var bx *Box
	c, _ := servedBox(t, func(b *Box) {
		bx = b
		b.ConversationsDir = conversationsDir(t)
		b.AgentHost = []string{"env", "BERTH_TEST_AGENT_HOST=1", exe}
	})
	if !slices.Contains(bx.Capabilities(), "prompt.suggestions") {
		t.Fatalf("capabilities don't list prompt.suggestions: %v", bx.Capabilities())
	}
	repo := gitRepo(t)
	call(t, c, "POST", "/v1/locations", "", map[string]string{"name": "acme", "path": repo}, nil)
	var sess Session
	if status := call(t, c, "POST", "/v1/sessions", "", map[string]any{"location": "acme", "agent": "claude", "driver": "claude", "prompt": "hello"}, &sess); status != 200 {
		t.Fatalf("start: %d", status)
	}
	t.Cleanup(func() { call(t, c, "DELETE", "/v1/sessions/"+sess.Name, "", nil, nil) })

	suggestion := func(query string) string {
		t.Helper()
		var out struct {
			Suggestion string `json:"suggestion"`
		}
		if status := call(t, c, "GET", "/v1/sessions/"+sess.Name+"/suggestion"+query, "", nil, &out); status != 200 {
			t.Fatalf("suggestion: %d", status)
		}
		return out.Suggestion
	}
	// It comes once the turn is over (the stand-in guesses only when
	// started with --prompt-suggestions); waiting for it returns it.
	deadline := time.Now().Add(15 * time.Second)
	for {
		if s := suggestion("?wait=5s"); s == "after hello" {
			break
		} else if time.Now().After(deadline) {
			t.Fatalf("the suggestion never came: %q", s)
		}
	}
	// A new turn takes it away until its own comes.
	call(t, c, "POST", "/v1/sessions/"+sess.Name+"/send", "", map[string]any{"text": "slow", "when": "now"}, nil)
	deadline = time.Now().Add(10 * time.Second)
	for suggestion("") != "" {
		if time.Now().After(deadline) {
			t.Fatal("the last turn's suggestion stayed while the agent worked")
		}
		time.Sleep(50 * time.Millisecond)
	}
	deadline = time.Now().Add(15 * time.Second)
	for {
		if s := suggestion("?wait=5s"); s == "after slow" {
			break
		} else if time.Now().After(deadline) {
			t.Fatalf("the next turn's suggestion never came: %q", s)
		}
	}
	if status := call(t, c, "GET", "/v1/sessions/"+sess.Name+"/suggestion?wait=soon", "", nil, nil); status != 400 {
		t.Fatalf("a bad wait: %d", status)
	}
}
