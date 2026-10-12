package integrations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cosscom/shipyard/internal/hooks"
)

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// acmePlugin writes a plugin "acme" with one MCP server and one skill into
// pluginsDir and returns its folder. allow says whether the user allowed it.
func acmePlugin(t *testing.T, pluginsDir string, allow bool) string {
	t.Helper()
	dir := filepath.Join(pluginsDir, "acme")
	bin := []byte("#!/bin/sh\necho acme\n")
	skill := []byte("---\nname: acme-tool\n---\nUse the acme tool.\n")
	writePluginFile(t, filepath.Join(dir, "bin", "acme-mcp"), bin)
	writePluginFile(t, filepath.Join(dir, "skills", "acme-tool", "SKILL.md"), skill)
	manifest, _ := json.Marshal(map[string]any{
		"name": "Acme",
		"mcp": map[string]any{
			"acme": map[string]any{"command": "bin/acme-mcp", "args": []string{"mcp"}, "sha256": sum(bin)},
		},
		"skills": map[string]any{
			"acme-tool": map[string]any{"path": "skills/acme-tool/SKILL.md", "sha256": sum(skill)},
		},
	})
	writePluginFile(t, filepath.Join(dir, hooks.PluginManifest), manifest)
	if allow {
		h, err := hooks.PluginHash(dir)
		if err != nil {
			t.Fatal(err)
		}
		if err := hooks.AllowPlugin(dir, h); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func writePluginFile(t *testing.T, p string, b []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, b, 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestPluginToolsRejectsUnsafeManifests(t *testing.T) {
	good := sum([]byte("x"))
	for name, manifest := range map[string]string{
		"path outside":   `{"mcp":{"a":{"command":"../../bin/sh","sha256":"` + good + `"}}}`,
		"absolute path":  `{"mcp":{"a":{"command":"/bin/sh","sha256":"` + good + `"}}}`,
		"no hash":        `{"mcp":{"a":{"command":"bin/a"}}}`,
		"bad name":       `{"mcp":{"A B":{"command":"bin/a","sha256":"` + good + `"}}}`,
		"berth's skill":  `{"skills":{"berth":{"path":"s/SKILL.md","sha256":"` + good + `"}}}`,
		"empty path":     `{"skills":{"x":{"path":"","sha256":"` + good + `"}}}`,
		"invalid json":   `{"mcp":`,
		"short sha":      `{"mcp":{"a":{"command":"bin/a","sha256":"abc"}}}`,
		"dot path":       `{"mcp":{"a":{"command":".","sha256":"` + good + `"}}}`,
		"traversal deep": `{"skills":{"x":{"path":"a/../../../etc/passwd","sha256":"` + good + `"}}}`,
	} {
		dir := filepath.Join(t.TempDir(), "acme")
		writePluginFile(t, filepath.Join(dir, hooks.PluginManifest), []byte(manifest))
		if _, err := readPluginTools(dir); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if got := pluginRel("./bin//acme-mcp"); got != "bin/acme-mcp" {
		t.Errorf("pluginRel should clean a plain path, got %q", got)
	}
}

func TestPluginMCPCommandChecksConsentAndBytes(t *testing.T) {
	plugins := t.TempDir()
	dir := acmePlugin(t, plugins, false)

	if _, _, _, err := PluginMCPCommand(plugins, "acme", "acme"); !errors.Is(err, ErrPluginOff) {
		t.Fatalf("a plugin nobody allowed must not run, got %v", err)
	}

	acmePlugin(t, plugins, true)
	exe, args, gotDir, err := PluginMCPCommand(plugins, "acme", "acme")
	if err != nil {
		t.Fatal(err)
	}
	if exe != filepath.Join(dir, "bin", "acme-mcp") || gotDir != dir || len(args) != 1 || args[0] != "mcp" {
		t.Fatalf("unexpected command %q %v in %q", exe, args, gotDir)
	}
	if _, _, _, err := PluginMCPCommand(plugins, "acme", "other"); err == nil {
		t.Fatal("expected an error for a server the plugin does not ship")
	}

	// The binary is swapped after the user allowed the plugin.
	writePluginFile(t, filepath.Join(dir, "bin", "acme-mcp"), []byte("#!/bin/sh\necho evil\n"))
	if _, _, _, err := PluginMCPCommand(plugins, "acme", "acme"); err == nil || !strings.Contains(err.Error(), "not the file") {
		t.Fatalf("a swapped binary must not run, got %v", err)
	}
}

func TestPluginToolsOffWhenManifestChanges(t *testing.T) {
	plugins := t.TempDir()
	dir := acmePlugin(t, plugins, true)
	if tools, _ := EnabledPluginTools(plugins); len(tools) != 1 {
		t.Fatalf("expected the allowed plugin's tools, got %v", tools)
	}
	// Re-pinning another binary edits the manifest: consent no longer holds.
	b, _ := os.ReadFile(filepath.Join(dir, hooks.PluginManifest))
	writePluginFile(t, filepath.Join(dir, hooks.PluginManifest), []byte(strings.Replace(string(b), `"Acme"`, `"Acme 2"`, 1)))
	if tools, _ := EnabledPluginTools(plugins); len(tools) != 0 {
		t.Fatalf("a changed manifest must turn its tools off, got %v", tools)
	}
}

// syncHome is a home with berth's own MCP server and skills installed for
// Claude Code and Codex, plus a Gemini settings file berth was kept out of.
func syncHome(t *testing.T) (home, bin string) {
	t.Helper()
	home = t.TempDir()
	bin = filepath.Join(home, "bin", "berthd")
	writePluginFile(t, filepath.Join(home, ".claude.json"), []byte(`{"mcpServers":{"berth":{"command":"`+bin+`","args":["mcp"]},"berth-mine":{"command":"/opt/mine"}}}`))
	writePluginFile(t, filepath.Join(home, ".claude", "skills", "berth", "SKILL.md"), []byte("berth"))
	writePluginFile(t, filepath.Join(home, ".claude", "skills", "my-skill", "SKILL.md"), []byte("mine"))
	writePluginFile(t, filepath.Join(home, ".codex", "config.toml"), []byte("model = \"x\"\n\n[mcp_servers.berth]\ncommand = \"berthd\"\nargs = [\"mcp\"]\n"))
	writePluginFile(t, filepath.Join(home, ".agents", "skills", "berth", "SKILL.md"), []byte("berth"))
	writePluginFile(t, filepath.Join(home, ".gemini", "settings.json"), []byte(`{"theme":"dark"}`))
	return home, bin
}

func TestSyncPluginToolsAddsAndRemovesOnlyItsOwn(t *testing.T) {
	home, bin := syncHome(t)
	plugins := filepath.Join(home, ".berth", "plugins")
	dir := acmePlugin(t, plugins, true)

	changed, err := SyncPluginTools(home, plugins, bin)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) == 0 {
		t.Fatal("expected changes")
	}
	claude := readJSON(t, filepath.Join(home, ".claude.json"))
	servers := claude["mcpServers"].(map[string]any)
	acme, ok := servers["berth-acme"].(map[string]any)
	if !ok || acme["command"] != bin || anyStrings(acme["args"])[0] != pluginMCPArg {
		t.Fatalf("missing plugin server entry: %#v", servers)
	}
	if servers["berth-mine"] == nil {
		t.Fatal("the user's own berth-mine server must be kept")
	}
	codex, _ := os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
	if !strings.Contains(string(codex), "[mcp_servers.berth-acme]") || !strings.Contains(string(codex), `"plugin-mcp", "acme", "acme"`) {
		t.Fatalf("missing Codex table:\n%s", codex)
	}
	for _, skills := range []string{filepath.Join(home, ".claude", "skills"), filepath.Join(home, ".agents", "skills")} {
		if _, err := os.Stat(filepath.Join(skills, "acme-tool", "SKILL.md")); err != nil {
			t.Fatalf("missing plugin skill in %s: %v", skills, err)
		}
	}
	gemini, _ := os.ReadFile(filepath.Join(home, ".gemini", "settings.json"))
	if strings.Contains(string(gemini), "berth-acme") {
		t.Fatal("an agent without berth's own MCP server must be left alone")
	}

	// Nothing to do the second time.
	if changed, err := SyncPluginTools(home, plugins, bin); err != nil || len(changed) != 0 {
		t.Fatalf("second sync should be a no-op, got %v %v", changed, err)
	}

	// Turning the plugin off takes its tools away, and nothing else.
	if err := hooks.ForbidPlugin(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := SyncPluginTools(home, plugins, bin); err != nil {
		t.Fatal(err)
	}
	servers = readJSON(t, filepath.Join(home, ".claude.json"))["mcpServers"].(map[string]any)
	if servers["berth-acme"] != nil || servers["berth-mine"] == nil || servers["berth"] == nil {
		t.Fatalf("unexpected servers after turning off: %#v", servers)
	}
	codex, _ = os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
	if strings.Contains(string(codex), "berth-acme") || !strings.Contains(string(codex), "[mcp_servers.berth]") {
		t.Fatalf("unexpected Codex config after turning off:\n%s", codex)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "acme-tool")); !os.IsNotExist(err) {
		t.Fatal("the plugin's skill should be removed")
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "my-skill", "SKILL.md")); err != nil {
		t.Fatal("the user's own skill must be kept")
	}
}

func TestSyncPluginToolsKeepsUsersTablesAndSkills(t *testing.T) {
	home, bin := syncHome(t)
	// The user has their own server and skill with the plugin's names.
	writePluginFile(t, filepath.Join(home, ".codex", "config.toml"), []byte("[mcp_servers.berth]\ncommand = \"berthd\"\nargs = [\"mcp\"]\n\n[mcp_servers.berth-acme]\ncommand = \"/opt/acme\"\n"))
	writePluginFile(t, filepath.Join(home, ".claude", "skills", "acme-tool", "SKILL.md"), []byte("the user's"))
	plugins := filepath.Join(home, ".berth", "plugins")
	acmePlugin(t, plugins, true)

	if _, err := SyncPluginTools(home, plugins, bin); err != nil {
		t.Fatal(err)
	}
	codex, _ := os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
	if !strings.Contains(string(codex), `command = "/opt/acme"`) {
		t.Fatalf("the user's own table must be kept:\n%s", codex)
	}
	b, _ := os.ReadFile(filepath.Join(home, ".claude", "skills", "acme-tool", "SKILL.md"))
	if string(b) != "the user's" {
		t.Fatalf("the user's own skill must not be overwritten, got %q", b)
	}
}

func TestSyncPluginToolsNeedsBerthd(t *testing.T) {
	home, _ := syncHome(t)
	plugins := filepath.Join(home, ".berth", "plugins")
	acmePlugin(t, plugins, true)
	if changed, err := SyncPluginTools(home, plugins, filepath.Join(home, "bin", "berth")); err != nil || len(changed) != 0 {
		t.Fatalf("only berthd runs plugin servers, got %v %v", changed, err)
	}
}

// setupPlugin writes and allows a plugin "acme" whose binary answers a
// status check with ready and records that setup ran.
func setupPlugin(t *testing.T, pluginsDir, ready, setupJSON string) string {
	t.Helper()
	dir := filepath.Join(pluginsDir, "acme")
	bin := []byte("#!/bin/sh\nif [ \"$1\" = status ]; then echo '{\"allGranted\": " + ready + "}'; exit 0; fi\ntouch \"$BERTH_PLUGIN_DIR/setup-ran\"\n")
	writePluginFile(t, filepath.Join(dir, "bin", "acme"), bin)
	manifest := `{"mcp":{"acme":{"command":"bin/acme","args":["mcp"],"sha256":"` + sum(bin) + `"}}` + setupJSON + `}`
	writePluginFile(t, filepath.Join(dir, hooks.PluginManifest), []byte(manifest))
	h, err := hooks.PluginHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := hooks.AllowPlugin(dir, h); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestPluginSetupStatusAndStart(t *testing.T) {
	for _, ready := range []string{"false", "true"} {
		plugins := t.TempDir()
		dir := setupPlugin(t, plugins, ready, `,"setup":{"server":"acme","status":["status"],"ready":"allGranted","label":"Allow it"}`)
		st, err := PluginSetupStatus(context.Background(), plugins, "acme")
		if err != nil {
			t.Fatal(err)
		}
		if !st.Known || st.Ready != (ready == "true") || st.Label != "Allow it" {
			t.Fatalf("ready=%s: unexpected state %+v", ready, st)
		}
		if err := StartPluginSetup(plugins, "acme"); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(5 * time.Second)
		for {
			if _, err := os.Stat(filepath.Join(dir, "setup-ran")); err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("setup did not run in the plugin's folder")
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
}

func TestPluginSetupNeedsAnAllowedPluginAndAKnownServer(t *testing.T) {
	plugins := t.TempDir()
	setupPlugin(t, plugins, "true", "")
	if _, err := PluginSetupStatus(context.Background(), plugins, "acme"); !errors.Is(err, ErrNoSetup) {
		t.Fatalf("a plugin without setup: got %v", err)
	}

	plugins = t.TempDir()
	dir := setupPlugin(t, plugins, "true", `,"setup":{"server":"other"}`)
	if _, err := readPluginTools(dir); err == nil {
		t.Fatal("setup naming a server the plugin does not ship must be refused")
	}

	plugins = t.TempDir()
	dir = setupPlugin(t, plugins, "true", `,"setup":{"server":"acme"}`)
	if err := hooks.ForbidPlugin(dir); err != nil {
		t.Fatal(err)
	}
	if err := StartPluginSetup(plugins, "acme"); !errors.Is(err, ErrPluginOff) {
		t.Fatalf("a plugin that is off must not run its setup, got %v", err)
	}
}
