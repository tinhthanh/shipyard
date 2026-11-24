package box

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cosscom/shipyard/internal/agenthost"
	"github.com/cosscom/shipyard/internal/integrations"
)

// Which account a session's agent signs in with, and moving a Claude
// conversation to another account (the fork's).
//
// Session.Account is the account folder's name ("thanh-5x"), or "default"
// for the agent's own folder: from the session's CLAUDE_CONFIG_DIR or
// CODEX_HOME (enrich calls sessionAccount).
//
// POST /v1/sessions/{name}/account {"account": "<folder or name>"} carries
// a Claude conversation on with another account, when the one it runs on
// reached a limit, say. Claude Code keeps a conversation's transcript under
// its account folder (projects/<dir>/<id>.jsonl) and --resume looks only
// there, so the transcript is copied to the other account's folder, the
// agent stopped, and started again in the same pane with the other
// account and --resume: in its chat (a structured session) or in Claude
// Code's own terminal. The old account keeps its copy.
//
// The default account is the variable unset, not set to ~/.claude: Claude
// Code keeps a folder's login in the macOS keychain under a name made from
// the variable, so ~/.claude named outright is a login it doesn't have.

// sessionAccountCaps says a box names each session's account and moves a
// conversation to another.
var sessionAccountCaps = []string{"session.account", "session.account.switch"}

// sessionAccount is the account folder s's agent uses: its name, "default"
// for the agent's own, "" for no agent with accounts.
func (b *Box) sessionAccount(ctx context.Context, s Session) string {
	key, ok := integrations.AccountVars[s.Agent]
	if !ok || b.Sessions == nil {
		return ""
	}
	dir := b.Sessions.EnvVar(ctx, s, key)
	if dir == "" {
		return "default"
	}
	// Every list asks it of every agent: what a folder is called is kept.
	if v, ok := accountNames.Load(s.Agent + "\x00" + dir); ok {
		return v.(string)
	}
	home, _ := os.UserHomeDir()
	clean := filepath.Clean(integrations.ExpandAccountDir(home, dir))
	name := filepath.Base(clean)
	if all := integrations.Accounts(home, s.Agent); len(all) > 0 && sameAccountDir(clean, all[0].Dir) {
		name = "default"
	}
	accountNames.Store(s.Agent+"\x00"+dir, name)
	return name
}

var accountNames sync.Map

func sameAccountDir(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}

// AccountSwitchRequest moves a session's conversation to another account:
// the folder (or its name, or "default"). Force stops an agent at work.
type AccountSwitchRequest struct {
	Account string `json:"account"`
	Force   bool   `json:"force,omitempty"`
}

func (b *Box) switchAccount(w http.ResponseWriter, r *http.Request) error {
	var req AccountSwitchRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	ctx := r.Context()
	name := r.PathValue("name")
	sess, err := b.Sessions.Get(ctx, name)
	if err != nil {
		return err
	}
	sess = b.enrich(ctx, []Session{sess})[0]
	if sess.Agent != "claude" {
		return badRequest("only a Claude Code session moves to another account")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	target, isDefault, err := accountDirFor(home, req.Account)
	if err != nil {
		return err
	}
	from := b.claudeHomeFor(ctx, sess)
	if sameAccountDir(from, target) {
		return httpError{http.StatusConflict, name + " already uses " + accountLabel(target, isDefault)}
	}
	if b.Turns != nil && !req.Force {
		if st, ok := b.Turns.State(name); ok && st.State == "running" {
			return httpError{http.StatusConflict, "the agent is at work: stop it or let it finish first"}
		}
	}
	structured := sess.Driver == DriverClaude && sess.conv != ""
	sid := b.claudeConversationID(sess)
	if sid == "" {
		return httpError{http.StatusConflict, "berthd doesn't know which Claude conversation this is yet: send it something first"}
	}
	if err := b.before(r, "session.account", map[string]any{"name": name, "account": target}); err != nil {
		return err
	}
	if err := copyClaudeConversation(from, target, sess.Dir, sid); err != nil {
		return err
	}
	if err := b.setSessionAccount(ctx, sess, target, isDefault); err != nil {
		return err
	}
	if structured {
		err = b.resumeOnAccount(r, sess, sid, target, isDefault)
	} else {
		err = b.restartTerminalOnAccount(r, sess, sid, target, isDefault)
	}
	if err != nil {
		return err
	}
	b.publish(r, "session.account", map[string]any{"name": name, "account": accountLabel(target, isDefault), "from": filepath.Base(from)})
	sess, err = b.Sessions.Get(ctx, name)
	if err != nil {
		return err
	}
	writeJSON(w, b.enrich(ctx, []Session{sess})[0])
	return nil
}

// accountDirFor is the Claude account folder asked for: one of the box's
// (integrations.Accounts), by folder or name; isDefault for the agent's own.
func accountDirFor(home, asked string) (dir string, isDefault bool, err error) {
	accounts := integrations.Accounts(home, "claude")
	if len(accounts) == 0 {
		return "", false, badRequest("this box has no Claude Code accounts")
	}
	if asked == "" || asked == "default" {
		return accounts[0].Dir, true, nil
	}
	want := filepath.Clean(integrations.ExpandAccountDir(home, asked))
	for i, a := range accounts {
		if sameAccountDir(a.Dir, want) || (!strings.ContainsRune(asked, filepath.Separator) && filepath.Base(a.Dir) == asked && i > 0) {
			if i == 0 {
				return a.Dir, true, nil
			}
			if _, err := os.Stat(filepath.Join(a.Dir, ".claude.json")); err != nil {
				return "", false, badRequest("%s has never been signed in to", filepath.Base(a.Dir))
			}
			return a.Dir, false, nil
		}
	}
	return "", false, badRequest("%s is not one of Claude Code's accounts on this box", asked)
}

func accountLabel(dir string, isDefault bool) string {
	if isDefault {
		return "default"
	}
	return filepath.Base(dir)
}

// claudeConversationID is the Claude conversation sess carries on: from
// its agent's hooks, else its chat's log.
func (b *Box) claudeConversationID(sess Session) string {
	if b.Turns != nil {
		if st, ok := b.Turns.State(sess.Name); ok && st.AgentSessionID != "" {
			return st.AgentSessionID
		}
	}
	if sess.conv != "" {
		if sid := conversationSession(sess.conv); sid != "" {
			return sid
		}
		return lastSessionID(filepath.Join(sess.conv, agenthost.LogName))
	}
	return ""
}

// copyClaudeConversation copies conversation sid in dir from one account's
// folder to another's: its transcript and the folder beside it (its
// subagents' and tool results), replacing an older copy there.
func copyClaudeConversation(from, to, dir, sid string) error {
	src := claudeFile(from, dir, sid)
	if src == "" {
		return httpError{http.StatusConflict, "Claude hasn't saved this conversation yet: send it something first"}
	}
	dest := filepath.Join(to, "projects", filepath.Base(filepath.Dir(src)))
	if err := os.MkdirAll(dest, 0o700); err != nil {
		return err
	}
	if err := copyFile(src, filepath.Join(dest, sid+".jsonl")); err != nil {
		return err
	}
	side := strings.TrimSuffix(src, ".jsonl")
	if st, err := os.Stat(side); err == nil && st.IsDir() {
		return copyTree(side, filepath.Join(dest, sid))
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".berth-tmp"
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		return copyFile(path, target)
	})
}

// setSessionAccount records the account in the session's environment, so
// what reads it (EnvVar: its transcript, its account) finds the new one.
func (b *Box) setSessionAccount(ctx context.Context, sess Session, dir string, isDefault bool) error {
	args := []string{"set-environment", "-t", "=" + sess.Name, "CLAUDE_CONFIG_DIR", dir}
	if isDefault {
		args = []string{"set-environment", "-u", "-t", "=" + sess.Name, "CLAUDE_CONFIG_DIR"}
	}
	if out, err := b.Sessions.tmux(ctx, args...); err != nil {
		return tmuxError("set-environment", out, err)
	}
	prefix := sess.Name + "\x00"
	b.Sessions.envs.Range(func(k, _ any) bool {
		if s, ok := k.(string); ok && strings.HasPrefix(s, prefix) {
			b.Sessions.envs.Delete(k)
		}
		return true
	})
	return nil
}

// onAccount is command run with the account: the default one unset (the
// box's or project's own pick would set it again otherwise), another set.
func onAccount(command string, env []string, dir string, isDefault bool) (string, []string) {
	if isDefault {
		return "env -u CLAUDE_CONFIG_DIR " + command, env
	}
	return command, append(append([]string(nil), env...), "CLAUDE_CONFIG_DIR="+dir)
}

// resumeOnAccount starts a structured session's host again on the other
// account, as resumeInPlace does, with its model and effort.
func (b *Box) resumeOnAccount(r *http.Request, sess Session, sid, dir string, isDefault bool) error {
	ctx := r.Context()
	stopCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := agenthost.Stop(stopCtx, sess.sock); err != nil && !errors.Is(err, agenthost.ErrNoHost) {
		return err
	}
	b.claude().forget(sess.Name)
	meta, _ := readConversation(sess.conv)
	l, err := b.launchClaudeIn(b.claudePreset(ctx, sess), meta.Model, meta.Effort, firstNonEmpty(modeFor(sess), meta.Mode), sid, sess.conv, sess.sock)
	if err != nil {
		return err
	}
	l.labels[labelHandoff] = ""
	b.forgetConversation(sess.Name)
	from := logEnd(sess.conv)
	command, env := onAccount(l.command, l.env, dir, isDefault)
	if err := b.respawnAgent(ctx, sess, command, env, l.labels); err != nil {
		return err
	}
	noteResumed(sess.conv, sess.Name)
	if f := awaitStart(sess.conv, sess.sock, from, 15*time.Second); f != nil {
		noteFailed(sess.conv, f)
		return launchFailed(l.agent, f)
	}
	if live, err := b.Sessions.Get(ctx, sess.Name); err == nil {
		b.startStructured(origin(r), gateOrigin(r), live, "")
	}
	return nil
}

// restartTerminalOnAccount runs Claude Code's terminal again in the pane,
// on the other account, with --resume.
func (b *Box) restartTerminalOnAccount(r *http.Request, sess Session, sid, dir string, isDefault bool) error {
	ctx := r.Context()
	command := b.claudePreset(ctx, sess).Command + " --resume " + shellQuote(sid)
	command, env := onAccount(command, nil, dir, isDefault)
	if err := b.respawnAgent(ctx, sess, command, env, nil); err != nil {
		return err
	}
	b.beginStartup(origin(r), Session{Name: sess.Name, Agent: "claude", Dir: sess.Dir})
	return nil
}
