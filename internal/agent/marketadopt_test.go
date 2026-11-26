package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// byHand puts a plugin's files where Install from URL (or someone with tar)
// would, with no record that a listing installed it.
func byHand(t *testing.T, m *marketAgent, files map[string]string) {
	t.Helper()
	for name, body := range files {
		name = strings.TrimSuffix(name, "*")
		write(t, filepath.Join(m.user, "plugins", filepath.FromSlash(name)), body)
	}
}

// releaseBeside serves a release of id at version beside the listing, with
// its .sha256, as vetgo-releases has every release, and returns the
// archive's path.
func (r *fakeRegistry) releaseBeside(t *testing.T, id, version string, files map[string]string) string {
	t.Helper()
	e := r.archived(t, id, version, files)
	a := e["archive"].(map[string]any)
	p := "/" + id + "-v" + version + "/" + id + "-" + version + ".tar.gz"
	r.serve(p+".sha256", []byte(a["sha256"].(string)+"  "+id+"-"+version+".tar.gz\n"))
	return p
}

func TestAPluginInstalledByHandFromTheListedReleaseIsTheListings(t *testing.T) {
	r := newRegistry(t)
	r.publishRegistry(r.archived(t, "phone", "0.1.0", phone("0.1.0")))
	m := newMarketAgent(t, r.registryURL())
	byHand(t, m, phone("0.1.0"))
	// A file the plugin wrote itself since doesn't make it someone's own.
	write(t, filepath.Join(m.pluginDir("phone"), "state.json"), "{}")

	if e := m.entry(t, "phone"); !e.Installed || e.ByHand || e.Update {
		t.Fatalf("listed = %+v", e)
	}
	if src, ok := m.readPluginSources()["phone"]; !ok || src.Version != "0.1.0" || src.Archive == "" {
		t.Fatalf("recorded = %+v, %v", src, ok)
	}
	if !strings.Contains(m.logs.String(), "its files are the release's") {
		t.Fatalf("logs = %s", m.logs)
	}
}

func TestAnOlderReleaseInstalledByHandIsUpdatedFromTheListing(t *testing.T) {
	r := newRegistry(t)
	r.releaseBeside(t, "phone", "0.1.0", phone("0.1.0"))
	r.publishRegistry(r.archived(t, "phone", "0.2.0", phone("0.2.0")))
	m := newMarketAgent(t, r.registryURL())
	byHand(t, m, phone("0.1.0"))

	e := m.entry(t, "phone")
	if e.ByHand || !e.Update || e.InstalledVersion != "0.1.0" {
		t.Fatalf("listed = %+v", e)
	}
	// An update, so no replace is needed.
	if status, body := m.call(t, "POST", "/v1/marketplace/phone/install", `{"version":"0.2.0"}`); status != 200 {
		t.Fatalf("update = %d %s", status, body)
	}
	if e := m.entry(t, "phone"); e.InstalledVersion != "0.2.0" || e.Update || e.ByHand {
		t.Fatalf("after update = %+v", e)
	}
}

func TestAFolderThatIsntTheReleaseStaysSomeonesOwn(t *testing.T) {
	r := newRegistry(t)
	path := r.releaseBeside(t, "phone", "0.1.0", phone("0.1.0"))
	r.publishRegistry(r.archived(t, "phone", "0.2.0", phone("0.2.0")))
	m := newMarketAgent(t, r.registryURL())
	changed := phone("0.1.0")
	changed["phone/dist/index.js"] = "mine"
	byHand(t, m, changed)

	if e := m.entry(t, "phone"); !e.ByHand {
		t.Fatalf("listed = %+v", e)
	}
	if _, ok := m.readPluginSources()["phone"]; ok {
		t.Fatal("recorded as the listing's")
	}
	// Checked once per version: reading the listing again downloads nothing.
	before := r.asked(path)
	m.entry(t, "phone")
	if r.asked(path) != before || before != 1 {
		t.Fatalf("the release was downloaded %d, then %d times", before, r.asked(path))
	}
	// Replacing it still has to be asked for.
	if status, body := m.call(t, "POST", "/v1/marketplace/phone/install", ""); status != 409 || !strings.Contains(body, "plugin_by_hand") {
		t.Fatalf("install = %d %s", status, body)
	}
}

func TestAVersionWithNoReleaseBesideStaysSomeonesOwn(t *testing.T) {
	r := newRegistry(t)
	r.publishRegistry(r.archived(t, "phone", "0.2.0", phone("0.2.0")))
	m := newMarketAgent(t, r.registryURL())
	byHand(t, m, phone("0.1.5"))

	if e := m.entry(t, "phone"); !e.ByHand || e.Update {
		t.Fatalf("listed = %+v", e)
	}
}

func TestALinkedPluginIsNeverTheListings(t *testing.T) {
	r := newRegistry(t)
	r.publishRegistry(r.archived(t, "phone", "0.1.0", phone("0.1.0")))
	m := newMarketAgent(t, r.registryURL())
	// A plugin being written, linked in from where it is.
	src := filepath.Join(t.TempDir(), "phone")
	for name, body := range phone("0.1.0") {
		write(t, filepath.Join(filepath.Dir(src), filepath.FromSlash(strings.TrimSuffix(name, "*"))), body)
	}
	if err := os.MkdirAll(filepath.Join(m.user, "plugins"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(src, m.pluginDir("phone")); err != nil {
		t.Fatal(err)
	}
	if e := m.entry(t, "phone"); !e.ByHand {
		t.Fatalf("listed = %+v", e)
	}
}
