package integrations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/cosscom/shipyard/internal/hooks"
)

// A plugin in ~/.berth/plugins can give agents tools: MCP servers and
// skills it ships in its folder. Its berth-plugin.json lists each file with
// a sha256, and the manifest is part of what the user allowed (see
// hooks.PluginHash), so pinning a file's bytes there puts the file under the
// same consent: a binary swapped after the review no longer matches.
//
// Agents never run a plugin's binary directly. Their config runs
// `berthd plugin-mcp <plugin> <server>`, which checks the plugin is still
// allowed and the binary still hashes to what the manifest pins, then
// replaces itself with it. SyncPluginTools keeps the agents' configs and
// skill folders in step with the plugins that are on.

// PluginMCPServer is one MCP stdio server a plugin ships.
type PluginMCPServer struct {
	// Command is the executable, relative to the plugin's folder.
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	// SHA256 is the hex sha256 of Command's bytes.
	SHA256 string `json:"sha256"`
}

// PluginSkill is one skill a plugin ships: a SKILL.md in its folder.
type PluginSkill struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// PluginSetup is the one-time step a plugin's tools need on the machine they
// run on, such as macOS permissions, that only the person at it can give. It
// reuses one of the plugin's pinned MCP binaries, so it needs no pin of its
// own.
type PluginSetup struct {
	// Server names the MCP server whose binary runs the setup.
	Server string `json:"server"`
	// Args start the setup: typically a window that walks the person through it.
	Args []string `json:"args,omitempty"`
	// Status, when set, are the args that report whether setup is done:
	// a JSON object whose Ready key is true, or an exit code of 0 without Ready.
	Status []string `json:"status,omitempty"`
	Ready  string   `json:"ready,omitempty"`
	// Problem, when set, is the status report's key that says what is left
	// to do, in the plugin's words: a string, or a list of strings.
	Problem string `json:"problem,omitempty"`
	// Label says what setup does, for the app's button.
	Label string `json:"label,omitempty"`
}

// PluginSettings are switches the person sets for a plugin's tools in the
// app, such as which Chrome profiles agents may use. The plugin lists them,
// with their current values, from one of its pinned MCP binaries; berth
// keeps the values outside the plugin's folder, so changing one needs no new
// review and survives updates. The binary reads them from the file named by
// $BERTH_PLUGIN_SETTINGS.
type PluginSettings struct {
	// Server names the MCP server whose binary lists the settings.
	Server string `json:"server"`
	// List are the args that print {"settings": [PluginSetting...]}.
	List []string `json:"list"`
	// Label heads the settings in the app.
	Label string `json:"label,omitempty"`
}

// PluginSetting is one switch a plugin lists.
type PluginSetting struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Value       bool   `json:"value"`
	// Warning, when set, is shown before the person turns the switch on.
	Warning string `json:"warning,omitempty"`
}

// PluginTools is what an allowed plugin gives agents.
type PluginTools struct {
	ID       string
	Dir      string
	MCP      map[string]PluginMCPServer
	Skills   map[string]PluginSkill
	Setup    *PluginSetup
	Settings *PluginSettings
}

// pluginToolName is the shape of plugin ids, server and skill names: they
// become MCP server keys, TOML table names and folder names.
var pluginToolName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)

// pluginMCPArg is the berthd subcommand agents' configs run for a plugin's
// server; an entry whose args start with it is one berth wrote.
const pluginMCPArg = "plugin-mcp"

// pluginSkillMarker sits beside a SKILL.md berth copied from a plugin and
// holds the plugin's id, so berth only ever removes skills it installed.
const pluginSkillMarker = ".berth-plugin"

// PluginMCPKey is the agents' name for a plugin's server: berth-<plugin>,
// or berth-<plugin>-<server> when the two differ.
func PluginMCPKey(plugin, server string) string {
	if plugin == server {
		return "berth-" + plugin
	}
	return "berth-" + plugin + "-" + server
}

func readPluginTools(dir string) (PluginTools, error) {
	id := filepath.Base(dir)
	t := PluginTools{ID: id, Dir: dir}
	if !pluginToolName.MatchString(id) {
		return t, fmt.Errorf("plugin %q: tools need an id of lowercase letters, digits and dashes", id)
	}
	b, err := os.ReadFile(filepath.Join(dir, hooks.PluginManifest))
	if err != nil {
		return t, err
	}
	var m struct {
		MCP      map[string]PluginMCPServer `json:"mcp"`
		Skills   map[string]PluginSkill     `json:"skills"`
		Setup    *PluginSetup               `json:"setup"`
		Settings *PluginSettings            `json:"settings"`
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return t, fmt.Errorf("plugin %s: %s: %w", id, hooks.PluginManifest, err)
	}
	for name, s := range m.MCP {
		if err := checkPluginFile(id, "MCP server", name, s.Command, s.SHA256); err != nil {
			return t, err
		}
	}
	for name, s := range m.Skills {
		if err := checkPluginFile(id, "skill", name, s.Path, s.SHA256); err != nil {
			return t, err
		}
		if _, err := SkillContent(name); err == nil {
			return t, fmt.Errorf("plugin %s: skill %s has the name of one of berth's own", id, name)
		}
	}
	if m.Setup != nil {
		if _, ok := m.MCP[m.Setup.Server]; !ok {
			return t, fmt.Errorf("plugin %s: setup names MCP server %q, which the plugin does not ship", id, m.Setup.Server)
		}
	}
	if m.Settings != nil {
		if _, ok := m.MCP[m.Settings.Server]; !ok {
			return t, fmt.Errorf("plugin %s: settings name MCP server %q, which the plugin does not ship", id, m.Settings.Server)
		}
		if len(m.Settings.List) == 0 {
			return t, fmt.Errorf("plugin %s: settings need list args", id)
		}
	}
	t.MCP, t.Skills, t.Setup, t.Settings = m.MCP, m.Skills, m.Setup, m.Settings
	return t, nil
}

func checkPluginFile(plugin, kind, name, rel, sum string) error {
	if !pluginToolName.MatchString(name) {
		return fmt.Errorf("plugin %s: %s %q: names are lowercase letters, digits and dashes", plugin, kind, name)
	}
	if pluginRel(rel) == "" {
		return fmt.Errorf("plugin %s: %s %s needs a file inside the plugin's folder", plugin, kind, name)
	}
	if b, err := hex.DecodeString(sum); err != nil || len(b) != sha256.Size {
		return fmt.Errorf("plugin %s: %s %s needs the file's sha256, in hex", plugin, kind, name)
	}
	return nil
}

// pluginRel is rel as a clean path inside the plugin's folder, or "" when
// it is not one. A path that climbs with ".." is refused outright rather
// than cleaned, so what a manifest shows the reviewer is what runs.
func pluginRel(rel string) string {
	if rel == "" || filepath.IsAbs(rel) {
		return ""
	}
	slashed := filepath.ToSlash(rel)
	for _, part := range strings.Split(slashed, "/") {
		if part == ".." {
			return ""
		}
	}
	clean := path.Clean("/" + slashed)[1:]
	if clean == "" || clean == "." {
		return ""
	}
	return clean
}

// verifyPluginFile checks that rel, read from inside dir only, hashes to sum.
func verifyPluginFile(dir, rel, sum string) ([]byte, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	f, err := root.Open(pluginRel(rel))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	h := sha256.New()
	var small []byte
	// Skills are read back to be installed; binaries are only hashed.
	if strings.HasSuffix(rel, ".md") {
		if small, err = io.ReadAll(io.LimitReader(f, 1<<20+1)); err != nil {
			return nil, err
		}
		if len(small) > 1<<20 {
			return nil, fmt.Errorf("%s is larger than 1 MB", rel)
		}
		h.Write(small)
	} else if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}
	if got := hex.EncodeToString(h.Sum(nil)); !strings.EqualFold(got, sum) {
		return nil, fmt.Errorf("%s is not the file the plugin was allowed with (sha256 %s, expected %s)", rel, got, strings.ToLower(sum))
	}
	return small, nil
}

// EnabledPluginTools lists the tools of the plugins in pluginsDir that are
// on (allowed, and still what was allowed). A plugin whose tools cannot be
// read is reported in errs and left out.
func EnabledPluginTools(pluginsDir string) (out []PluginTools, errs []error) {
	manifests, _ := filepath.Glob(filepath.Join(pluginsDir, "*", hooks.PluginManifest))
	sort.Strings(manifests)
	for _, m := range manifests {
		dir := filepath.Dir(m)
		if !hooks.PluginEnabled(dir) {
			continue
		}
		t, err := readPluginTools(dir)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if len(t.MCP) > 0 || len(t.Skills) > 0 {
			out = append(out, t)
		}
	}
	return out, errs
}

// ErrPluginOff is returned for a plugin that is not on.
var ErrPluginOff = errors.New("plugin is off")

// PluginMCPCommand resolves the server a plugin ships for `berthd
// plugin-mcp`: the executable's absolute path, its args, and the plugin's
// folder to run it in. It refuses a plugin that is off and a binary that is
// not the one the manifest pins.
func PluginMCPCommand(pluginsDir, plugin, server string) (exe string, args []string, dir string, err error) {
	if !pluginToolName.MatchString(plugin) || !pluginToolName.MatchString(server) {
		return "", nil, "", fmt.Errorf("plugin-mcp: bad plugin or server name")
	}
	dir = filepath.Join(pluginsDir, plugin)
	if !hooks.PluginEnabled(dir) {
		return "", nil, "", fmt.Errorf("plugin %s: %w; turn it on in Shipyard's Settings → Plugins", plugin, ErrPluginOff)
	}
	t, err := readPluginTools(dir)
	if err != nil {
		return "", nil, "", err
	}
	s, ok := t.MCP[server]
	if !ok {
		return "", nil, "", fmt.Errorf("plugin %s has no MCP server %s", plugin, server)
	}
	if _, err := verifyPluginFile(dir, s.Command, s.SHA256); err != nil {
		return "", nil, "", fmt.Errorf("plugin %s: %w", plugin, err)
	}
	return filepath.Join(dir, filepath.FromSlash(pluginRel(s.Command))), s.Args, dir, nil
}

// SyncPluginTools makes the agents' configs match the plugins that are on:
// an MCP server entry per plugin server (berth-<plugin>[-<server>]) running
// `bin plugin-mcp`, and a copy of each plugin skill, in every Claude Code and
// Codex account and Gemini CLI where berth's own MCP server or skills are
// installed, so it never reaches an agent the user kept berth out of. It
// removes what it wrote for plugins that are off, and returns what changed.
func SyncPluginTools(home, pluginsDir, bin string) ([]string, error) {
	if !mcpBin(bin) {
		return nil, nil
	}
	tools, errs := EnabledPluginTools(pluginsDir)
	want := map[string][]any{}
	skills := map[string]pluginSkillCopy{}
	for _, t := range tools {
		for name := range t.MCP {
			want[PluginMCPKey(t.ID, name)] = []any{pluginMCPArg, t.ID, name}
		}
		for name, s := range t.Skills {
			b, err := verifyPluginFile(t.Dir, s.Path, s.SHA256)
			if err != nil {
				errs = append(errs, fmt.Errorf("plugin %s: %w", t.ID, err))
				continue
			}
			skills[name] = pluginSkillCopy{plugin: t.ID, content: b}
		}
	}
	var changed []string
	note := func(did bool, err error, what string) {
		if err != nil {
			errs = append(errs, err)
		} else if did {
			changed = append(changed, what)
		}
	}
	for _, a := range Accounts(home, "claude") {
		mf, rel := a.claudeJSON(home)
		did, err := syncPluginMCPJSON(mf, rel, bin, want, true)
		note(did, err, "MCP servers in "+tilde(home, mf.path(rel)))
		f := a.files()
		did, err = syncPluginSkills(f, "skills", skills)
		note(did, err, "skills in "+tilde(home, f.path("skills")))
	}
	for _, a := range Accounts(home, "codex") {
		f := a.files()
		did, err := syncPluginMCPToml(f, "config.toml", bin, want)
		note(did, err, "MCP servers in "+tilde(home, f.path("config.toml")))
	}
	codexSkills := files{root: home}
	did, err := syncPluginSkills(codexSkills, skillDirs["codex"], skills)
	note(did, err, "skills in "+tilde(home, codexSkills.path(skillDirs["codex"])))
	gemini := files{root: filepath.Join(home, ".gemini")}
	did, err = syncPluginMCPJSON(gemini, "settings.json", bin, want, false)
	note(did, err, "MCP servers in "+tilde(home, gemini.path("settings.json")))
	return changed, errors.Join(errs...)
}

type pluginSkillCopy struct {
	plugin  string
	content []byte
}

func ownedPluginEntry(v any) bool {
	m, _ := v.(map[string]any)
	args, _ := m["args"].([]any)
	return len(args) > 0 && args[0] == pluginMCPArg
}

// syncPluginMCPJSON edits mcpServers in a JSON settings file, only when
// berth's own server is already there.
func syncPluginMCPJSON(f files, rel, bin string, want map[string][]any, claude bool) (bool, error) {
	if _, err := f.read(rel); os.IsNotExist(err) {
		return false, nil
	}
	return editJSON(f, rel, func(root map[string]any) bool {
		servers, _ := root["mcpServers"].(map[string]any)
		if _, ok := servers["berth"]; !ok {
			return false
		}
		changed := false
		for key, v := range servers {
			if _, keep := want[key]; !keep && strings.HasPrefix(key, "berth-") && ownedPluginEntry(v) {
				delete(servers, key)
				changed = true
			}
		}
		for key, args := range want {
			entry := map[string]any{"command": bin, "args": args}
			if claude {
				entry["type"] = "stdio"
			}
			if cur, ok := servers[key].(map[string]any); ok && cur["command"] == bin && slices.Equal(anyStrings(cur["args"]), anyStrings(args)) {
				continue
			}
			if cur, ok := servers[key]; ok && !ownedPluginEntry(cur) {
				// The user's own server by that name: leave it be.
				continue
			}
			servers[key] = entry
			changed = true
		}
		return changed
	})
}

func anyStrings(v any) []string {
	list, _ := v.([]any)
	out := make([]string, 0, len(list))
	for _, x := range list {
		s, _ := x.(string)
		out = append(out, s)
	}
	return out
}

// codexPluginTable matches a table berth wrote for a plugin server.
var codexPluginTable = regexp.MustCompile(`^\[mcp_servers\.(berth-[a-z0-9-]+)\]\s*$`)

// syncPluginMCPToml rewrites the [mcp_servers.berth-…] tables berth wrote
// for plugins in Codex's config.toml, only when berth's own server is there.
func syncPluginMCPToml(f files, rel, bin string, want map[string][]any) (bool, error) {
	before, err := f.read(rel)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !strings.Contains(string(before), "[mcp_servers.berth]") {
		return false, nil
	}
	lines := strings.SplitAfter(string(before), "\n")
	var kept []string
	for i := 0; i < len(lines); i++ {
		m := codexPluginTable.FindStringSubmatch(strings.TrimSpace(lines[i]))
		if m == nil {
			kept = append(kept, lines[i])
			continue
		}
		end := i + 1
		for end < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[end]), "[") {
			end++
		}
		if !strings.Contains(strings.Join(lines[i:end], ""), `"`+pluginMCPArg+`"`) {
			// Not one of berth's: keep the user's table.
			kept = append(kept, lines[i:end]...)
		}
		i = end - 1
	}
	after := strings.TrimRight(strings.Join(kept, ""), "\n") + "\n"
	keys := make([]string, 0, len(want))
	for k := range want {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		args := make([]string, 0, len(want[key]))
		for _, a := range want[key] {
			args = append(args, fmt.Sprintf("%q", a))
		}
		after += fmt.Sprintf("\n[mcp_servers.%s]\ncommand = %q\nargs = [%s]\n", key, bin, strings.Join(args, ", "))
	}
	if after == string(before) {
		return false, nil
	}
	if err := f.backup(rel, before); err != nil {
		return false, err
	}
	return true, f.write(rel, []byte(after), 0o600)
}

// syncPluginSkills makes the plugin skills in dir (below f) match want, only
// when berth's own skills are installed there, and only ever touches skill
// folders that carry berth's marker.
func syncPluginSkills(f files, dir string, want map[string]pluginSkillCopy) (bool, error) {
	if _, err := f.read(filepath.Join(dir, "berth", "SKILL.md")); err != nil {
		return false, nil
	}
	changed := false
	entries, _ := os.ReadDir(f.path(dir))
	for _, e := range entries {
		marker, err := f.read(filepath.Join(dir, e.Name(), pluginSkillMarker))
		if err != nil {
			continue
		}
		if w, ok := want[e.Name()]; ok && strings.TrimSpace(string(marker)) == w.plugin {
			continue
		}
		if err := os.RemoveAll(f.path(filepath.Join(dir, e.Name()))); err != nil {
			return changed, err
		}
		changed = true
	}
	for name, w := range want {
		skill := filepath.Join(dir, name, "SKILL.md")
		if cur, err := f.read(skill); err == nil {
			if _, owned := f.read(filepath.Join(dir, name, pluginSkillMarker)); owned != nil {
				continue // the user's own skill by that name
			}
			if string(cur) == string(w.content) {
				continue
			}
		}
		if err := f.write(filepath.Join(dir, name, pluginSkillMarker), []byte(w.plugin+"\n"), 0o644); err != nil {
			return changed, err
		}
		if err := f.write(skill, w.content, 0o644); err != nil {
			return changed, err
		}
		changed = true
	}
	return changed, nil
}

// ErrNoSetup is returned for a plugin whose tools need no setup.
var ErrNoSetup = errors.New("this plugin needs no setup")

// pluginSetupCommand resolves a plugin's setup: the verified binary (as
// `berthd plugin-mcp` would run it), the setup, and the plugin's folder.
func pluginSetupCommand(pluginsDir, plugin string) (string, *PluginSetup, string, error) {
	if !pluginToolName.MatchString(plugin) {
		return "", nil, "", fmt.Errorf("bad plugin name")
	}
	t, err := readPluginTools(filepath.Join(pluginsDir, plugin))
	if err != nil {
		return "", nil, "", err
	}
	if t.Setup == nil {
		return "", nil, "", ErrNoSetup
	}
	exe, _, dir, err := PluginMCPCommand(pluginsDir, plugin, t.Setup.Server)
	if err != nil {
		return "", nil, "", err
	}
	return exe, t.Setup, dir, nil
}

func pluginCommand(ctx context.Context, exe, dir string, args []string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Dir = dir
	cmd.Env = PluginEnv(dir)
	return cmd
}

// PluginEnv is the environment a plugin's binaries run with: this process's,
// plus the plugin's folder and its settings file.
func PluginEnv(dir string) []string {
	return append(os.Environ(), "BERTH_PLUGIN_DIR="+dir, "BERTH_PLUGIN_SETTINGS="+PluginSettingsPath(filepath.Dir(dir), filepath.Base(dir)))
}

// PluginSettingsPath is where berth keeps a plugin's setting values: beside
// the plugins folder (~/.berth/plugin-settings/<id>.json), so updating or
// replacing the plugin keeps them.
func PluginSettingsPath(pluginsDir, plugin string) string {
	return filepath.Join(filepath.Dir(pluginsDir), "plugin-settings", plugin+".json")
}

// ErrNoSettings is returned for a plugin that has no settings.
var ErrNoSettings = errors.New("this plugin has no settings")

// ListPluginSettings asks the plugin for its settings and their values.
func ListPluginSettings(ctx context.Context, pluginsDir, plugin string) (*PluginSettings, []PluginSetting, error) {
	if !pluginToolName.MatchString(plugin) {
		return nil, nil, fmt.Errorf("bad plugin name")
	}
	t, err := readPluginTools(filepath.Join(pluginsDir, plugin))
	if err != nil {
		return nil, nil, err
	}
	if t.Settings == nil {
		return nil, nil, ErrNoSettings
	}
	exe, _, dir, err := PluginMCPCommand(pluginsDir, plugin, t.Settings.Server)
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := pluginCommand(ctx, exe, dir, t.Settings.List).Output()
	if err != nil {
		return nil, nil, fmt.Errorf("the plugin could not list its settings: %w", err)
	}
	var report struct {
		Settings []PluginSetting `json:"settings"`
	}
	if err := json.Unmarshal(out, &report); err != nil {
		return nil, nil, errors.New("the plugin's settings list is not JSON")
	}
	return t.Settings, report.Settings, nil
}

// SetPluginSetting turns one of a plugin's listed settings on or off.
func SetPluginSetting(ctx context.Context, pluginsDir, plugin, key string, value bool) error {
	_, list, err := ListPluginSettings(ctx, pluginsDir, plugin)
	if err != nil {
		return err
	}
	known := false
	for _, s := range list {
		known = known || s.Key == key
	}
	if !known {
		return fmt.Errorf("the plugin has no setting %q", key)
	}
	path := PluginSettingsPath(pluginsDir, plugin)
	doc := struct {
		Values map[string]bool `json:"values"`
	}{}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &doc)
	}
	if doc.Values == nil {
		doc.Values = map[string]bool{}
	}
	doc.Values[key] = value
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// PluginSetupState is what the app shows for a plugin's setup.
type PluginSetupState struct {
	Label string `json:"label,omitempty"`
	// Ready: setup is done (or the plugin cannot tell, having no status).
	Ready bool `json:"ready"`
	// Known: the plugin reported its state; false when it has no status
	// command or the check failed.
	Known bool   `json:"known"`
	Error string `json:"error,omitempty"`
	// Problem: what is left to do, as the plugin says it (its manifest's
	// setup.problem key), when it is not ready.
	Problem string `json:"problem,omitempty"`
	// Locked: the box's screen is locked, so a setup that needs the person
	// waits until someone unlocks it (berthd fills it in on macOS).
	Locked bool `json:"locked,omitempty"`
}

// PluginSetupStatus runs a plugin's status check, for at most a few seconds.
func PluginSetupStatus(ctx context.Context, pluginsDir, plugin string) (PluginSetupState, error) {
	exe, setup, dir, err := pluginSetupCommand(pluginsDir, plugin)
	if err != nil {
		return PluginSetupState{}, err
	}
	st := PluginSetupState{Label: setup.Label}
	if len(setup.Status) == 0 {
		return st, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := pluginCommand(ctx, exe, dir, setup.Status).Output()
	if setup.Ready == "" {
		st.Known, st.Ready = true, err == nil
		return st, nil
	}
	if err != nil {
		st.Error = "the plugin's status check failed: " + err.Error()
		return st, nil
	}
	var report map[string]any
	if err := json.Unmarshal(out, &report); err != nil {
		st.Error = "the plugin's status check did not print JSON"
		return st, nil
	}
	ready, ok := report[setup.Ready].(bool)
	st.Known, st.Ready = ok, ready
	if setup.Problem != "" && !st.Ready {
		st.Problem = problemText(report[setup.Problem])
	}
	return st, nil
}

// problemText is a status report's problem: its string, or its strings
// joined, or "" when it says nothing.
func problemText(v any) string {
	switch v := v.(type) {
	case string:
		return strings.TrimSpace(v)
	case []any:
		var parts []string
		for _, x := range v {
			if s, ok := x.(string); ok && strings.TrimSpace(s) != "" {
				parts = append(parts, strings.TrimSpace(s))
			}
		}
		if len(parts) > 0 {
			return "Still needs: " + strings.Join(parts, ", ")
		}
	}
	return ""
}

// StartPluginSetup starts a plugin's setup and returns without waiting for
// it: it typically opens a window for the person at this computer.
func StartPluginSetup(pluginsDir, plugin string) error {
	exe, setup, dir, err := pluginSetupCommand(pluginsDir, plugin)
	if err != nil {
		return err
	}
	cmd := pluginCommand(context.Background(), exe, dir, setup.Args)
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
