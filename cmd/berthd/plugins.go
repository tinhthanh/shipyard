package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/cosscom/shipyard/internal/integrations"
	"github.com/cosscom/shipyard/internal/statefile"
)

// pluginToolsInterval is how often berthd brings agents' configs in line
// with the plugins that are on; allowing a plugin happens elsewhere (the
// app, or a hand-edited plugins.json), so berthd checks rather than waits.
const pluginToolsInterval = 30 * time.Second

func pluginsDir() (string, error) {
	userDir, err := statefile.UserDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(userDir, "plugins"), nil
}

// pluginMCP is `berthd plugin-mcp <plugin> <server>`: what an agent's
// config runs for a plugin's MCP server. It checks the plugin is on and its
// binary is the one allowed, then becomes that binary, in the plugin's
// folder, so the agent talks to it directly over stdio.
func pluginMCP(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: berthd plugin-mcp <plugin> <server>")
	}
	dir, err := pluginsDir()
	if err != nil {
		return err
	}
	exe, srvArgs, pluginDir, err := integrations.PluginMCPCommand(dir, args[0], args[1])
	if err != nil {
		return err
	}
	if err := os.Chdir(pluginDir); err != nil {
		return err
	}
	return syscall.Exec(exe, append([]string{exe}, srvArgs...), integrations.PluginEnv(pluginDir))
}

// pluginsCommand is `berthd plugins sync`.
func pluginsCommand(args []string, out io.Writer) error {
	if len(args) != 1 || args[0] != "sync" {
		return fmt.Errorf("usage: berthd plugins sync")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir, err := pluginsDir()
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	changed, err := integrations.SyncPluginTools(home, dir, exe)
	for _, c := range changed {
		fmt.Fprintln(out, "updated", c)
	}
	if len(changed) == 0 && err == nil {
		fmt.Fprintln(out, "agents' plugin tools are up to date")
	}
	return err
}

// syncPluginToolsLoop keeps agents' plugin MCP servers and skills in step
// with the plugins that are on, until ctx ends.
func syncPluginToolsLoop(ctx context.Context, home, pluginsDir, exe string, logger *log.Logger) {
	sync := func() {
		changed, err := integrations.SyncPluginTools(home, pluginsDir, exe)
		for _, c := range changed {
			logger.Printf("plugins: updated %s", c)
		}
		if err != nil {
			logger.Printf("plugins: %v", err)
		}
	}
	sync()
	t := time.NewTicker(pluginToolsInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			sync()
		}
	}
}
