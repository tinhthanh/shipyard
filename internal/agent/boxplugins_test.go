package agent

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cosscom/shipyard/internal/box"
	"github.com/cosscom/shipyard/internal/events"
	"github.com/cosscom/shipyard/internal/hooks"
	"github.com/cosscom/shipyard/internal/wire"
)

// toolsPlugin writes a plugin with an MCP server and a skill into the
// agent's plugins folder, as a person installs one; server is the server's
// binary.
func toolsPlugin(t *testing.T, a *runningAgent, version string, server []byte) string {
	t.Helper()
	skill := "---\nname: acme-tools\ndescription: test\n---\nUse the acme tools.\n"
	manifest := fmt.Sprintf(`{"name":"Acme tools","version":%q,
 "mcp":{"acme":{"command":"bin/acme-mcp","args":["mcp"],"sha256":%q}},
 "skills":{"acme-tools":{"path":"skills/acme-tools/SKILL.md","sha256":%q}}}`, version, sum(string(server)), sum(skill))
	dir := filepath.Join(a.dir, "user", "plugins", "acme")
	write(t, filepath.Join(dir, hooks.PluginManifest), manifest)
	write(t, filepath.Join(dir, "skills", "acme-tools", "SKILL.md"), skill)
	write(t, filepath.Join(dir, "bin", "acme-mcp"), string(server))
	os.Chmod(filepath.Join(dir, "bin", "acme-mcp"), 0o755)
	return dir
}

// pluginsRig is an agent paired with a box that takes plugins.
type pluginsRig struct {
	agent  *runningAgent
	token  string
	boxDir string
	box    *testBox
}

func newPluginsRig(t *testing.T) *pluginsRig {
	t.Helper()
	boxDir := filepath.Join(t.TempDir(), "plugins")
	bx := &box.Box{Name: "devbox", Events: &events.Bus{}, Plugins: &box.BoxPlugins{Dir: boxDir}}
	b := newBoxWith(t, func(s *wire.Server) { bx.MountPlugins(s) })
	a := startAgentConfig(t, b.pairLaptop(), &fakeNetworks{}, nil)
	eventually(t, "box online", func() bool { return stateOf(t, a) == StateOnline })
	return &pluginsRig{agent: a, token: uiToken(t, a), boxDir: boxDir, box: b}
}

func (r *pluginsRig) boxes(t *testing.T) PluginBoxes {
	t.Helper()
	resp, body := uiCall(t, r.agent, "GET", "/v1/plugins/acme/boxes", r.token)
	if resp.StatusCode != 200 {
		t.Fatalf("boxes: %d %s", resp.StatusCode, body)
	}
	var out PluginBoxes
	json.Unmarshal([]byte(body), &out)
	return out
}

func (r *pluginsRig) state(t *testing.T) string {
	t.Helper()
	pb := r.boxes(t)
	if len(pb.Boxes) != 1 {
		t.Fatalf("boxes: %+v", pb)
	}
	return pb.Boxes[0].State
}

func TestAPluginsToolsGoToABoxAndComeOff(t *testing.T) {
	r := newPluginsRig(t)
	dir := toolsPlugin(t, r.agent, "1.0.0", []byte("#!/bin/sh\necho one\n"))

	// Not allowed here: nothing is sent.
	if resp, body := uiCall(t, r.agent, "POST", "/v1/plugins/acme/boxes/devbox", r.token); resp.StatusCode != 409 {
		t.Fatalf("sending a plugin that is off: %d %s", resp.StatusCode, body)
	}
	allowPlugin(t, dir)
	if pb := r.boxes(t); !pb.Tools || !pb.Enabled || pb.Boxes[0].State != "off" {
		t.Fatalf("before: %+v", pb)
	}

	resp, body := uiCall(t, r.agent, "POST", "/v1/plugins/acme/boxes/devbox", r.token)
	if resp.StatusCode != 200 {
		t.Fatalf("install: %d %s", resp.StatusCode, body)
	}
	if got := r.state(t); got != "on" {
		t.Fatalf("after install: %s", got)
	}
	if !hooks.PluginEnabled(filepath.Join(r.boxDir, "acme")) {
		t.Fatal("the box should hold the plugin and its grant")
	}

	// A new version allowed here: the box's is outdated until it is sent.
	toolsPlugin(t, r.agent, "2.0.0", []byte("#!/bin/sh\necho two\n"))
	allowPlugin(t, dir)
	if got := r.state(t); got != "outdated" {
		t.Fatalf("after an update here: %s", got)
	}
	if resp, body := uiCall(t, r.agent, "POST", "/v1/plugins/acme/boxes/devbox", r.token); resp.StatusCode != 200 {
		t.Fatalf("update: %d %s", resp.StatusCode, body)
	}
	if got := r.state(t); got != "on" {
		t.Fatalf("after the update: %s", got)
	}

	if resp, body := uiCall(t, r.agent, "DELETE", "/v1/plugins/acme/boxes/devbox", r.token); resp.StatusCode != 200 {
		t.Fatalf("remove: %d %s", resp.StatusCode, body)
	}
	if got := r.state(t); got != "off" {
		t.Fatalf("after removing: %s", got)
	}
	if _, err := os.Stat(filepath.Join(r.boxDir, "acme")); !os.IsNotExist(err) {
		t.Fatal("the plugin is still on the box")
	}
}

func TestAPluginForAnotherPlatformIsMarkedAndNotSent(t *testing.T) {
	r := newPluginsRig(t)
	// An ELF for a Linux machine this test box is not.
	h := make([]byte, 64)
	copy(h, "\x7fELF")
	h[4], h[5], h[6] = 2, 1, 1
	binary.LittleEndian.PutUint16(h[16:], 2)
	machine := uint16(62)
	if box.HelperPlatform() == "linux-amd64" {
		machine = 183
	}
	binary.LittleEndian.PutUint16(h[18:], machine)
	binary.LittleEndian.PutUint32(h[20:], 1)
	binary.LittleEndian.PutUint16(h[52:], 64)
	allowPlugin(t, toolsPlugin(t, r.agent, "1.0.0", h))
	if got := r.state(t); got != "unsupported" {
		t.Fatalf("a plugin for another platform: %s", got)
	}
	if resp, _ := uiCall(t, r.agent, "POST", "/v1/plugins/acme/boxes/devbox", r.token); resp.StatusCode != 409 {
		t.Fatalf("sending it anyway should be refused by the box: %d", resp.StatusCode)
	}
}

func (r *pluginsRig) ledger() boxPluginLedgerDoc {
	var doc boxPluginLedgerDoc
	if b, err := os.ReadFile(filepath.Join(r.agent.dir, "box-plugins.json")); err == nil {
		json.Unmarshal(b, &doc)
	}
	return doc
}

func (r *pluginsRig) onBox() bool {
	_, err := os.Stat(filepath.Join(r.boxDir, "acme"))
	return err == nil
}

func (r *pluginsRig) putOn(t *testing.T) {
	t.Helper()
	if resp, body := uiCall(t, r.agent, "POST", "/v1/plugins/acme/boxes/devbox", r.token); resp.StatusCode != 200 {
		t.Fatalf("install: %d %s", resp.StatusCode, body)
	}
	if on := r.ledger().On["acme"]; len(on) != 1 || on[0] != "devbox" {
		t.Fatalf("box-plugins.json after the install: %+v", r.ledger())
	}
}

func TestAPluginTurnedOffHereComesOffTheBoxes(t *testing.T) {
	r := newPluginsRig(t)
	dir := toolsPlugin(t, r.agent, "1.0.0", []byte("#!/bin/sh\necho one\n"))
	allowPlugin(t, dir)
	r.putOn(t)

	if resp, body := uiCall(t, r.agent, "POST", "/v1/plugins/acme/disable", r.token); resp.StatusCode != 200 {
		t.Fatalf("disable: %d %s", resp.StatusCode, body)
	}
	eventually(t, "acme off the box", func() bool { return !r.onBox() })
	eventually(t, "nothing left to do", func() bool {
		doc := r.ledger()
		return len(doc.On) == 0 && len(doc.Pending) == 0
	})

	// Turning it back on here puts it on no box: the person does.
	allowPlugin(t, dir)
	if got := r.state(t); got != "off" {
		t.Fatalf("after turning it back on: %s", got)
	}
}

func TestAPluginTurnedOffWhileItsBoxIsAwayComesOffWhenItIsBack(t *testing.T) {
	r := newPluginsRig(t)
	dir := toolsPlugin(t, r.agent, "1.0.0", []byte("#!/bin/sh\necho one\n"))
	allowPlugin(t, dir)
	r.putOn(t)

	addr := r.box.address
	r.box.stop()
	eventually(t, "devbox offline", func() bool { return stateOf(t, r.agent) == StateOffline })
	if resp, body := uiCall(t, r.agent, "POST", "/v1/plugins/acme/disable", r.token); resp.StatusCode != 200 {
		t.Fatalf("disable: %d %s", resp.StatusCode, body)
	}
	eventually(t, "the removal written down", func() bool { return len(r.ledger().Pending) == 1 })
	if !r.onBox() {
		t.Fatal("the box was away: nothing could have taken it off")
	}

	r.box.start(addr)
	eventually(t, "acme off the box once it is back", func() bool { return !r.onBox() })
	eventually(t, "the removal struck off", func() bool { return len(r.ledger().Pending) == 0 })
}

func TestAPluginWaitingToBeAllowedAgainStaysOnTheBoxes(t *testing.T) {
	r := newPluginsRig(t)
	dir := toolsPlugin(t, r.agent, "1.0.0", []byte("#!/bin/sh\necho one\n"))
	allowPlugin(t, dir)
	r.putOn(t)

	// Its files change (an update): off here until the person allows them,
	// but the box keeps the version that was allowed.
	toolsPlugin(t, r.agent, "2.0.0", []byte("#!/bin/sh\necho two\n"))
	// The box reconnecting has the agent look.
	addr := r.box.address
	r.box.stop()
	eventually(t, "devbox offline", func() bool { return stateOf(t, r.agent) == StateOffline })
	r.box.start(addr)
	eventually(t, "devbox back", func() bool { return stateOf(t, r.agent) == StateOnline })
	time.Sleep(500 * time.Millisecond)
	if !r.onBox() || len(r.ledger().Pending) != 0 || len(r.ledger().On["acme"]) != 1 {
		t.Fatalf("a changed plugin was taken off its box: %+v", r.ledger())
	}
	allowPlugin(t, dir)
	if got := r.state(t); got != "outdated" {
		t.Fatalf("its box once the new files are allowed: %s", got)
	}
}

func TestABoxFoundHoldingThePluginIsNoted(t *testing.T) {
	r := newPluginsRig(t)
	dir := toolsPlugin(t, r.agent, "1.0.0", []byte("#!/bin/sh\necho one\n"))
	allowPlugin(t, dir)
	r.putOn(t)
	// As if it had been put there before box-plugins.json was kept.
	os.Remove(filepath.Join(r.agent.dir, "box-plugins.json"))
	r.agent.stop()
	r.agent = startAgentConfig(t, r.agent.dir, &fakeNetworks{}, nil)
	eventually(t, "box online", func() bool { return stateOf(t, r.agent) == StateOnline })
	r.token = uiToken(t, r.agent)

	if got := r.state(t); got != "on" {
		t.Fatalf("state: %s", got)
	}
	if on := r.ledger().On["acme"]; len(on) != 1 || on[0] != "devbox" {
		t.Fatalf("box-plugins.json after looking: %+v", r.ledger())
	}
	if resp, body := uiCall(t, r.agent, "POST", "/v1/plugins/acme/disable", r.token); resp.StatusCode != 200 {
		t.Fatalf("disable: %d %s", resp.StatusCode, body)
	}
	eventually(t, "acme off the box", func() bool { return !r.onBox() })
}
