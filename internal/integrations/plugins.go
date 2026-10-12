package integrations

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

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

// PluginTools is what an allowed plugin gives agents.
type PluginTools struct {
	ID     string
	Dir    string
	MCP    map[string]PluginMCPServer
	Skills map[string]PluginSkill
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
		MCP    map[string]PluginMCPServer `json:"mcp"`
		Skills map[string]PluginSkill     `json:"skills"`
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
	t.MCP, t.Skills = m.MCP, m.Skills
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
