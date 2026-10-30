package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cosscom/shipyard/internal/identity"
)

// This computer as a box on Windows itself, without WSL (Labs). berthd for
// Windows runs agents in PowerShell, under psmux, a tmux for Windows. The
// app carries both beside berth.exe (scripts/windows-app-build.ps1): berthd.exe,
// berthd-bg.exe (the same program without a console, which Windows starts
// at sign-in) and tmux.exe (psmux). Setting up copies them to
// BERTH_HOME\bin, as a Mac's box copies its berthd out of the app, runs
// `berthd install` listening on 127.0.0.1, and pairs with it.

// nativeFiles are what the native box runs, beside berth.exe in the app;
// berthd.exe first.
var nativeFiles = []string{"berthd.exe", "berthd-bg.exe", "tmux.exe", "psmux-LICENSE.txt"}

var errNoNative = errors.New("this copy of the app carries no berthd for Windows itself; set this computer up in WSL instead")

// nativeBundle says whether the app carries the native box's files.
func (a *Agent) nativeBundle() error {
	exe, err := a.cliPath()
	if err != nil {
		return errNoNative
	}
	for _, name := range nativeFiles[:3] {
		if !isFile(filepath.Join(filepath.Dir(exe), name)) {
			return errNoNative
		}
	}
	return nil
}

// copyReplacing puts src at dst unless the same file is there, and says
// whether it did. Windows won't overwrite a program while it runs, but will
// rename it: a running copy is moved aside first (and the one moved aside
// last time, no longer running, deleted).
func copyReplacing(src, dst string) (bool, error) {
	if isFile(dst) {
		want, err := fileSHA256(src)
		if err != nil {
			return false, err
		}
		if have, err := fileSHA256(dst); err == nil && have == want {
			return false, nil
		}
		// Copies moved aside before go when nothing runs them any more. One
		// still running (an agent's host outlives berthd's updates) stays,
		// and this one is moved aside under a name of its own.
		removeAside(dst)
		old := dst + ".old"
		if _, err := os.Lstat(old); err == nil {
			old = fmt.Sprintf("%s.old-%d", dst, time.Now().UnixNano())
		}
		if err := os.Rename(dst, old); err != nil {
			return false, err
		}
	}
	return true, copyExecutable(src, dst)
}

// removeAside deletes the copies of dst moved aside earlier (dst.old,
// dst.old-…) that can go: a program still running from one can't be.
func removeAside(dst string) {
	matches, _ := filepath.Glob(dst + ".old*")
	for _, m := range matches {
		os.Remove(m)
	}
}

// setUpNativeBox is setUpLocalBox for berthd on Windows itself.
func (a *Agent) setUpNativeBox(ctx context.Context, say sayFunc) (string, error) {
	if err := a.nativeBundle(); err != nil {
		return "", err
	}
	exe, _ := a.cliPath()
	home := a.localHome()
	bin := filepath.Join(home, "bin")
	copied := false
	for _, name := range nativeFiles {
		src := filepath.Join(filepath.Dir(exe), name)
		if !isFile(src) {
			continue
		}
		did, err := copyReplacing(src, filepath.Join(bin, name))
		if err != nil {
			return "", fmt.Errorf("copying %s to %s: %w", name, bin, err)
		}
		if did {
			say("Copied %s to %s.", name, bin)
		}
		copied = copied || did
	}
	prog := filepath.Join(bin, "berthd.exe")

	listen := ""
	if rec := a.localRecord(); rec != nil && rec.Native && loopback(rec.Listen) {
		listen = rec.Listen
	}
	running := listen != "" && wslServing(listen)
	if !running && listen != "" && !portFree(listen) {
		say("Something else is using %s now; berthd moves to another port.", listen)
		listen = ""
	}
	var err error
	if listen == "" {
		if listen, err = freeLoopbackPort(a.localPort()); err != nil {
			return "", err
		}
	}
	if copied || !running {
		say("Installing berthd on Windows, listening on %s: this computer only. Agents run in PowerShell…", listen)
		if err := runBerthd(ctx, say, prog, home, "install", "--listen", listen, "--no-tools"); err != nil {
			return "", err
		}
		if !waitServing(ctx, listen, 20*time.Second) {
			return "", fmt.Errorf("berthd does not answer on %s", listen)
		}
	} else {
		say("berthd is installed and running.")
	}
	run := func(ctx context.Context, args ...string) (string, error) {
		return berthdOutput(ctx, prog, home, args...)
	}
	rec := &localBoxRecord{Program: prog, Home: home, Owned: true, Native: true, Listen: listen}
	name, err := a.pairWith(ctx, run, listen, rec, say)
	if err != nil {
		return "", err
	}
	a.sync()
	a.checkAll(ctx)
	return name, nil
}

// removeNativeBox is removeLocalBox for berthd on Windows itself: the box is
// forgotten, its service removed and berthd stopped; with removeData, its
// state (BERTH_HOME\box) goes too. Repositories are never touched.
func (a *Agent) removeNativeBox(ctx context.Context, rec *localBoxRecord, removeData bool, say sayFunc) error {
	if fp, err := identity.ParseFingerprint(rec.Fingerprint); err == nil {
		if peer, ok, _ := a.boxes.Trusted(fp); ok {
			say("Forgetting %s on this laptop…", peer.Name)
			if _, err := a.cli(ctx, "forget", peer.Name).Output(); err != nil {
				return errors.New(cliError(err))
			}
			a.sync()
		}
	}
	if isFile(rec.Program) {
		say("Stopping berthd and removing its service…")
		if err := runBerthd(ctx, say, rec.Program, rec.Home, "uninstall"); err != nil {
			return err
		}
	}
	bin := filepath.Dir(rec.Program)
	for _, name := range nativeFiles {
		os.Remove(filepath.Join(bin, name))
		removeAside(filepath.Join(bin, name))
	}
	dir := filepath.Join(rec.Home, "box")
	if removeData {
		say("Deleting %s…", dir)
		if err := os.RemoveAll(dir); err != nil && !strings.Contains(err.Error(), "cannot find") {
			return err
		}
	} else if _, err := os.Stat(dir); err == nil {
		say("Kept its data in %s; setting this computer up again uses it.", dir)
	}
	return a.saveLocalRecord(nil)
}

// refreshNativeBox is refreshLocalBox for berthd on Windows itself: after an
// app update its files are copied again and its service reinstalled, which
// replaces the berthd running (sessions keep running, in psmux).
func (a *Agent) refreshNativeBox(ctx context.Context, rec *localBoxRecord) {
	exe, err := a.cliPath()
	if err != nil || a.nativeBundle() != nil {
		return
	}
	src := filepath.Join(filepath.Dir(exe), "berthd.exe")
	st, err := os.Stat(src)
	if err != nil {
		return
	}
	stamp := fmt.Sprint(st.Size(), st.ModTime().UnixNano())
	done, err := a.work.begin("updating this computer's box")
	if err != nil {
		return
	}
	defer done()
	if !a.local.mu.TryLock() {
		return
	}
	defer a.local.mu.Unlock()
	if a.local.seen == stamp {
		return
	}
	say := func(format string, args ...any) { a.cfg.Log.Printf("this computer's box: "+format, args...) }
	copied := false
	for _, name := range nativeFiles {
		s := filepath.Join(filepath.Dir(exe), name)
		if !isFile(s) {
			continue
		}
		did, err := copyReplacing(s, filepath.Join(filepath.Dir(rec.Program), name))
		if err != nil {
			say("copying %s: %v", name, err)
			return
		}
		copied = copied || did
	}
	if copied {
		if err := runBerthd(ctx, say, rec.Program, rec.Home, "install", "--keep-listen", "--no-tools"); err != nil {
			say("reinstalling berthd: %v", err)
			return
		}
		a.checkSoon()
	}
	a.local.seen = stamp
}
