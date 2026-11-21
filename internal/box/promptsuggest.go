package box

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Prompt suggestions (fork): after each turn Claude Code guesses what the
// person will ask next ("run it"), as its own screen shows in grey and
// Tab takes. A structured session asks for them with --prompt-suggestions,
// and Claude Code then writes a {"type":"prompt_suggestion","suggestion":…}
// line once a turn's result is out. The driver keeps the latest
// (noteSuggestion), and the chat reads it:
//
//	GET /v1/sessions/{name}/suggestion?wait=10s
//
// answers {"suggestion": "…"}, empty while the agent works or has none.
// With wait, an idle session with none yet waits that long for one: it
// comes a second or so after the turn ends.

// promptSuggestionCaps says a box answers GET .../suggestion.
var promptSuggestionCaps = []string{"prompt.suggestions"}

// maxSuggestion bounds what is kept: a suggestion is a line or two.
const maxSuggestion = 2000

// suggestFlags are the flags that ask the preset's Claude Code for
// suggestions: none from a Claude Code too old to know the flag, which
// would refuse to start.
func suggestFlags(command string) []string {
	if claudeSuggests(command) {
		return []string{"--prompt-suggestions"}
	}
	return nil
}

// claudeSuggests says whether command runs a Claude Code that takes
// --prompt-suggestions: its --help says so, read once per binary. A test
// sets it.
var claudeSuggests = func(command string) bool {
	words := strings.Fields(command)
	if len(words) == 0 || filepath.Base(words[0]) != "claude" {
		return false
	}
	path := words[0]
	if !filepath.IsAbs(path) {
		f, ok := agentFound(path)
		if !ok {
			return false
		}
		path = f.Path
	}
	st, err := os.Stat(path)
	if err != nil {
		return false
	}
	key := path + "\x00" + st.ModTime().String()
	claudeHelp.Lock()
	defer claudeHelp.Unlock()
	if v, ok := claudeHelp.seen[key]; ok {
		return v
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(ctx, path, "--help").CombinedOutput()
	v := bytes.Contains(out, []byte("--prompt-suggestions"))
	if claudeHelp.seen == nil {
		claudeHelp.seen = map[string]bool{}
	}
	claudeHelp.seen[key] = v
	return v
}

var claudeHelp struct {
	sync.Mutex
	seen map[string]bool
}

// noteSuggestion keeps what a stream line says about the next prompt: a
// suggestion, or (a turn ending, before its own comes) none. Called with
// c.mu held.
func (c *claudeConv) noteSuggestion(l streamLine) {
	switch {
	case l.Type == "prompt_suggestion":
		s := strings.TrimSpace(l.Suggestion)
		if len(s) > maxSuggestion {
			s = ""
		}
		c.nextAsk = s
	case l.Type == "result", l.Type == "user" && l.IsReplay:
		c.nextAsk = ""
	}
}

// suggestionNow is the suggestion the chat may show: none while the agent
// works, waits for a person, or has ended. Called with c.mu held.
func (c *claudeConv) suggestionNow() string {
	if c.busy || c.exited || len(c.pending) > 0 {
		return ""
	}
	return c.nextAsk
}

// maxSuggestionWait bounds ?wait.
const maxSuggestionWait = 30 * time.Second

func (b *Box) suggestion(w http.ResponseWriter, r *http.Request) error {
	sess, err := b.Sessions.Get(r.Context(), r.PathValue("name"))
	if err != nil {
		return err
	}
	out := map[string]string{"suggestion": ""}
	if sess.Driver != DriverClaude {
		writeJSON(w, out)
		return nil
	}
	c, err := b.claude().conv(sess)
	if err != nil {
		writeJSON(w, out)
		return nil
	}
	var wait time.Duration
	if v := r.URL.Query().Get("wait"); v != "" {
		if wait, err = time.ParseDuration(v); err != nil || wait < 0 {
			return badRequest("wait is a duration, such as 10s")
		}
		wait = min(wait, maxSuggestionWait)
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	for {
		c.mu.Lock()
		s, busy, changed := c.suggestionNow(), c.busy || c.exited || len(c.pending) > 0, c.changed
		c.mu.Unlock()
		if s != "" || busy || wait == 0 {
			out["suggestion"] = s
			writeJSON(w, out)
			return nil
		}
		select {
		case <-changed:
		case <-timer.C:
			writeJSON(w, out)
			return nil
		case <-r.Context().Done():
			return nil
		}
	}
}
