package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// A plugin someone installed from its release by hand (Install from URL, or
// a .tar.gz unpacked into ~/.berth/plugins) before the listing had it is
// the listing's own when its files are exactly a release's: then the app
// offers its updates, instead of asking to replace a folder it takes for
// someone's own.
//
// When the listing is read, each listed plugin (listed by its archive)
// that is installed with no record of where it came from is checked once
// against the release of the version it has: the listed archive when the
// versions are the same, or else the release of that version beside it
// (../<id>-v<version>/<id>-<version>[-<os>-<arch>].tar.gz, as
// vetgo-releases names them, with its .sha256), on the listing's own
// address. Every file the archive holds must be in the folder with the same
// SHA-256; files the plugin wrote beside them since are its own business. A
// match is recorded as if the listing had installed it; anything else
// leaves the folder someone's own, and is not checked again for that
// version until the agent restarts. Nothing in the folder is changed, and
// no permission either.

// adoptTimeout bounds the checks one reading of the listing makes.
const adoptTimeout = 90 * time.Second

// adoptTried is what was checked and didn't match, by user folder, plugin
// and version, so a listing read again doesn't download the same archives.
var adoptTried sync.Map

// adoptInstalled records the listed plugins installed by hand from a
// release as the listing's, and returns sources with them.
func (a *Agent) adoptInstalled(ctx context.Context, registry *url.URL, entries []MarketPlugin, sources map[string]pluginSource) map[string]pluginSource {
	var todo []MarketPlugin
	for _, p := range entries {
		if p.Archive != nil && p.Error == "" && sources[p.ID].Version == "" {
			todo = append(todo, p)
		}
	}
	if len(todo) == 0 {
		return sources
	}
	a.market.change.Lock()
	defer a.market.change.Unlock()
	// Read again: an install may have written it since.
	sources = a.readPluginSources()
	ctx, cancel := context.WithTimeout(ctx, adoptTimeout)
	defer cancel()
	changed := false
	for _, p := range todo {
		if _, ok := sources[p.ID]; ok {
			continue
		}
		st := a.installedPlugin(p.ID, sources)
		if !st.present || !pluginVersionPattern.MatchString(st.version) {
			continue
		}
		key := a.cfg.UserDir + "\x00" + p.ID + "\x00" + st.version
		if _, tried := adoptTried.Load(key); tried {
			continue
		}
		src, ok := a.matchRelease(ctx, registry, p, st.version)
		if !ok {
			if ctx.Err() == nil {
				adoptTried.Store(key, true)
			}
			continue
		}
		sources[p.ID] = src
		changed = true
		a.cfg.Log.Printf("plugin %s %s: its files are the release's, so the listing updates it from now on", p.ID, st.version)
	}
	if changed {
		if err := a.writePluginSources(sources); err != nil {
			a.cfg.Log.Printf("plugin listing: recording plugins installed by hand: %v", err)
		}
	}
	return sources
}

// matchRelease checks ~/.berth/plugins/<id> against the release of version,
// and returns what to record when every file of it is there unchanged.
func (a *Agent) matchRelease(ctx context.Context, registry *url.URL, p MarketPlugin, version string) (pluginSource, bool) {
	dir := filepath.Join(a.pluginsDir(), p.ID)
	if fi, err := os.Lstat(dir); err != nil || !fi.IsDir() {
		return pluginSource{}, false
	}
	type candidate struct {
		url  *url.URL
		sum  string
		size int64
	}
	var candidates []candidate
	if version == p.Version {
		if u, err := fileURL(registry, p.Archive.URL); err == nil {
			candidates = append(candidates, candidate{u, p.Archive.SHA256, p.Archive.Size})
		}
	} else {
		names := []string{p.ID + "-" + version + ".tar.gz"}
		for _, k := range platformKeys(goos, goarch) {
			names = append(names, p.ID+"-"+version+"-"+k+".tar.gz")
		}
		for _, n := range names {
			if u, err := fileURL(registry, "../"+p.ID+"-v"+version+"/"+n); err == nil {
				candidates = append(candidates, candidate{url: u})
			}
		}
	}
	installed, err := os.OpenRoot(dir)
	if err != nil {
		return pluginSource{}, false
	}
	defer installed.Close()
	for _, c := range candidates {
		if c.sum == "" {
			// A release beside the listing: its checksum is published with it.
			side, err := marketGet(ctx, registry, &url.URL{Scheme: c.url.Scheme, Host: c.url.Host, Path: c.url.Path + ".sha256"}, 4<<10)
			if err != nil {
				continue
			}
			if f := strings.Fields(string(side)); len(f) > 0 && sha256Pattern.MatchString(strings.ToLower(f[0])) {
				c.sum = strings.ToLower(f[0])
			} else {
				continue
			}
		}
		max := c.size
		if max == 0 {
			max = maxMarketArchive
		}
		data, err := marketGet(ctx, registry, c.url, max)
		if err != nil || int64(len(data)) > max {
			continue
		}
		if sum := sha256.Sum256(data); hex.EncodeToString(sum[:]) != c.sum {
			continue
		}
		files, ok := sameAsInstalled(filepath.Dir(a.pluginsDir()), p.ID, data, installed)
		if !ok {
			// The release was found and the folder isn't it: no other name will be.
			return pluginSource{}, false
		}
		return pluginSource{Registry: safeURL(registry), Version: version, At: a.cfg.Now().UTC(), Files: files, Archive: c.sum}, true
	}
	return pluginSource{}, false
}

// sameAsInstalled unpacks a release beside the plugins folder and reports
// whether every file of it is in installed with the same SHA-256, with
// those files.
func sameAsInstalled(parent, id string, data []byte, installed *os.Root) ([]sourceFile, bool) {
	unpacked, folder, err := unpackFolder(parent, id, data)
	if unpacked != "" {
		defer os.RemoveAll(unpacked)
	}
	if err != nil {
		return nil, false
	}
	release, err := os.OpenRoot(folder)
	if err != nil {
		return nil, false
	}
	defer release.Close()
	files, err := hashFolder(release)
	if err != nil || len(files) == 0 {
		return nil, false
	}
	for _, f := range files {
		if sum, err := hashIn(installed, f.Path); err != nil || sum != f.SHA256 {
			return nil, false
		}
	}
	return files, true
}
