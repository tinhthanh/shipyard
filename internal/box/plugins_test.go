package box

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cosscom/shipyard/internal/events"
	"github.com/cosscom/shipyard/internal/hooks"
)

// A plugin as a laptop holds it: a manifest pinning an MCP server's binary
// and a skill, and those files.
type testPlugin struct {
	files map[string][]byte
	links map[string]string
}

func sum(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func newTestPlugin(version string, server []byte) testPlugin {
	skill := []byte("---\nname: acme-tools\ndescription: test\n---\nUse the acme tools.\n")
	manifest := fmt.Sprintf(`{"name":"Acme tools","version":%q,
 "mcp":{"acme":{"command":"bin/acme-mcp","args":["mcp"],"sha256":%q}},
 "skills":{"acme-tools":{"path":"skills/acme-tools/SKILL.md","sha256":%q}}}`, version, sum(server), sum(skill))
	return testPlugin{files: map[string][]byte{
		hooks.PluginManifest:         []byte(manifest),
		"bin/acme-mcp":               server,
		"skills/acme-tools/SKILL.md": skill,
	}, links: map[string]string{}}
}

// hash is what the person allowed: hooks.PluginHash of the folder.
func (p testPlugin) hash(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "acme")
	for name, b := range p.files {
		os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755)
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	h, err := hooks.PluginHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func (p testPlugin) archive(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, b := range p.files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(b)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		tw.Write(b)
	}
	for name, target := range p.links {
		tw.WriteHeader(&tar.Header{Name: name, Linkname: target, Typeflag: tar.TypeSymlink})
	}
	tw.Close()
	gz.Close()
	return &buf
}

func testBoxPlugins(t *testing.T) *BoxPlugins {
	return &BoxPlugins{Dir: filepath.Join(t.TempDir(), "plugins")}
}

func TestAPluginFromTheLaptopIsInstalledAllowedAndReplaced(t *testing.T) {
	p := testBoxPlugins(t)
	v1 := newTestPlugin("1.0.0", []byte("#!/bin/sh\necho one\n"))
	bp, err := p.install("acme", v1.hash(t), v1.archive(t))
	if err != nil {
		t.Fatal(err)
	}
	if !bp.Enabled || !bp.Tools || bp.Version != "1.0.0" {
		t.Fatalf("installed: %+v", bp)
	}
	if fi, err := os.Stat(filepath.Join(p.Dir, "acme", "bin", "acme-mcp")); err != nil || fi.Mode()&0o111 == 0 {
		t.Fatalf("the server's binary should be there and executable: %v", err)
	}
	if !hooks.PluginEnabled(filepath.Join(p.Dir, "acme")) {
		t.Fatal("the box should hold a grant for it")
	}

	v2 := newTestPlugin("2.0.0", []byte("#!/bin/sh\necho two\n"))
	if bp, err = p.install("acme", v2.hash(t), v2.archive(t)); err != nil || bp.Version != "2.0.0" || !bp.Enabled {
		t.Fatalf("replacing it: %+v, %v", bp, err)
	}
	if got, _ := os.ReadFile(filepath.Join(p.Dir, "acme", "bin", "acme-mcp")); !bytes.Contains(got, []byte("two")) {
		t.Fatal("the new version's files should have replaced the old")
	}
	if list := p.list(); len(list.Plugins) != 1 || list.Platform != HelperPlatform() {
		t.Fatalf("list: %+v (leftover staging folders are not plugins)", list)
	}
}

func TestAPluginThatIsNotTheOneAllowedIsRefused(t *testing.T) {
	p := testBoxPlugins(t)
	allowed := newTestPlugin("1.0.0", []byte("#!/bin/sh\necho one\n"))
	other := newTestPlugin("1.0.1", []byte("#!/bin/sh\necho other\n"))
	if _, err := p.install("acme", allowed.hash(t), other.archive(t)); !hasCode(err, "plugin_changed") {
		t.Fatalf("another plugin under the allowed hash: %v", err)
	}
	// The manifest is what was allowed, but a binary it pins was swapped.
	swapped := newTestPlugin("1.0.0", []byte("#!/bin/sh\necho one\n"))
	swapped.files["bin/acme-mcp"] = []byte("#!/bin/sh\necho evil\n")
	if _, err := p.install("acme", swapped.hash(t), swapped.archive(t)); !hasCode(err, "plugin_invalid") || !strings.Contains(err.Error(), "not the file") {
		t.Fatalf("a binary that isn't its pin: %v", err)
	}
	if _, err := os.Stat(filepath.Join(p.Dir, "acme")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a refused plugin was left on the box")
	}
}

func TestAPluginCannotWriteOutsideItsFolder(t *testing.T) {
	p := testBoxPlugins(t)
	for name, mod := range map[string]func(*testPlugin){
		"a path with ..":   func(tp *testPlugin) { tp.files["../escaped"] = []byte("x") },
		"an absolute path": func(tp *testPlugin) { tp.files["/tmp/escaped"] = []byte("x") },
		"a link out":       func(tp *testPlugin) { tp.links["bin/out"] = "../../../etc" },
		"an absolute link": func(tp *testPlugin) { tp.links["bin/out"] = "/etc/passwd" },
	} {
		tp := newTestPlugin("1.0.0", []byte("#!/bin/sh\n"))
		hash := tp.hash(t)
		mod(&tp)
		if _, err := p.install("acme", hash, tp.archive(t)); !hasCode(err, "plugin_invalid") {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(p.Dir), "escaped")); err == nil {
		t.Fatal("a file was written outside the plugin")
	}
	// A link that stays inside, as an app bundle's do, is fine.
	tp := newTestPlugin("1.0.0", []byte("#!/bin/sh\n"))
	tp.links["bin/current"] = "acme-mcp"
	if _, err := p.install("acme", tp.hash(t), tp.archive(t)); err != nil {
		t.Fatalf("a link inside the plugin: %v", err)
	}
}

// elfFor is the smallest ELF header for a machine: enough to say what it
// was built for.
func elfFor(machine uint16) []byte {
	h := make([]byte, 64)
	copy(h, "\x7fELF")
	h[4], h[5], h[6] = 2, 1, 1               // 64-bit, little-endian, version 1
	binary.LittleEndian.PutUint16(h[16:], 2) // executable
	binary.LittleEndian.PutUint16(h[18:], machine)
	binary.LittleEndian.PutUint32(h[20:], 1)
	binary.LittleEndian.PutUint16(h[52:], 64) // header size
	return h
}

func TestAPluginBuiltForAnotherPlatformIsRefused(t *testing.T) {
	p := testBoxPlugins(t)
	// x86-64 Linux, or arm64 Linux on an x86-64 Linux box: never this box.
	machine := uint16(62)
	if HelperPlatform() == "linux-amd64" {
		machine = 183
	}
	tp := newTestPlugin("1.0.0", elfFor(machine))
	_, err := p.install("acme", tp.hash(t), tp.archive(t))
	if !hasCode(err, "plugin_unsupported") || !strings.Contains(err.Error(), HelperPlatform()) {
		t.Fatalf("a plugin for another platform: %v", err)
	}
}

func TestRemovingAPluginTakesItAndItsGrantOff(t *testing.T) {
	p := testBoxPlugins(t)
	tp := newTestPlugin("1.0.0", []byte("#!/bin/sh\n"))
	if _, err := p.install("acme", tp.hash(t), tp.archive(t)); err != nil {
		t.Fatal(err)
	}
	if err := p.remove("acme"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(p.Dir, "acme")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the plugin's folder is still there")
	}
	if _, ok := hooks.PluginGrantFor(filepath.Join(p.Dir, "acme")); ok {
		t.Fatal("its grant is still there")
	}
}

// What runs on the box itself (an agent, a script) reaches berthd through
// its socket: it can see the plugins but not add, remove or set one up.
func TestOnlyAPairedLaptopPutsAPluginOnABox(t *testing.T) {
	p := testBoxPlugins(t)
	bx := &Box{Name: "devbox", Locations: NewLocations(filepath.Join(t.TempDir(), "locations.json")), Events: &events.Bus{}, Plugins: p}
	local := localBox(t, bx)
	ctx := context.Background()
	tp := newTestPlugin("1.0.0", []byte("#!/bin/sh\n"))

	_, err := local.InstallBoxPlugin(ctx, "acme", tp.hash(t), tp.archive(t))
	var he *HelperError
	if !errors.As(err, &he) || he.Status != 403 || he.Code != CodeRefused {
		t.Fatalf("an install from the box itself: %v", err)
	}
	if _, err := p.install("acme", tp.hash(t), tp.archive(t)); err != nil {
		t.Fatal(err)
	}
	list, err := local.BoxPlugins(ctx)
	if err != nil || len(list.Plugins) != 1 || list.Plugins[0].ID != "acme" {
		t.Fatalf("the box's own list: %+v, %v", list, err)
	}
	if err := local.RemoveBoxPlugin(ctx, "acme"); !errors.As(err, &he) || he.Status != 403 {
		t.Fatalf("a removal from the box itself: %v", err)
	}
	if _, err := local.StartBoxPluginSetup(ctx, "acme"); !errors.As(err, &he) || he.Status != 403 {
		t.Fatalf("a setup from the box itself: %v", err)
	}
}

func hasCode(err error, code string) bool {
	var rc reviewCode
	return errors.As(err, &rc) && rc.code == code
}

// An archive a Mac's tar made carries "._name" AppleDouble files beside the
// real ones (extended attributes, such as com.apple.provenance): they are
// left out, so the plugin's folder holds only its files and an app bundle in
// it keeps a valid signature.
func TestAMacsMetadataInAnArchiveIsLeftOut(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, name := range []string{"._acme", "acme/._" + hooks.PluginManifest, "acme/" + hooks.PluginManifest, "acme/App.app/Contents/._Info.plist", "acme/App.app/Contents/Info.plist", "__MACOSX/acme/x"} {
		tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: 2, Typeflag: tar.TypeReg})
		tw.Write([]byte("{}"))
	}
	tw.Close()
	gz.Close()
	dir := t.TempDir()
	if err := UnpackPlugin(dir, &buf); err != nil {
		t.Fatal(err)
	}
	var got []string
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if !d.IsDir() {
			rel, _ := filepath.Rel(dir, p)
			got = append(got, filepath.ToSlash(rel))
		}
		return nil
	})
	if strings.Join(got, " ") != "acme/App.app/Contents/Info.plist acme/"+hooks.PluginManifest {
		t.Fatalf("unpacked %v", got)
	}
}
