package agent

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

const testSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><circle cx="8" cy="8" r="7"/></svg>`

func TestAnInstalledPluginsIconIsADataURL(t *testing.T) {
	m := newMarketAgent(t, "")
	dir := m.pluginDir("phone")
	write(t, filepath.Join(dir, "berth-plugin.json"), `{"name":"Phone","version":"1","icon":"icon.svg"}`)
	write(t, filepath.Join(dir, "icon.svg"), testSVG)
	write(t, filepath.Join(m.pluginDir("escape"), "berth-plugin.json"), `{"name":"Escape","version":"1","icon":"../phone/icon.svg"}`)
	write(t, filepath.Join(m.pluginDir("script"), "berth-plugin.json"), `{"name":"Script","version":"1","icon":"icon.js"}`)
	write(t, filepath.Join(m.pluginDir("script"), "icon.js"), "alert(1)")

	status, body := m.call(t, "GET", "/v1/plugins", "")
	var got []PluginInfo
	if status != 200 || json.Unmarshal([]byte(body), &got) != nil {
		t.Fatalf("plugins = %d %s", status, body)
	}
	icons := map[string]string{}
	for _, p := range got {
		icons[p.ID] = p.IconURL
	}
	if !strings.HasPrefix(icons["phone"], "data:image/svg+xml;base64,") {
		t.Fatalf("phone's icon = %q", icons["phone"])
	}
	// A path out of its folder, or a file that isn't an image, is no icon.
	if icons["escape"] != "" || icons["script"] != "" {
		t.Fatalf("icons = %v", icons)
	}
}

func TestAListedIconIsCheckedAndInlined(t *testing.T) {
	r := newRegistry(t)
	r.serve("/registry/phone-icon.svg", []byte(testSVG))
	good := r.archived(t, "phone", "0.1.0", phone("0.1.0"))
	good["icon"] = map[string]any{"url": "phone-icon.svg", "sha256": sum(testSVG), "size": len(testSVG)}
	// The listing can't give the app an address of its own to load.
	good["icon_url"] = "https://tracker.example/pixel.png"
	bad := r.archived(t, "camera", "0.1.0", map[string]string{"camera/berth-plugin.json": manifestOf("Camera", "0.1.0")})
	r.serve("/registry/camera-icon.svg", []byte(testSVG))
	bad["icon"] = map[string]any{"url": "camera-icon.svg", "sha256": strings.Repeat("0", 64), "size": len(testSVG)}
	r.publishRegistry(good, bad)
	m := newMarketAgent(t, r.registryURL())

	if e := m.entry(t, "phone"); !strings.HasPrefix(e.IconURL, "data:image/svg+xml;base64,") {
		t.Fatalf("phone's icon = %q", e.IconURL)
	}
	if e := m.entry(t, "camera"); e.IconURL != "" {
		t.Fatalf("camera's icon = %q", e.IconURL)
	}
	// Read again: the icon comes from what was fetched, not the network.
	before := r.asked("/registry/phone-icon.svg")
	m.entry(t, "phone")
	if r.asked("/registry/phone-icon.svg") != before {
		t.Fatal("the icon was downloaded again")
	}
}
