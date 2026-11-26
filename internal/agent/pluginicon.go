package agent

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
)

// A plugin's icon, shown in Settings → Plugins. Its manifest names a file in
// its folder ("icon": "icon.svg"); a listing names one beside index.json
// ("icon": {"url", "sha256", "size"}), as scripts/plugin-registry.py writes
// it. The app may only talk to this agent and shows no image from elsewhere,
// so the agent reads the icon and hands it over as a data: URL (icon_url),
// which the app's CSP allows. An icon is an SVG, PNG or WebP of at most
// iconMax bytes; anything else is no icon, and the app draws the name's
// first letter instead. An SVG shown in an <img> runs no script.

const iconMax = 64 << 10

var iconTypes = map[string]string{
	".svg":  "image/svg+xml",
	".png":  "image/png",
	".webp": "image/webp",
}

func iconDataURL(name string, data []byte) string {
	mime := iconTypes[strings.ToLower(path.Ext(name))]
	if mime == "" || len(data) == 0 || len(data) > iconMax {
		return ""
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// pluginIconURL is the icon an installed plugin's manifest names, read from
// inside its folder only, or "".
func pluginIconURL(dir, icon string) string {
	icon = strings.TrimSpace(icon)
	if icon == "" || !localPath(icon) {
		return ""
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return ""
	}
	defer root.Close()
	f, err := root.Open(icon)
	if err != nil {
		return ""
	}
	defer f.Close()
	if fi, err := f.Stat(); err != nil || !fi.Mode().IsRegular() || fi.Size() > iconMax {
		return ""
	}
	data, err := io.ReadAll(io.LimitReader(f, iconMax+1))
	if err != nil {
		return ""
	}
	return iconDataURL(icon, data)
}

// listedIcons are the listing's icons already fetched, by SHA-256: a
// listing read again (every five minutes at most) downloads none of them.
var listedIcons sync.Map

// listedIconURL is the icon a listing gives a plugin, downloaded from the
// listing's own address and held to its size and SHA-256, or "".
func (a *Agent) listedIconURL(ctx context.Context, registry *url.URL, icon *MarketArchive) string {
	if icon == nil || !sha256Pattern.MatchString(icon.SHA256) || icon.Size <= 0 || icon.Size > iconMax {
		return ""
	}
	if v, ok := listedIcons.Load(icon.SHA256); ok {
		return v.(string)
	}
	u, err := fileURL(registry, icon.URL)
	if err != nil || iconTypes[strings.ToLower(path.Ext(u.Path))] == "" {
		return ""
	}
	data, err := marketGet(ctx, registry, u, icon.Size)
	if err != nil || int64(len(data)) != icon.Size {
		return ""
	}
	if sum := sha256.Sum256(data); hex.EncodeToString(sum[:]) != icon.SHA256 {
		return ""
	}
	out := iconDataURL(u.Path, data)
	if out != "" {
		listedIcons.Store(icon.SHA256, out)
	}
	return out
}
