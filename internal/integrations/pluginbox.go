package integrations

import (
	"debug/elf"
	"debug/macho"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
)

// A plugin's tools can also go to a box (internal/box/plugins.go): the
// laptop sends the plugin's folder as the person allowed it, and the box
// checks it is that folder before its agents get the tools.

// VerifyPluginTools reads the tools of the plugin in dir and checks every
// file its manifest pins: each MCP server's binary and each skill hash to
// what the manifest says.
func VerifyPluginTools(dir string) (PluginTools, error) {
	t, err := readPluginTools(dir)
	if err != nil {
		return t, err
	}
	for name, s := range t.MCP {
		if _, err := verifyPluginFile(dir, s.Command, s.SHA256); err != nil {
			return t, fmt.Errorf("plugin %s: MCP server %s: %w", t.ID, name, err)
		}
	}
	for name, s := range t.Skills {
		if _, err := verifyPluginFile(dir, s.Path, s.SHA256); err != nil {
			return t, fmt.Errorf("plugin %s: skill %s: %w", t.ID, name, err)
		}
	}
	return t, nil
}

// ReadPluginTools reads the tools the plugin in dir's manifest declares,
// without checking its files.
func ReadPluginTools(dir string) (PluginTools, error) { return readPluginTools(dir) }

// PluginPlatforms is where the plugin's MCP servers can run, as GOOS-GOARCH
// ("darwin-arm64"): what every one of its native binaries was built for.
// A universal macOS binary runs on both Macs. known is false when no binary
// says (scripts, or a plugin with no servers): it may run anywhere.
func PluginPlatforms(t PluginTools) (platforms []string, known bool) {
	var common []string
	for _, s := range t.MCP {
		rel := pluginRel(s.Command)
		if rel == "" {
			continue
		}
		p := binaryPlatforms(filepath.Join(t.Dir, filepath.FromSlash(rel)))
		if p == nil {
			continue
		}
		if !known {
			common, known = p, true
			continue
		}
		common = slices.DeleteFunc(common, func(c string) bool { return !slices.Contains(p, c) })
	}
	sort.Strings(common)
	return common, known
}

// PluginRunsOn says whether a plugin built for platforms runs on platform.
func PluginRunsOn(platforms []string, known bool, platform string) bool {
	return !known || slices.Contains(platforms, platform)
}

// binaryPlatforms reads a Mach-O (thin or universal) or ELF executable's
// platforms; nil for anything else.
func binaryPlatforms(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	if fat, err := macho.NewFatFile(f); err == nil {
		var out []string
		for _, a := range fat.Arches {
			if p := machoPlatform(a.Cpu); p != "" && !slices.Contains(out, p) {
				out = append(out, p)
			}
		}
		return out
	}
	if m, err := macho.NewFile(f); err == nil {
		if p := machoPlatform(m.Cpu); p != "" {
			return []string{p}
		}
		return nil
	}
	if e, err := elf.NewFile(f); err == nil {
		switch e.Machine {
		case elf.EM_X86_64:
			return []string{"linux-amd64"}
		case elf.EM_AARCH64:
			return []string{"linux-arm64"}
		}
	}
	return nil
}

func machoPlatform(cpu macho.Cpu) string {
	switch cpu {
	case macho.CpuArm64:
		return "darwin-arm64"
	case macho.CpuAmd64:
		return "darwin-amd64"
	}
	return ""
}
