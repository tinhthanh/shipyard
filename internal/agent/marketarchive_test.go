package agent

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// tarball is a .tar.gz of files, by path, the way a plugin's release script
// packs its folder. A path ending in "*" is made executable (without it).
func tarball(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		mode := int64(0o644)
		if strings.HasSuffix(name, "*") {
			name, mode = strings.TrimSuffix(name, "*"), 0o755
		}
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: mode, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte(body))
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

// archived serves a plugin's release archive the way vetgo-releases has it
// (<id>-v<version>/<id>-<version>.tar.gz beside registry/index.json) and
// returns its entry for the listing.
func (r *fakeRegistry) archived(t *testing.T, id, version string, files map[string]string) map[string]any {
	t.Helper()
	data := tarball(t, files)
	rel := "../" + id + "-v" + version + "/" + id + "-" + version + ".tar.gz"
	r.serve("/"+id+"-v"+version+"/"+id+"-"+version+".tar.gz", data)
	return map[string]any{"id": id, "name": "Acme " + id, "version": version,
		"archive": map[string]any{"url": rel, "sha256": sum(string(data)), "size": len(data)}}
}

func phone(version string) map[string]string {
	return map[string]string{
		"phone/berth-plugin.json": manifestOf("Acme phone", version),
		"phone/dist/index.js":     "export default function activate() { /* " + version + " */ }\n",
		"phone/bin/helper*":       "#!/bin/sh\necho " + version + "\n",
	}
}

func (r *fakeRegistry) registryURL() string { return r.srv.URL + "/registry/index.json" }

func (r *fakeRegistry) publishRegistry(plugins ...map[string]any) {
	b, _ := json.Marshal(map[string]any{"plugins": plugins})
	r.serve("/registry/index.json", b)
}

func TestTheForksListingIsVetgoReleases(t *testing.T) {
	if !strings.HasPrefix(DefaultPluginRegistry, "https://github.com/tinhthanh/vetgo-releases/releases/download/") {
		t.Fatalf("DefaultPluginRegistry = %s", DefaultPluginRegistry)
	}
}

func TestAPluginListedByItsArchiveInstallsAndUpdates(t *testing.T) {
	r := newRegistry(t)
	r.publishRegistry(r.archived(t, "phone", "0.1.0", phone("0.1.0")))
	m := newMarketAgent(t, r.registryURL())

	e := m.entry(t, "phone")
	if e.Error != "" || e.Installed || e.Size == 0 {
		t.Fatalf("listed = %+v", e)
	}
	if status, body := m.call(t, "POST", "/v1/marketplace/phone/install", `{"version":"0.1.0"}`); status != 200 {
		t.Fatalf("install = %d %s", status, body)
	}
	dir := m.pluginDir("phone")
	if b, _ := os.ReadFile(filepath.Join(dir, "dist", "index.js")); !strings.Contains(string(b), "0.1.0") {
		t.Fatalf("index.js = %q", b)
	}
	// A program in the archive stays a program.
	if fi, err := os.Stat(filepath.Join(dir, "bin", "helper")); err != nil || fi.Mode()&0o111 == 0 {
		t.Fatalf("bin/helper = %v, %v", fi, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "phone")); err == nil {
		t.Fatal("the archive's top folder was kept inside the plugin's")
	}
	if e := m.entry(t, "phone"); !e.Installed || e.InstalledVersion != "0.1.0" || e.Update || e.ByHand {
		t.Fatalf("after install = %+v", e)
	}
	if left := m.leftovers(t); len(left) != 0 {
		t.Fatalf("left beside the plugins folder: %v", left)
	}

	// A new release is an update, installed over it without asking.
	r.publishRegistry(r.archived(t, "phone", "0.2.0", phone("0.2.0")))
	if e := m.entry(t, "phone"); !e.Update || e.ByHand {
		t.Fatalf("with 0.2.0 listed = %+v", e)
	}
	if status, body := m.call(t, "POST", "/v1/marketplace/phone/install", ""); status != 200 {
		t.Fatalf("update = %d %s", status, body)
	}
	if e := m.entry(t, "phone"); e.InstalledVersion != "0.2.0" || e.Update {
		t.Fatalf("after update = %+v", e)
	}

	// A file changed by hand makes the folder someone's own again.
	write(t, filepath.Join(dir, "dist", "index.js"), "mine")
	if e := m.entry(t, "phone"); !e.ByHand {
		t.Fatalf("after a change by hand = %+v", e)
	}
}

func TestAPluginBuiltPerPlatformInstallsThisComputersBuild(t *testing.T) {
	r := newRegistry(t)
	here := platformKeys(runtime.GOOS, runtime.GOARCH)[0]
	// This computer's build, and another's whose files say "elsewhere".
	mine := r.archived(t, "phone", "0.1.0", phone("0.1.0"))
	elsewhere := phone("0.1.0")
	elsewhere["phone/dist/index.js"] = "elsewhere"
	other := r.archived(t, "phone-other", "0.1.0", elsewhere)
	perPlatform := map[string]any{"id": "phone", "name": "Acme phone", "version": "0.1.0", "archives": map[string]any{
		here: mine["archive"], "plan9-mips": other["archive"],
	}}
	// A plugin built only for another computer isn't listed here.
	onlyElsewhere := map[string]any{"id": "camera", "version": "0.1.0", "archives": map[string]any{"plan9-mips": other["archive"]}}
	r.publishRegistry(perPlatform, onlyElsewhere)
	m := newMarketAgent(t, r.registryURL())

	for _, p := range m.listed(t, "?fresh=1").Plugins {
		if p.ID == "camera" {
			t.Fatalf("camera is listed: %+v", p)
		}
	}
	if status, body := m.call(t, "POST", "/v1/marketplace/phone/install", ""); status != 200 {
		t.Fatalf("install = %d %s", status, body)
	}
	if b, _ := os.ReadFile(filepath.Join(m.pluginDir("phone"), "dist", "index.js")); string(b) == "elsewhere" {
		t.Fatal("another computer's build was installed")
	}
	if e := m.entry(t, "phone"); !e.Installed || e.Update {
		t.Fatalf("after install = %+v", e)
	}
}

func TestAMacTakesAUniversalBuild(t *testing.T) {
	p := listedPlugin{Archives: map[string]MarketArchive{"darwin-universal": {URL: "u"}, "linux-arm64": {URL: "l"}}}
	if keys := platformKeys("darwin", "arm64"); !slices.Contains(keys, "darwin-universal") || keys[0] != "darwin-arm64" {
		t.Fatalf("keys = %v", keys)
	}
	if keys := platformKeys("linux", "arm64"); slices.Contains(keys, "darwin-universal") {
		t.Fatalf("keys = %v", keys)
	}
	if runtime.GOOS == "darwin" && (!platformArchive(&p) || p.Archive.URL != "u") {
		t.Fatalf("archive = %+v", p.Archive)
	}
}

func TestAnArchiveIsCheckedBeforeAnythingIsInstalled(t *testing.T) {
	set := func(e map[string]any, key string, v any) map[string]any {
		e["archive"].(map[string]any)[key] = v
		return e
	}
	for name, tc := range map[string]struct {
		entry  func(r *fakeRegistry) map[string]any
		listed string // the listing's error, or "" when it lists fine
		code   string // the install's
	}{
		"a checksum that isn't the archive's": {func(r *fakeRegistry) map[string]any {
			return set(r.archived(t, "phone", "0.1.0", phone("0.1.0")), "sha256", strings.Repeat("0", 64))
		}, "", "plugin_mismatch"},
		"a folder named for another plugin": {func(r *fakeRegistry) map[string]any {
			return r.archived(t, "phone", "0.1.0", map[string]string{"camera/berth-plugin.json": manifestOf("Camera", "0.1.0")})
		}, "", "plugin_invalid"},
		"an archive on another host": {func(r *fakeRegistry) map[string]any {
			return set(r.archived(t, "phone", "0.1.0", phone("0.1.0")), "url", "https://example.com/phone-0.1.0.tar.gz")
		}, "not on the registry's own address", ""},
		"both files and an archive": {func(r *fakeRegistry) map[string]any {
			e := r.archived(t, "phone", "0.1.0", phone("0.1.0"))
			e["files"] = []any{map[string]any{"path": "berth-plugin.json", "url": "x", "sha256": sum("x"), "size": 1}}
			return e
		}, "both files and an archive", ""},
		"a manifest of another version": {func(r *fakeRegistry) map[string]any {
			return r.archived(t, "phone", "0.1.0", phone("0.3.0"))
		}, "", "plugin_invalid"},
	} {
		t.Run(name, func(t *testing.T) {
			r := newRegistry(t)
			r.publishRegistry(tc.entry(r))
			m := newMarketAgent(t, r.registryURL())
			e := m.entry(t, "phone")
			if tc.listed != "" {
				if !strings.Contains(e.Error, tc.listed) {
					t.Fatalf("listed error = %q, want %q", e.Error, tc.listed)
				}
				return
			}
			status, body := m.call(t, "POST", "/v1/marketplace/phone/install", "")
			if status == 200 || !strings.Contains(body, tc.code) {
				t.Fatalf("install = %d %s, want %s", status, body, tc.code)
			}
			if _, err := os.Stat(m.pluginDir("phone")); err == nil {
				t.Fatal("a plugin was installed")
			}
			if left := m.leftovers(t); len(left) != 0 {
				t.Fatalf("left beside the plugins folder: %v", left)
			}
		})
	}
}
