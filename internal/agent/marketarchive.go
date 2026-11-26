package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"

	"github.com/cosscom/shipyard/internal/box"
	"github.com/cosscom/shipyard/internal/hooks"
)

// The fork's plugin listing: an index.json attached to the "registry"
// release of github.com/tinhthanh/vetgo-releases, where the fork's plugins
// (vglink, mobile, …) are released as <id>-v<version> with a
// <id>-<version>.tar.gz of their folder. scripts/plugin-registry.py writes
// that index.json from the releases. Upstream's listing (berthd.app) is not
// browsed: the fork ships none of its plugins.
//
// A listed plugin may give its release archive ("archive") instead of its
// files one by one, or one archive per platform ("archives"), of which this
// computer's is taken (platformArchive). The archive is held to the same rules as a listed file
// (on the registry's own address, exactly the size and SHA-256 the listing
// gives) and unpacked as Install from URL unpacks one (box.UnpackPlugin:
// nothing outside the folder, programs stay executable).
const forkPluginRegistry = "https://github.com/tinhthanh/vetgo-releases/releases/download/registry/index.json"

// MarketArchive is a listed plugin's release archive: a .tar.gz with one top
// folder named for the plugin's id. URL is relative to the listing.
type MarketArchive struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// maxMarketArchive is the largest archive taken, as for Install from URL.
const maxMarketArchive = maxURLPlugin

// platformArchive picks, for a plugin built for each platform ("archives",
// by "<os>-<arch>", a Mac's also as "darwin-universal"), the archive for
// this computer and lists it as the plugin's archive. It reports false for
// a plugin with builds, none of them for this computer: it isn't listed
// here at all, rather than listed as one that can't be installed.
func platformArchive(p *listedPlugin) bool {
	if len(p.Archives) == 0 || p.Archive != nil {
		return true
	}
	for _, key := range platformKeys(runtime.GOOS, runtime.GOARCH) {
		if a, ok := p.Archives[key]; ok {
			p.Archive = &a
			return true
		}
	}
	return false
}

// platformKeys are the names a build for os/arch may be listed under, best
// first.
func platformKeys(goos, goarch string) []string {
	keys := []string{goos + "-" + goarch}
	if goos == "darwin" {
		keys = append(keys, "darwin-universal")
	}
	return keys
}

// checkArchive is checkListed for a plugin listed by its archive.
func checkArchive(registry *url.URL, p listedPlugin) (int64, string) {
	a := p.Archive
	if len(p.Files) > 0 {
		return 0, "the listing gives it both files and an archive"
	}
	if !sha256Pattern.MatchString(a.SHA256) {
		return 0, "its archive has no SHA-256 checksum"
	}
	if a.Size <= 0 || a.Size > maxMarketArchive {
		return 0, fmt.Sprintf("its archive is empty or larger than %d MB", maxMarketArchive>>20)
	}
	if _, err := fileURL(registry, a.URL); err != nil {
		return 0, "its archive " + err.Error()
	}
	return a.Size, ""
}

// listedSame reports whether what a listing installed is what it lists now.
func listedSame(src pluginSource, p MarketPlugin) bool {
	if p.Archive != nil {
		return src.Archive == p.Archive.SHA256
	}
	return src.Archive == "" && sameFiles(src.Files, p.Files)
}

// archiveSum is the SHA-256 of the archive a plugin is listed by, or "".
func (p listedPlugin) archiveSum() string {
	if p.Archive == nil {
		return ""
	}
	return p.Archive.SHA256
}

// archiveFiles is installPlugin's seam: for a plugin listed by its archive,
// the files unpackArchive put in root; for one listed by its files, files
// as they are (installPlugin downloads those itself).
func (a *Agent) archiveFiles(ctx context.Context, registry *url.URL, p MarketPlugin, root *os.Root, files []sourceFile) ([]sourceFile, error) {
	if p.Archive == nil {
		return files, nil
	}
	return a.unpackArchive(ctx, registry, p, root)
}

// unpackArchive downloads a plugin's listed archive, checks it, and puts the
// folder it holds into root (the install's stage, empty). It returns the
// files it put there, hashed as installedPlugin hashes them, so a folder
// changed since is told from one the listing installed.
func (a *Agent) unpackArchive(ctx context.Context, registry *url.URL, p MarketPlugin, root *os.Root) ([]sourceFile, error) {
	u, err := fileURL(registry, p.Archive.URL)
	if err != nil {
		return nil, marketErr(http.StatusBadRequest, "plugin_invalid", "%s can't be installed: its archive %v", p.ID, err)
	}
	data, err := marketGet(ctx, registry, u, p.Archive.Size)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) != p.Archive.Size {
		return nil, marketErr(http.StatusBadGateway, "plugin_mismatch", "the archive of %s is not the %d bytes the listing gives; nothing was installed", p.ID, p.Archive.Size)
	}
	if sum := sha256.Sum256(data); hex.EncodeToString(sum[:]) != p.Archive.SHA256 {
		return nil, marketErr(http.StatusBadGateway, "plugin_mismatch", "the archive of %s does not match the listing's checksum; nothing was installed", p.ID)
	}
	stage := root.Name()
	unpacked, err := os.MkdirTemp(filepath.Dir(stage), ".plugin-"+p.ID+"-x-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(unpacked)
	if err := box.UnpackPlugin(unpacked, bytes.NewReader(data)); err != nil {
		return nil, marketErr(http.StatusBadGateway, "plugin_invalid", "%s can't be installed: %v", p.ID, err)
	}
	folder := filepath.Join(unpacked, p.ID)
	entries, err := os.ReadDir(unpacked)
	if err != nil || len(entries) != 1 || entries[0].Name() != p.ID || !entries[0].IsDir() {
		return nil, marketErr(http.StatusBadGateway, "plugin_invalid", "%s can't be installed: its archive holds more than its %s folder", p.ID, p.ID)
	}
	inner, err := os.ReadDir(folder)
	if err != nil {
		return nil, err
	}
	for _, e := range inner {
		if err := os.Rename(filepath.Join(folder, e.Name()), filepath.Join(stage, e.Name())); err != nil {
			return nil, err
		}
	}
	var files []sourceFile
	err = fs.WalkDir(root.FS(), ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		sum, err := hashIn(root, name)
		if err != nil {
			return err
		}
		files = append(files, sourceFile{Path: name, SHA256: sum, Size: info.Size()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	if _, err := root.Stat(hooks.PluginManifest); err != nil {
		return nil, marketErr(http.StatusBadGateway, "plugin_invalid", "%s can't be installed: its archive has no %s", p.ID, hooks.PluginManifest)
	}
	return files, nil
}
