package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cosscom/shipyard/internal/box"
	"github.com/cosscom/shipyard/internal/hooks"
)

// A plugin can also come from a release: a .tar.gz of its folder (as a
// plugin's own packaging script makes it, "chrome/…") at an https address,
// with its SHA-256 given or published beside it as <url>.sha256. It goes
// where the marketplace puts plugins, the same way: unpacked beside the
// plugins folder (nothing outside the plugin's folder is written, no link
// leads out of it), then swapped in at once. Installing never turns it on:
// a new plugin is off until the person allows it, and one installed over
// another is asked about again unless its files are the same.

// URLInstalled is what installing a plugin from an address did.
type URLInstalled struct {
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`
	Version string `json:"version,omitempty"`
	SHA256  string `json:"sha256"`
	// Replaced: a plugin of that id was there and was replaced.
	Replaced bool `json:"replaced,omitempty"`
}

// maxURLPlugin is the largest release archive taken (a universal app or
// binary is tens of MB).
const maxURLPlugin = 256 << 20

func (a *Agent) installPluginFromURL(ctx context.Context, raw, want string, replace bool) (URLInstalled, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || !(u.Scheme == "https" || (u.Scheme == "http" && loopbackHost(u.Hostname()))) {
		return URLInstalled{}, marketErr(http.StatusBadRequest, "url_invalid", "a plugin's address must be https://")
	}
	want = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(want), "sha256:")))
	if want == "" {
		// Published beside the archive, as "<hex>  <file name>".
		side, err := urlGet(ctx, u.String()+".sha256", 4<<10)
		if err != nil {
			return URLInstalled{}, marketErr(http.StatusBadRequest, "checksum_missing", "give the archive's SHA-256: %s has none beside it (%v)", safeURL(u), err)
		}
		if f := strings.Fields(string(side)); len(f) > 0 {
			want = strings.ToLower(f[0])
		}
	}
	if len(want) != 64 || strings.Trim(want, "0123456789abcdef") != "" {
		return URLInstalled{}, marketErr(http.StatusBadRequest, "checksum_invalid", "a SHA-256 is 64 hex digits")
	}
	data, err := urlGet(ctx, u.String(), maxURLPlugin)
	if err != nil {
		return URLInstalled{}, marketErr(http.StatusBadGateway, "download_failed", "downloading %s: %v", safeURL(u), err)
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != want {
		return URLInstalled{}, marketErr(http.StatusBadGateway, "plugin_mismatch", "the archive's SHA-256 is %s, not %s; nothing was installed", got, want)
	}

	a.market.change.Lock()
	defer a.market.change.Unlock()
	if err := os.MkdirAll(a.pluginsDir(), 0o700); err != nil {
		return URLInstalled{}, err
	}
	unpacked, err := os.MkdirTemp(filepath.Dir(a.pluginsDir()), ".plugin-url-")
	if err != nil {
		return URLInstalled{}, err
	}
	defer os.RemoveAll(unpacked)
	if err := box.UnpackPlugin(unpacked, bytes.NewReader(data)); err != nil {
		return URLInstalled{}, marketErr(http.StatusBadRequest, "plugin_invalid", "the archive can't be installed: %v", err)
	}
	// Its folder: the archive's one top folder, or the archive itself.
	folder := unpacked
	if entries, err := os.ReadDir(unpacked); err == nil && len(entries) == 1 && entries[0].IsDir() {
		folder = filepath.Join(unpacked, entries[0].Name())
	}
	id := filepath.Base(folder)
	if folder == unpacked || !pluginIDPattern.MatchString(id) {
		return URLInstalled{}, marketErr(http.StatusBadRequest, "plugin_invalid", "the archive holds no plugin folder (a top folder named like \"chrome\", with %s in it)", hooks.PluginManifest)
	}
	var manifest struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	b, err := os.ReadFile(filepath.Join(folder, hooks.PluginManifest))
	if err != nil || json.Unmarshal(b, &manifest) != nil {
		return URLInstalled{}, marketErr(http.StatusBadRequest, "plugin_invalid", "%s has no %s it can read", id, hooks.PluginManifest)
	}
	out := URLInstalled{ID: id, Name: manifest.Name, Version: manifest.Version, SHA256: want}

	stage, plugins, err := a.pluginStage(id)
	if err != nil {
		return out, err
	}
	defer os.RemoveAll(stage)
	os.Remove(stage)
	if err := os.Rename(folder, stage); err != nil {
		return out, err
	}
	target := filepath.Join(plugins, id)
	dest := filepath.Join(a.pluginsDir(), id)
	_, statErr := os.Lstat(target)
	present := statErr == nil
	if present && !replace {
		return out, marketErr(http.StatusConflict, "plugin_exists", "~/.berth/plugins/%s is there already; replace it to install this one", id)
	}
	before, _ := hooks.PluginHash(dest)
	old := stage + ".old"
	if present {
		if err := os.Rename(target, old); err != nil {
			return out, err
		}
		defer os.RemoveAll(old)
	}
	if err := os.Rename(stage, target); err != nil {
		if present {
			os.Rename(old, target)
		}
		return out, err
	}
	out.Replaced = present
	after, _ := hooks.PluginHash(dest)
	forgot := false
	if before != after && hooks.PluginEnabled(dest) {
		if err := hooks.ForbidPlugin(dest); err != nil {
			return out, err
		}
		forgot = true
	}
	if present && before != after {
		a.helperPluginReplaced(id, forgot)
	}
	a.cfg.Log.Printf("installed plugin %s %s from %s", id, manifest.Version, safeURL(u))
	return out, nil
}

// urlGet downloads an https address, following redirects only to https
// (release files redirect to a download host), up to max bytes.
func urlGet(ctx context.Context, raw string, max int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > 5 {
			return fmt.Errorf("too many redirects")
		}
		if r.URL.Scheme != "https" && !(r.URL.Scheme == "http" && loopbackHost(r.URL.Hostname())) {
			return fmt.Errorf("redirected away from https")
		}
		return nil
	}}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("larger than %d MB", max>>20)
	}
	return data, nil
}

func (a *Agent) urlInstallRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/plugins/install-url", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			URL     string `json:"url"`
			SHA256  string `json:"sha256"`
			Replace bool   `json:"replace"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		done, err := a.work.begin("install a plugin from an address")
		if err != nil {
			writeCoded(w, http.StatusServiceUnavailable, err.Error(), "agent_restarting")
			return
		}
		defer done()
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
		defer cancel()
		out, err := a.installPluginFromURL(ctx, req.URL, req.SHA256, req.Replace)
		if err != nil {
			a.cfg.Log.Printf("installing a plugin from %s: %v", req.URL, err)
			writeMarketError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
}
