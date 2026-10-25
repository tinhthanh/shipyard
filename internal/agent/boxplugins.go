package agent

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cosscom/shipyard/internal/box"
	"github.com/cosscom/shipyard/internal/hooks"
	"github.com/cosscom/shipyard/internal/integrations"
	"github.com/cosscom/shipyard/internal/statefile"
)

// A plugin's tools on boxes (box/plugins.go): the person turns a plugin on
// for a box in Settings → Plugins, and the agent sends the box the plugin's
// folder as they allowed it here, with the hash they allowed. The box checks
// it and gives its own agents the tools. Its setup (macOS permissions, a
// browser extension) is started on the box's screen from here, for the
// person to finish there.
//
//	GET    /v1/plugins/{id}/boxes              each online box: is it there, the same, set up
//	POST   /v1/plugins/{id}/boxes/{box}        put it on the box, or update it there
//	DELETE /v1/plugins/{id}/boxes/{box}        take it off the box
//	POST   /v1/plugins/{id}/boxes/{box}/setup  start its setup on the box's screen
//	GET    /v1/plugins/{id}/boxes/{box}/settings  its settings on the box
//	PUT    /v1/plugins/{id}/boxes/{box}/settings  turn one on or off there

// PluginOnBox is one box's row for a plugin.
type PluginOnBox struct {
	Box string `json:"box"`
	// State: "off" (not there), "on" (the plugin as allowed here),
	// "outdated" (another version), "unsupported" (it does not run there:
	// another platform), "old_berthd" (the box cannot take plugins) or
	// "error".
	State    string `json:"state"`
	Platform string `json:"platform,omitempty"`
	Version  string `json:"version,omitempty"`
	// Setup is the plugin's setup on the box, while it is there and has one.
	Setup *integrations.PluginSetupState `json:"setup,omitempty"`
	Error string                         `json:"error,omitempty"`
}

// PluginBoxes is GET /v1/plugins/{id}/boxes.
type PluginBoxes struct {
	Plugin string `json:"plugin"`
	// Tools: the plugin gives agents MCP servers or skills; only then is
	// it something to put on a box.
	Tools bool `json:"tools"`
	// Enabled: it is on here, so it can be sent.
	Enabled bool `json:"enabled"`
	// Platforms its binaries run on; empty when they don't say.
	Platforms []string      `json:"platforms,omitempty"`
	Boxes     []PluginOnBox `json:"boxes"`
}

// localPlugin is the plugin in ~/.berth/plugins as the person allowed it.
type localPlugin struct {
	dir       string
	hash      string
	enabled   bool
	tools     bool
	platforms []string
	known     bool
	version   string
}

func (a *Agent) localPlugin(id string) (localPlugin, error) {
	dir := filepath.Join(a.pluginsDir(), id)
	manifest, state, err := hooks.AllowedPluginManifest(dir)
	if err != nil {
		return localPlugin{}, marketErr(http.StatusNotFound, "plugin_not_installed", "%s is not installed on this computer", id)
	}
	lp := localPlugin{dir: dir, hash: state.Allowed, enabled: state.Enabled}
	if t, err := integrations.ReadPluginTools(dir); err == nil {
		lp.tools = len(t.MCP) > 0 || len(t.Skills) > 0
		lp.platforms, lp.known = integrations.PluginPlatforms(t)
	}
	lp.version = manifestVersion(manifest)
	return lp, nil
}

func manifestVersion(manifest []byte) string {
	var m struct {
		Version string `json:"version"`
	}
	_ = json.Unmarshal(manifest, &m)
	return m.Version
}

func (a *Agent) pluginBoxes(ctx context.Context, id string) (PluginBoxes, error) {
	lp, err := a.localPlugin(id)
	if err != nil {
		return PluginBoxes{}, err
	}
	out := PluginBoxes{Plugin: id, Tools: lp.tools, Enabled: lp.enabled, Platforms: lp.platforms, Boxes: []PluginOnBox{}}
	if !lp.tools {
		return out, nil
	}
	a.sync()
	var names []string
	for _, b := range a.status().Boxes {
		// A box on this computer shares its ~/.berth/plugins: its agents
		// have the plugin already, and taking it off there would take it
		// off here.
		if b.State == StateOnline && !b.Local {
			names = append(names, b.Name)
		}
	}
	rows := make([]PluginOnBox, len(names))
	var wg sync.WaitGroup
	for i, name := range names {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			rows[i] = a.pluginOnBox(ctx, id, name, lp)
		}()
	}
	wg.Wait()
	sort.Slice(rows, func(i, j int) bool { return rows[i].Box < rows[j].Box })
	out.Boxes = rows
	// A box holding exactly the version allowed here got it from here (or
	// before box-plugins.json was kept): note it, so turning the plugin
	// off takes it off there too.
	if lp.enabled {
		for _, r := range rows {
			if r.State == "on" {
				a.notePluginSeenOnBox(id, r.Box)
			}
		}
	}
	return out, nil
}

func (a *Agent) pluginOnBox(ctx context.Context, id, name string, lp localPlugin) PluginOnBox {
	row := PluginOnBox{Box: name}
	bc, err := a.helperBox(name)
	if err != nil {
		row.State, row.Error = "error", err.Error()
		return row
	}
	list, err := bc.BoxPlugins(ctx)
	if err != nil {
		var he *box.HelperError
		if errors.As(err, &he) && he.NoHelpers {
			row.State, row.Error = "old_berthd", "this box runs an older berthd, which cannot take plugins; update it"
			return row
		}
		row.State, row.Error = "error", err.Error()
		return row
	}
	row.Platform = list.Platform
	for _, bp := range list.Plugins {
		if bp.ID != id {
			continue
		}
		row.Version = bp.Version
		row.State = "outdated"
		if bp.Enabled && bp.Hash == lp.hash {
			row.State = "on"
		}
		if bp.Enabled && bp.Setup {
			if st, err := bc.BoxPluginSetup(ctx, id); err == nil {
				row.Setup = &st
			}
		}
		return row
	}
	row.State = "off"
	if !integrations.PluginRunsOn(lp.platforms, lp.known, list.Platform) {
		row.State, row.Error = "unsupported", "this plugin is built for "+strings.Join(lp.platforms, ", ")+"; "+name+" is "+list.Platform
	}
	return row
}

// localBoxName says whether name is a box on this computer, which shares
// its plugins with it.
func (a *Agent) localBoxName(name string) bool {
	for _, b := range a.status().Boxes {
		if b.Name == name {
			return b.Local
		}
	}
	return false
}

func errThisComputer(name string) error {
	return marketErr(http.StatusConflict, "box_is_this_computer", "%s is this computer: its agents have this computer's plugins already", name)
}

// installPluginOnBox sends the plugin, as allowed here, to the box.
func (a *Agent) installPluginOnBox(ctx context.Context, id, name string) (PluginOnBox, error) {
	if a.localBoxName(name) {
		return PluginOnBox{}, errThisComputer(name)
	}
	lp, err := a.localPlugin(id)
	if err != nil {
		return PluginOnBox{}, err
	}
	if !lp.enabled {
		return PluginOnBox{}, marketErr(http.StatusConflict, "plugin_not_allowed", "%s is off here, or changed since you allowed it; turn it on in Settings → Plugins first", id)
	}
	if !lp.tools {
		return PluginOnBox{}, marketErr(http.StatusConflict, "plugin_no_tools", "%s gives agents no tools to put on a box", id)
	}
	bc, err := a.helperBox(name)
	if err != nil {
		return PluginOnBox{}, err
	}
	pr, pw := io.Pipe()
	go func() { pw.CloseWithError(packPlugin(lp.dir, pw)) }()
	if _, err := bc.InstallBoxPlugin(ctx, id, lp.hash, pr); err != nil {
		pr.CloseWithError(err)
		return PluginOnBox{}, a.boxProblem(err)
	}
	a.notePluginOnBox(id, name, true)
	return a.pluginOnBox(ctx, id, name, lp), nil
}

func (a *Agent) removePluginFromBox(ctx context.Context, id, name string) (PluginOnBox, error) {
	if a.localBoxName(name) {
		return PluginOnBox{}, errThisComputer(name)
	}
	bc, err := a.helperBox(name)
	if err != nil {
		return PluginOnBox{}, err
	}
	if err := bc.RemoveBoxPlugin(ctx, id); err != nil {
		return PluginOnBox{}, a.boxProblem(err)
	}
	a.notePluginOnBox(id, name, false)
	return PluginOnBox{Box: name, State: "off"}, nil
}

func (a *Agent) startPluginSetupOnBox(ctx context.Context, id, name string) (box.PluginSetupStarted, error) {
	bc, err := a.helperBox(name)
	if err != nil {
		return box.PluginSetupStarted{}, err
	}
	out, err := bc.StartBoxPluginSetup(ctx, id)
	if err != nil {
		return box.PluginSetupStarted{}, a.boxProblem(err)
	}
	return out, nil
}

// pluginSettingsOnBox lists a plugin's settings on a box, or with key set,
// turns that one on or off there first. A box's settings are its own: which
// Chrome profiles agents may use differs from one computer to the next.
func (a *Agent) pluginSettingsOnBox(ctx context.Context, id, name, key string, value *bool) (box.BoxPluginSettings, error) {
	bc, err := a.helperBox(name)
	if err != nil {
		return box.BoxPluginSettings{}, err
	}
	var out box.BoxPluginSettings
	if value != nil {
		out, err = bc.SetBoxPluginSetting(ctx, id, key, *value)
	} else {
		out, err = bc.BoxPluginSettings(ctx, id)
	}
	if err != nil {
		return box.BoxPluginSettings{}, a.boxProblem(err)
	}
	return out, nil
}

// packPlugin writes the plugin's folder as a tar.gz: its files with their
// modes and its links, but not "disabled" (that is the laptop's switch) or
// Finder's litter.
func packPlugin(dir string, w io.Writer) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil || rel == "." {
			return err
		}
		name := filepath.ToSlash(rel)
		if name == "disabled" || d.Name() == ".DS_Store" {
			return nil
		}
		info, err := os.Lstat(p)
		if err != nil {
			return err
		}
		link := ""
		if info.Mode()&fs.ModeSymlink != 0 {
			if link, err = os.Readlink(p); err != nil {
				return err
			}
		}
		h, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return err
		}
		h.Name = name
		if info.IsDir() {
			h.Name += "/"
		}
		h.Uname, h.Gname, h.Uid, h.Gid = "", "", 0, 0
		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(tw, f)
		return err
	})
	if err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

func (a *Agent) boxPluginRoutes(mux *http.ServeMux) {
	plugin := func(w http.ResponseWriter, r *http.Request) (string, bool) {
		id := r.PathValue("id")
		if !pluginIDPattern.MatchString(id) {
			writeError(w, http.StatusBadRequest, "not a plugin id")
			return "", false
		}
		return id, true
	}
	mux.HandleFunc("GET /v1/plugins/{id}/boxes", func(w http.ResponseWriter, r *http.Request) {
		id, ok := plugin(w, r)
		if !ok {
			return
		}
		out, err := a.pluginBoxes(r.Context(), id)
		if err != nil {
			writeMarketError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	mux.HandleFunc("POST /v1/plugins/{id}/boxes/{box}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := plugin(w, r)
		if !ok {
			return
		}
		done, err := a.work.begin("put " + id + " on " + r.PathValue("box"))
		if err != nil {
			writeMarketError(w, marketErr(http.StatusServiceUnavailable, "agent_restarting", "%v", err))
			return
		}
		defer done()
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
		defer cancel()
		row, err := a.installPluginOnBox(ctx, id, r.PathValue("box"))
		if err != nil {
			writeMarketError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, row)
	})
	mux.HandleFunc("DELETE /v1/plugins/{id}/boxes/{box}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := plugin(w, r)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), time.Minute)
		defer cancel()
		row, err := a.removePluginFromBox(ctx, id, r.PathValue("box"))
		if err != nil {
			writeMarketError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, row)
	})
	mux.HandleFunc("POST /v1/plugins/{id}/boxes/{box}/setup", func(w http.ResponseWriter, r *http.Request) {
		id, ok := plugin(w, r)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		out, err := a.startPluginSetupOnBox(ctx, id, r.PathValue("box"))
		if err != nil {
			writeMarketError(w, err)
			return
		}
		out.Started = true
		writeJSON(w, http.StatusOK, out)
	})
	mux.HandleFunc("GET /v1/plugins/{id}/boxes/{box}/settings", func(w http.ResponseWriter, r *http.Request) {
		id, ok := plugin(w, r)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		out, err := a.pluginSettingsOnBox(ctx, id, r.PathValue("box"), "", nil)
		if err != nil {
			writeMarketError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	mux.HandleFunc("PUT /v1/plugins/{id}/boxes/{box}/settings", func(w http.ResponseWriter, r *http.Request) {
		id, ok := plugin(w, r)
		if !ok {
			return
		}
		var req struct {
			Key   string `json:"key"`
			Value *bool  `json:"value"`
		}
		if !decodeBody(w, r, &req) {
			return
		}
		if req.Key == "" || req.Value == nil {
			writeError(w, http.StatusBadRequest, "send a key and a value")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		out, err := a.pluginSettingsOnBox(ctx, id, r.PathValue("box"), req.Key, req.Value)
		if err != nil {
			writeMarketError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
}

// A plugin's tools do not outlive it on the boxes either. The boxes a
// plugin was put on are noted in box-plugins.json in the agent's state
// directory; when the plugin is turned off here, or uninstalled, it is
// taken off each of them: at once from those online, and from the others
// when they next connect (the note is kept until then, for the next agent
// too). A plugin whose files changed is off here until the person allows
// the new ones, but the boxes keep the version they were allowed: it is
// held, as a plugin's helper is, and Update sends the new one. Turning a
// plugin back on puts it on no box: the person does, box by box.

type boxPluginRemoval struct {
	Plugin string    `json:"plugin"`
	Box    string    `json:"box"`
	Since  time.Time `json:"since"`
}

type boxPluginLedgerDoc struct {
	// On: the boxes each plugin was put on from here.
	On      map[string][]string `json:"on,omitempty"`
	Pending []boxPluginRemoval  `json:"pending,omitempty"`
}

type boxPluginLedger struct {
	once sync.Once
	mu   sync.Mutex
	path string
	doc  boxPluginLedgerDoc
	wake chan struct{}
}

const boxPluginTidyEvery = 2 * time.Minute

func (a *Agent) boxPluginLedger() *boxPluginLedger {
	l := &a.boxPlugins
	l.once.Do(func() {
		l.path = filepath.Join(a.cfg.Dir, "box-plugins.json")
		l.wake = make(chan struct{}, 1)
		if b, err := os.ReadFile(l.path); err == nil {
			_ = json.Unmarshal(b, &l.doc)
		}
	})
	return l
}

// change edits the note and saves it when fn says it changed.
func (l *boxPluginLedger) change(fn func(doc *boxPluginLedgerDoc) bool) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.doc.On == nil {
		l.doc.On = map[string][]string{}
	}
	if !fn(&l.doc) {
		return nil
	}
	b, err := json.MarshalIndent(l.doc, "", "  ")
	if err != nil {
		return err
	}
	return statefile.Write(l.path, append(b, '\n'))
}

func (l *boxPluginLedger) snapshot() boxPluginLedgerDoc {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := boxPluginLedgerDoc{On: map[string][]string{}, Pending: append([]boxPluginRemoval(nil), l.doc.Pending...)}
	for k, v := range l.doc.On {
		out.On[k] = append([]string(nil), v...)
	}
	return out
}

func (a *Agent) notePluginOnBox(id, name string, on bool) {
	err := a.boxPluginLedger().change(func(doc *boxPluginLedgerDoc) bool {
		boxes := slices.DeleteFunc(doc.On[id], func(b string) bool { return b == name })
		if on {
			boxes = append(boxes, name)
		}
		if len(boxes) == 0 {
			delete(doc.On, id)
		} else {
			doc.On[id] = boxes
		}
		// Put back on by hand: no longer to be taken off.
		doc.Pending = slices.DeleteFunc(doc.Pending, func(p boxPluginRemoval) bool { return p.Plugin == id && p.Box == name })
		return true
	})
	if err != nil {
		a.cfg.Log.Printf("plugins on boxes: %v", err)
	}
}

// notePluginSeenOnBox is notePluginOnBox for a box found holding the
// plugin, written only when it is news.
func (a *Agent) notePluginSeenOnBox(id, name string) {
	if slices.Contains(a.boxPluginLedger().snapshot().On[id], name) {
		return
	}
	a.notePluginOnBox(id, name, true)
}

// wakeBoxPluginTidy has the tidy loop look now: a plugin was turned off or
// uninstalled.
func (a *Agent) wakeBoxPluginTidy() {
	select {
	case a.boxPluginLedger().wake <- struct{}{}:
	default:
	}
}

// keepBoxPluginsTidy takes plugins that are off here off the boxes: when
// the agent starts, when told, when a box connects, and now and then.
func (a *Agent) keepBoxPluginsTidy(ctx context.Context) {
	l := a.boxPluginLedger()
	stop := a.bus.Observe(func(e Event) {
		if e.Type == EventBoxConnected {
			a.wakeBoxPluginTidy()
		}
	})
	defer stop()
	ticker := time.NewTicker(boxPluginTidyEvery)
	defer ticker.Stop()
	for {
		a.sweepBoxPlugins()
		a.runBoxPluginRemovals(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-l.wake:
		}
	}
}

// sweepBoxPlugins queues for removal every box of a plugin that is off
// here or gone; one waiting for the person to allow new files is held.
func (a *Agent) sweepBoxPlugins() {
	for id, boxes := range a.boxPluginLedger().snapshot().On {
		_, state, err := hooks.AllowedPluginManifest(filepath.Join(a.pluginsDir(), id))
		if err == nil && (state.Enabled || state.Changed) {
			continue
		}
		now := time.Now().UTC()
		_ = a.boxPluginLedger().change(func(doc *boxPluginLedgerDoc) bool {
			for _, b := range boxes {
				if !slices.ContainsFunc(doc.Pending, func(p boxPluginRemoval) bool { return p.Plugin == id && p.Box == b }) {
					doc.Pending = append(doc.Pending, boxPluginRemoval{Plugin: id, Box: b, Since: now})
				}
			}
			delete(doc.On, id)
			return true
		})
	}
}

// runBoxPluginRemovals takes plugins off the boxes they are queued for. A
// box that is away keeps its turn; one no longer paired, or that never
// could hold plugins, is let go.
func (a *Agent) runBoxPluginRemovals(ctx context.Context) {
	for _, p := range a.boxPluginLedger().snapshot().Pending {
		done := false
		if _, ok := a.client(p.Box); !ok {
			done = true
		} else if _, _, away := a.away(p.Box); !away {
			if bc, err := a.helperBox(p.Box); err == nil {
				rctx, cancel := context.WithTimeout(ctx, 30*time.Second)
				err = bc.RemoveBoxPlugin(rctx, p.Plugin)
				cancel()
				var he *box.HelperError
				switch {
				case err == nil:
					a.cfg.Log.Printf("plugins on boxes: took %s off %s: it is off here", p.Plugin, p.Box)
					done = true
				case errors.As(err, &he) && he.NoHelpers:
					done = true
				default:
					a.cfg.Log.Printf("plugins on boxes: taking %s off %s: %v", p.Plugin, p.Box, err)
				}
			}
		}
		if done {
			_ = a.boxPluginLedger().change(func(doc *boxPluginLedgerDoc) bool {
				doc.Pending = slices.DeleteFunc(doc.Pending, func(q boxPluginRemoval) bool { return q.Plugin == p.Plugin && q.Box == p.Box })
				return true
			})
		}
	}
}
