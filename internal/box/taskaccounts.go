package box

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"sort"

	"github.com/cosscom/shipyard/internal/integrations"
)

// A session or task can pick which account its agent uses, for it alone:
// "accounts": {"claude": "~/.berth/accounts/claude/work"} in the request
// (SessionRequest, TaskRequest). Without one, the project's or the box's
// choice stands (the Usage plugin's "Use for new sessions"). The request
// carries the variables down to the session it starts (accountEnv in
// createAgentSessionWith), after the box's and the project's environment,
// so they win.

type accountsKey struct{}

// taskAccountCaps says a box takes "accounts": one that doesn't would start
// the session with the project's account and say nothing (an unknown field
// is ignored), so the app offers the picker only where this is listed.
var taskAccountCaps = []string{"task.accounts"}

// withAccounts is r carrying the account variables picks asks for. Each
// must be one of that agent's account folders on this box
// (integrations.Accounts); anything else is refused before anything starts.
func (b *Box) withAccounts(r *http.Request, picks map[string]string) (*http.Request, error) {
	if len(picks) == 0 {
		return r, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return r, err
	}
	var env []string
	for agent, dir := range picks {
		if dir == "" {
			continue
		}
		name, ok := integrations.AccountVars[agent]
		if !ok {
			return r, badRequest("%s has no accounts; use claude or codex", agent)
		}
		want := filepath.Clean(integrations.ExpandAccountDir(home, dir))
		found := ""
		for _, a := range integrations.Accounts(home, agent) {
			if filepath.Clean(a.Dir) == want {
				found = a.Dir
				break
			}
		}
		if found == "" {
			return r, badRequest("%s is not one of %s's accounts on this box", dir, agent)
		}
		env = append(env, name+"="+found)
	}
	if len(env) == 0 {
		return r, nil
	}
	sort.Strings(env)
	return r.WithContext(context.WithValue(r.Context(), accountsKey{}, env)), nil
}

// accountEnv is the account variables a request picked, for the session it
// starts; none when it picked none.
func accountEnv(ctx context.Context) []string {
	env, _ := ctx.Value(accountsKey{}).([]string)
	return env
}
