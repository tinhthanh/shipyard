package box

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/cosscom/shipyard/internal/hooks"
	"github.com/cosscom/shipyard/internal/integrations"
	"github.com/cosscom/shipyard/internal/wire"
)

// Plugins' tools on a box: the MCP servers and skills a plugin gives agents
// (internal/integrations/plugins.go), there for the box's own agents too.
// The laptop sends the plugin's folder as the person allowed it, with the
// hash they allowed; the box takes it only if it is that folder, every file
// the manifest pins hashes to its pin, and its binaries run here. It then
// records the grant in its own ~/.berth/plugins.json, as allowing the
// plugin on the laptop does there, and syncs the agents' configs. The setup
// a plugin needs (macOS permissions, a browser extension) runs on the box,
// in the session of the person logged in at it, for them to finish there.
//
//	GET    /v1/plugins              the box's platform and the plugins on it
//	PUT    /v1/plugins/{id}         install or replace one (a paired laptop's upload)
//	DELETE /v1/plugins/{id}         remove one
//	GET    /v1/plugins/{id}/setup   its setup's state on the box
//	POST   /v1/plugins/{id}/setup   start its setup on the box's screen

// PluginHashHeader carries the hash of the plugin the person allowed
// (hooks.PluginHash) with an upload.
const PluginHashHeader = "X-Berth-Plugin-Hash"

// What an upload may hold: a plugin is a few binaries and skills.
const (
	maxPluginArchive = 256 << 20
	maxPluginFiles   = 20000
)

var pluginID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)

// BoxPlugins keeps the plugins in Dir (~/.berth/plugins) for the box's
// agents. Home and Exe are what agents' configs are synced against: the
// person's home and berthd's own path.
type BoxPlugins struct {
	Dir  string
	Home string
	Exe  string
	// One change at a time: two uploads of a plugin, or one and a removal,
	// would swap folders under each other.
	mu sync.Mutex
}

// BoxPlugin is one plugin on the box.
type BoxPlugin struct {
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`
	Version string `json:"version,omitempty"`
	// Hash identifies its files as they are (hooks.PluginHash).
	Hash string `json:"hash"`
	// Enabled: allowed here, and still what was allowed.
	Enabled bool `json:"enabled"`
	// Tools: it gives agents MCP servers or skills.
	Tools bool `json:"tools"`
	// Setup: it has a setup step to run on the box.
	Setup bool `json:"setup"`
}

// BoxPluginList is GET /v1/plugins.
type BoxPluginList struct {
	Platform string      `json:"platform"`
	Plugins  []BoxPlugin `json:"plugins"`
}

func (p *BoxPlugins) list() BoxPluginList {
	out := BoxPluginList{Platform: HelperPlatform(), Plugins: []BoxPlugin{}}
	entries, _ := os.ReadDir(p.Dir)
	for _, e := range entries {
		if !e.IsDir() || !pluginID.MatchString(e.Name()) {
			continue
		}
		if bp, ok := p.describe(e.Name()); ok {
			out.Plugins = append(out.Plugins, bp)
		}
	}
	sort.Slice(out.Plugins, func(i, j int) bool { return out.Plugins[i].ID < out.Plugins[j].ID })
	return out
}

func (p *BoxPlugins) describe(id string) (BoxPlugin, bool) {
	dir := filepath.Join(p.Dir, id)
	raw, err := os.ReadFile(filepath.Join(dir, hooks.PluginManifest))
	if err != nil {
		return BoxPlugin{}, false
	}
	var m struct {
		Name    string          `json:"name"`
		Version string          `json:"version"`
		MCP     json.RawMessage `json:"mcp"`
		Skills  json.RawMessage `json:"skills"`
		Setup   json.RawMessage `json:"setup"`
	}
	_ = json.Unmarshal(raw, &m)
	hash, _ := hooks.PluginHash(dir)
	return BoxPlugin{
		ID: id, Name: m.Name, Version: m.Version, Hash: hash,
		Enabled: hooks.PluginEnabled(dir),
		Tools:   len(m.MCP) > 2 || len(m.Skills) > 2,
		Setup:   len(m.Setup) > 0 && string(m.Setup) != "null",
	}, true
}

// install puts the plugin in archive (a tar.gz of its folder) in place of
// any it replaces, once it is the plugin of hash and runs here, allows it
// and gives agents its tools.
func (p *BoxPlugins) install(id, hash string, archive io.Reader) (BoxPlugin, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := os.MkdirAll(p.Dir, 0o700); err != nil {
		return BoxPlugin{}, err
	}
	stage, err := os.MkdirTemp(p.Dir, ".incoming-")
	if err != nil {
		return BoxPlugin{}, err
	}
	defer os.RemoveAll(stage)
	// Staged under its own id: the checks read the id from the folder name.
	dir := filepath.Join(stage, id)
	if err := os.Mkdir(dir, 0o755); err != nil {
		return BoxPlugin{}, err
	}
	if err := unpackPlugin(dir, archive); err != nil {
		return BoxPlugin{}, helperProblem(http.StatusBadRequest, "plugin_invalid", "%v", err)
	}
	got, err := hooks.PluginHash(dir)
	if err != nil {
		return BoxPlugin{}, helperProblem(http.StatusBadRequest, "plugin_invalid", "%v", err)
	}
	if got != hash {
		return BoxPlugin{}, helperProblem(http.StatusConflict, "plugin_changed", "the plugin sent is not the one that was allowed (%s, expected %s)", got, hash)
	}
	tools, err := integrations.VerifyPluginTools(dir)
	if err != nil {
		return BoxPlugin{}, helperProblem(http.StatusBadRequest, "plugin_invalid", "%v", err)
	}
	if platforms, known := integrations.PluginPlatforms(tools); !integrations.PluginRunsOn(platforms, known, HelperPlatform()) {
		return BoxPlugin{}, helperProblem(http.StatusConflict, "plugin_unsupported", "this plugin is built for %s; this box is %s", strings.Join(platforms, ", "), HelperPlatform())
	}
	final := filepath.Join(p.Dir, id)
	if _, err := os.Lstat(final); err == nil {
		if err := os.Rename(final, filepath.Join(stage, ".replaced")); err != nil {
			return BoxPlugin{}, err
		}
	}
	if err := os.Rename(dir, final); err != nil {
		return BoxPlugin{}, err
	}
	if err := hooks.AllowPlugin(final, hash); err != nil {
		return BoxPlugin{}, err
	}
	p.sync()
	bp, _ := p.describe(id)
	return bp, nil
}

// remove takes the plugin off the box: its grant, its folder, and its tools
// from agents' configs.
func (p *BoxPlugins) remove(id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	dir := filepath.Join(p.Dir, id)
	if err := hooks.ForbidPlugin(dir); err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	p.sync()
	return nil
}

// sync gives agents the tools of the plugins that are on now, at once
// (berthd also does it every half minute).
func (p *BoxPlugins) sync() {
	if p.Home == "" || p.Exe == "" {
		return
	}
	_, _ = integrations.SyncPluginTools(p.Home, p.Dir, p.Exe)
}

// unpackPlugin writes a tar.gz of a plugin's folder into dir, through an
// os.Root so nothing lands outside it: no absolute paths, no "..", and
// symbolic links (an app bundle has some) only to somewhere inside.
// macMetadata says whether a path in an archive is a Mac's metadata, not a
// file: an AppleDouble "._name", or anything under "__MACOSX/".
func macMetadata(name string) bool {
	for _, part := range strings.Split(name, "/") {
		if strings.HasPrefix(part, "._") || part == "__MACOSX" {
			return true
		}
	}
	return false
}

// UnpackPlugin is unpackPlugin for the laptop's agent, which installs a
// plugin from a release archive the same careful way.
func UnpackPlugin(dir string, archive io.Reader) error { return unpackPlugin(dir, archive) }

func unpackPlugin(dir string, archive io.Reader) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	gz, err := gzip.NewReader(io.LimitReader(archive, maxPluginArchive+1))
	if err != nil {
		return fmt.Errorf("the upload is not a gzip'd tar of the plugin: %w", err)
	}
	tr := tar.NewReader(gz)
	var files int
	var written int64
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("reading the plugin: %w", err)
		}
		if files++; files > maxPluginFiles {
			return fmt.Errorf("the plugin has more than %d files", maxPluginFiles)
		}
		name, ok := insidePath(h.Name)
		if !ok {
			if strings.Trim(path.Clean(h.Name), "./") == "" {
				continue
			}
			return fmt.Errorf("%q is not a path inside the plugin", h.Name)
		}
		if macMetadata(name) {
			// A Mac's tar adds "._x" beside each file with extended
			// attributes: none of the plugin's, and inside an app bundle
			// they'd break its signature.
			continue
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := root.MkdirAll(name, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if parent := path.Dir(name); parent != "." {
				if err := root.MkdirAll(parent, 0o755); err != nil {
					return err
				}
			}
			mode := fs.FileMode(0o644)
			if h.Mode&0o111 != 0 {
				mode = 0o755
			}
			f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
			if err != nil {
				return err
			}
			n, err := io.Copy(f, io.LimitReader(tr, maxPluginArchive-written+1))
			if cerr := f.Close(); err == nil {
				err = cerr
			}
			if err != nil {
				return err
			}
			if written += n; written > maxPluginArchive {
				return fmt.Errorf("the plugin is larger than %d MB", maxPluginArchive>>20)
			}
		case tar.TypeSymlink:
			target := path.Join(path.Dir(name), h.Linkname)
			if _, ok := insidePath(target); !ok || path.IsAbs(h.Linkname) {
				return fmt.Errorf("%q links outside the plugin", h.Name)
			}
			if parent := path.Dir(name); parent != "." {
				if err := root.MkdirAll(parent, 0o755); err != nil {
					return err
				}
			}
			if err := root.Symlink(h.Linkname, name); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%q is neither a file, a folder nor a link", h.Name)
		}
	}
	return nil
}

// insidePath is name as a clean relative path inside the plugin, refusing
// absolute paths and any "..".
func insidePath(name string) (string, bool) {
	if name == "" || path.IsAbs(name) || strings.HasPrefix(name, "/") {
		return "", false
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return "", false
		}
	}
	clean := path.Clean(name)
	if clean == "." || clean == "" {
		return "", false
	}
	return clean, true
}

// The routes.

func (b *Box) mountPlugins(route func(string, func(http.ResponseWriter, *http.Request) error)) {
	route("GET /v1/plugins", b.listBoxPlugins)
	route("PUT /v1/plugins/{id}", b.putBoxPlugin)
	route("DELETE /v1/plugins/{id}", b.deleteBoxPlugin)
	route("GET /v1/plugins/{id}/setup", b.boxPluginSetup)
	route("POST /v1/plugins/{id}/setup", b.startBoxPluginSetup)
}

// MountPlugins registers the plugin routes alone on s, for a laptop agent's
// tests that stand up a box with only them.
func (b *Box) MountPlugins(s *wire.Server) {
	b.mountPlugins(func(pattern string, h func(http.ResponseWriter, *http.Request) error) {
		s.Handle(pattern, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := h(w, r); err != nil {
				writeErr(w, err)
			}
		}))
	})
}

func (b *Box) boxPlugins(id string) (*BoxPlugins, error) {
	if b.Plugins == nil {
		return nil, helperProblem(http.StatusNotImplemented, CodeUnsupported, "this box cannot take plugins")
	}
	if id != "" && !pluginID.MatchString(id) {
		return nil, helperProblem(http.StatusBadRequest, "plugin_invalid", "%q is not a plugin's id", clip(id, 80))
	}
	return b.Plugins, nil
}

func (b *Box) listBoxPlugins(w http.ResponseWriter, r *http.Request) error {
	p, err := b.boxPlugins("")
	if err != nil {
		return err
	}
	writeJSON(w, p.list())
	return nil
}

// putBoxPlugin installs a plugin from a paired laptop: the laptop is where
// the person allowed it. What runs on the box itself (an agent in a
// session) cannot give the box's agents new tools.
func (b *Box) putBoxPlugin(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	p, err := b.boxPlugins(id)
	if err != nil {
		return err
	}
	if wire.IsLocal(r.Context()) {
		return helperProblem(http.StatusForbidden, CodeRefused, "a plugin is put on a box from a paired laptop, where it was allowed")
	}
	hash := r.Header.Get(PluginHashHeader)
	if !strings.HasPrefix(hash, "sha256:") {
		return helperProblem(http.StatusBadRequest, "plugin_invalid", "the %s header must be the allowed plugin's hash", PluginHashHeader)
	}
	bp, err := p.install(id, hash, r.Body)
	if err != nil {
		return err
	}
	b.publish(r, "plugin.installed", map[string]any{"plugin": id, "version": bp.Version})
	writeJSON(w, bp)
	return nil
}

func (b *Box) deleteBoxPlugin(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	p, err := b.boxPlugins(id)
	if err != nil {
		return err
	}
	if wire.IsLocal(r.Context()) {
		return helperProblem(http.StatusForbidden, CodeRefused, "a plugin is taken off a box from a paired laptop")
	}
	if err := p.remove(id); err != nil {
		return err
	}
	b.publish(r, "plugin.removed", map[string]any{"plugin": id})
	writeJSON(w, map[string]string{"removed": id})
	return nil
}

func (b *Box) boxPluginSetup(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	p, err := b.boxPlugins(id)
	if err != nil {
		return err
	}
	st, err := integrations.PluginSetupStatus(r.Context(), p.Dir, id)
	if err != nil {
		return setupProblem(err)
	}
	if !st.Ready {
		st.Locked = screenLocked(r.Context())
	}
	writeJSON(w, st)
	return nil
}

// startBoxPluginSetup opens the plugin's setup on the box's screen, for the
// person at it (or reaching it by remote desktop) to finish.
func (b *Box) startBoxPluginSetup(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	p, err := b.boxPlugins(id)
	if err != nil {
		return err
	}
	if wire.IsLocal(r.Context()) {
		return helperProblem(http.StatusForbidden, CodeRefused, "a plugin's setup on a box is started from a paired laptop")
	}
	if err := integrations.StartPluginSetup(p.Dir, id); err != nil {
		return setupProblem(err)
	}
	// It opens all the same: the window waits there for whoever unlocks it.
	writeJSON(w, map[string]bool{"started": true, "locked": screenLocked(r.Context())})
	return nil
}

func setupProblem(err error) error {
	switch {
	case errors.Is(err, integrations.ErrPluginOff):
		return helperProblem(http.StatusConflict, "plugin_off", "the plugin is not on this box")
	case errors.Is(err, integrations.ErrNoSetup):
		return helperProblem(http.StatusConflict, "plugin_no_setup", "the plugin has no setup")
	}
	return err
}

// client side

// BoxPlugins lists the plugins on the box and its platform.
func (c *Client) BoxPlugins(ctx context.Context) (out BoxPluginList, err error) {
	return out, c.helperCall(ctx, http.MethodGet, "/v1/plugins", nil, nil, &out)
}

// InstallBoxPlugin uploads a plugin's folder (a tar.gz) with the hash the
// person allowed it with.
func (c *Client) InstallBoxPlugin(ctx context.Context, id, hash string, archive io.Reader) (out BoxPlugin, err error) {
	header := http.Header{PluginHashHeader: {hash}, "Content-Type": {"application/gzip"}}
	return out, c.helperCall(ctx, http.MethodPut, "/v1/plugins/"+id, header, archive, &out)
}

// RemoveBoxPlugin takes a plugin off the box.
func (c *Client) RemoveBoxPlugin(ctx context.Context, id string) error {
	return c.helperCall(ctx, http.MethodDelete, "/v1/plugins/"+id, nil, nil, nil)
}

// BoxPluginSetup is the state of a plugin's setup on the box.
func (c *Client) BoxPluginSetup(ctx context.Context, id string) (out integrations.PluginSetupState, err error) {
	return out, c.helperCall(ctx, http.MethodGet, "/v1/plugins/"+id+"/setup", nil, nil, &out)
}

// PluginSetupStarted is what starting a plugin's setup on a box says: Locked,
// the box's screen is locked, so the window waits for whoever unlocks it.
type PluginSetupStarted struct {
	Started bool `json:"started"`
	Locked  bool `json:"locked,omitempty"`
}

// StartBoxPluginSetup opens a plugin's setup on the box's screen.
func (c *Client) StartBoxPluginSetup(ctx context.Context, id string) (out PluginSetupStarted, err error) {
	return out, c.helperCall(ctx, http.MethodPost, "/v1/plugins/"+id+"/setup", nil, nil, &out)
}
