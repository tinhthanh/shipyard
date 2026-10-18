package agent

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cosscom/shipyard/internal/hooks"
)

// releaseArchive is a plugin's release: its folder ("acme/…") as a tar.gz.
func releaseArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte(body))
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func hexSum(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

// releaseServer serves archives at /<name> and, unless bare, their
// checksums beside them as /<name>.sha256.
func releaseServer(t *testing.T, archives map[string][]byte, bare bool) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if strings.HasSuffix(name, ".sha256") && !bare {
			if b, ok := archives[strings.TrimSuffix(name, ".sha256")]; ok {
				io.WriteString(w, hexSum(b)+"  "+name+"\n")
				return
			}
		}
		if b, ok := archives[name]; ok {
			w.Write(b)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func (m *marketAgent) installURL(t *testing.T, body map[string]any) (int, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(body)
	resp, err := http.Post(m.srv.URL+"/v1/plugins/install-url", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func newURLAgent(t *testing.T) *marketAgent {
	t.Helper()
	m := newMarketAgent(t, "")
	mux := http.NewServeMux()
	m.marketplaceRoutes(mux)
	m.urlInstallRoutes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	m.srv = srv
	return m
}

func TestAPluginInstallsFromARelease(t *testing.T) {
	v1 := releaseArchive(t, map[string]string{"acme/" + hooks.PluginManifest: `{"name":"Acme","version":"1.0.0"}`, "acme/bin/tool": "one"})
	v2 := releaseArchive(t, map[string]string{"acme/" + hooks.PluginManifest: `{"name":"Acme","version":"2.0.0"}`, "acme/bin/tool": "two"})
	base := releaseServer(t, map[string][]byte{"acme-1.tar.gz": v1, "acme-2.tar.gz": v2}, false)
	m := newURLAgent(t)
	dir := filepath.Join(m.user, "plugins", "acme")

	status, out := m.installURL(t, map[string]any{"url": base + "/acme-1.tar.gz"})
	if status != 200 || out["id"] != "acme" || out["version"] != "1.0.0" {
		t.Fatalf("install: %d %v", status, out)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "bin", "tool")); string(b) != "one" {
		t.Fatalf("the plugin's files: %q", b)
	}
	if hooks.PluginEnabled(dir) {
		t.Fatal("installing never turns a plugin on")
	}

	// Allowed, then a new release over it: there already, so only with
	// replace, and asked about again.
	h, _ := hooks.PluginHash(dir)
	hooks.AllowPlugin(dir, h)
	if status, out := m.installURL(t, map[string]any{"url": base + "/acme-2.tar.gz"}); status != http.StatusConflict || out["code"] != "plugin_exists" {
		t.Fatalf("over an installed plugin without replace: %d %v", status, out)
	}
	if status, out := m.installURL(t, map[string]any{"url": base + "/acme-2.tar.gz", "replace": true}); status != 200 || out["replaced"] != true {
		t.Fatalf("replace: %d %v", status, out)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "bin", "tool")); string(b) != "two" {
		t.Fatalf("the new release's files: %q", b)
	}
	if hooks.PluginEnabled(dir) {
		t.Fatal("new files are asked about again")
	}
}

func TestAReleaseIsCheckedBeforeAnythingIsWritten(t *testing.T) {
	good := releaseArchive(t, map[string]string{"acme/" + hooks.PluginManifest: `{"name":"Acme","version":"1.0.0"}`})
	escape := releaseArchive(t, map[string]string{"acme/" + hooks.PluginManifest: `{}`, "acme/../../escaped": "x"})
	loose := releaseArchive(t, map[string]string{hooks.PluginManifest: `{}`})
	base := releaseServer(t, map[string][]byte{"good.tar.gz": good, "escape.tar.gz": escape, "loose.tar.gz": loose}, false)
	bare := releaseServer(t, map[string][]byte{"good.tar.gz": good}, true)
	m := newURLAgent(t)

	for _, tc := range []struct {
		body map[string]any
		code string
	}{
		{map[string]any{"url": base + "/good.tar.gz", "sha256": strings.Repeat("0", 64)}, "plugin_mismatch"},
		{map[string]any{"url": bare + "/good.tar.gz"}, "checksum_missing"},
		{map[string]any{"url": "http://example.com/good.tar.gz"}, "url_invalid"},
		{map[string]any{"url": base + "/escape.tar.gz"}, "plugin_invalid"},
		{map[string]any{"url": base + "/loose.tar.gz"}, "plugin_invalid"},
	} {
		if status, out := m.installURL(t, tc.body); status == 200 || out["code"] != tc.code {
			t.Errorf("%v: %d %v, want %s", tc.body, status, out, tc.code)
		}
	}
	if entries, _ := os.ReadDir(filepath.Join(m.user, "plugins")); len(entries) != 0 {
		t.Fatalf("a refused release left %v", entries)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(m.user), "escaped")); err == nil {
		t.Fatal("a file was written outside the plugin")
	}
	// Given by hand, the checksum is enough without one beside it.
	if status, out := m.installURL(t, map[string]any{"url": bare + "/good.tar.gz", "sha256": "sha256:" + hexSum(good)}); status != 200 {
		t.Fatalf("with its checksum given: %d %v", status, out)
	}
}
