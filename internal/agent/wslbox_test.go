package agent

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cosscom/shipyard/internal/trust"
)

// A fake wsl.exe for this computer's box on Windows: it logs each run and
// answers as WSL would, keeping the distribution, the copied berthd and
// whether it serves as files in FAKE_WSL_STATE. Runs of the box's berthd
// (through bash, as its user) are logged to FAKE_BERTHD_LOG and answered as
// berthd would.
const fakeWSL = `#!/bin/sh
echo "$*" >> "$FAKE_WSL_LOG"
S="$FAKE_WSL_STATE"
case "$1" in
  --status) [ -z "$FAKE_WSL_MISSING" ] || { echo "WSL is not installed"; exit 1; }; echo "Default Version: 2"; exit 0 ;;
  --list) echo "Ubuntu-24.04"; [ -f "$S/distro" ] && echo "berth-box"; exit 0 ;;
  --install) touch "$S/distro"; echo "Distribution successfully installed."; exit 0 ;;
  --unregister) rm -f "$S/distro" "$S/berthd" "$S/serving"; exit 0 ;;
  --terminate) exit 0 ;;
  -d) shift 2 ;;
esac
user=""
if [ "$1" = "-u" ]; then user="$2"; shift 2; fi
[ "$1" = "--exec" ] && shift
case "$1" in
  /bin/sleep) exec sleep 1000 ;;
  wslpath) echo "$4"; exit 0 ;;
  sha256sum)
    [ -f "$S/berthd" ] || { echo "sha256sum: $2: No such file or directory" >&2; exit 1; }
    h=$( (sha256sum "$S/berthd" 2>/dev/null || shasum -a 256 "$S/berthd") | cut -d' ' -f1)
    echo "$h  $2"; exit 0 ;;
  install) cp "$5" "$S/berthd"; exit 0 ;;
  /bin/bash)
    if [ "$user" = "root" ]; then echo "$FAKE_WSL_USER /home/$FAKE_WSL_USER"; exit 0; fi
    shift 5
    echo "$*" >> "$FAKE_BERTHD_LOG"
    case "$1" in
      id) echo "$FAKE_FP" ;;
      install) touch "$S/serving"; echo "Installed berthd.service; berthd is serving."; echo "Next: berthd pair" ;;
      pair) addr=""; while [ $# -gt 0 ]; do [ "$1" = "--address" ] && addr="$2"; shift; done
            printf '{"link":"berth://%s?code=c0de&fp=%s"}\n' "$addr" "$FAKE_FP" ;;
      uninstall) rm -f "$S/serving"; echo "Removed berthd.service" ;;
    esac
    exit 0 ;;
esac
echo "fake wsl: unexpected $*" >&2
exit 2
`

type wslEnv struct {
	*localEnv
	state, wsl string
}

// newWSLEnv is newLocalEnv as Windows: the bundled berthd is the Linux one
// beside berth, and wsl.exe is the fake above.
func newWSLEnv(t *testing.T) *wslEnv {
	t.Helper()
	e := &wslEnv{localEnv: newLocalEnv(t)}
	e.state, e.wsl = filepath.Join(e.root, "wsl"), filepath.Join(e.root, "wsl.log")
	writeFile(t, filepath.Join(e.state, ".keep"), "", 0o644)
	writeFile(t, filepath.Join(e.dir, wslBerthdName), fakeBerthd("linux-new"), 0o755)
	fake := filepath.Join(e.root, "wsl.exe")
	writeFile(t, fake, fakeWSL, 0o755)
	t.Setenv("FAKE_WSL_LOG", e.wsl)
	t.Setenv("FAKE_WSL_STATE", e.state)
	t.Setenv("FAKE_WSL_USER", wslUser())
	t.Setenv("FAKE_WSL_MISSING", "")
	oldGOOS, oldCmd, oldServing := localGOOS, wslCmd, wslServing
	localGOOS = "windows"
	wslCmd = func(ctx context.Context, args ...string) *exec.Cmd { return exec.CommandContext(ctx, fake, args...) }
	wslServing = func(string) bool { return isFile(filepath.Join(e.state, "serving")) }
	forgetWSLPresent()
	t.Cleanup(func() {
		localGOOS, wslCmd, wslServing = oldGOOS, oldCmd, oldServing
		forgetWSLPresent()
	})
	return e
}

// record is the agent's localbox.json, or nil.
func (e *wslEnv) record() *localBoxRecord {
	var r localBoxRecord
	if json.Unmarshal([]byte(e.log(filepath.Join(e.dir, "localbox.json"))), &r) != nil || r.Fingerprint == "" {
		return nil
	}
	return &r
}

func TestOnWindowsThisComputerIsABoxInWSL(t *testing.T) {
	e := newWSLEnv(t)
	a := startAgent(t, e.dir)
	tok := uiToken(t, a)

	_, body := uiSend(t, a, "GET", "/v1/boxes/local", tok, "")
	var st LocalBoxStatus
	json.Unmarshal([]byte(body), &st)
	if !st.Supported || !st.Available || st.Installed || st.NeedsWSL || st.WSL != wslDistro {
		t.Fatalf("before: %s", body)
	}

	_, body = uiSend(t, a, "POST", "/v1/boxes/local", tok, "")
	lines, last := streamed(t, body)
	if last.Error != "" || last.Box != localName() {
		t.Fatalf("set up: %s", body)
	}
	all := strings.Join(lines, "\n")
	for _, want := range []string{
		"Installing Ubuntu 24.04 in WSL as berth-box",
		"Preparing berth-box: the user " + wslUser() + ", tmux and git…",
		"Copying berthd into berth-box…",
		"Installing the berthd service in berth-box, listening on 127.0.0.1:",
		"Pairing this laptop with it…",
		"Paired as " + localName() + ".",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("set up did not say %q:\n%s", want, all)
		}
	}
	if strings.Contains(all, "Next: berthd pair") {
		t.Error("the set up passed on berthd's advice to pair by hand")
	}
	rec := e.record()
	if rec == nil || rec.WSL != wslDistro || !loopback(rec.Listen) || rec.Program != "/home/"+wslUser()+"/.local/bin/berthd" {
		t.Fatalf("record = %+v", rec)
	}
	berthd := e.log(e.berthd)
	for _, want := range []string{"install --listen " + rec.Listen + " --no-tools", "id", "pair --json --ttl 2m --address " + rec.Listen} {
		if !strings.Contains(berthd, want+"\n") {
			t.Errorf("berthd did not run %q:\n%s", want, berthd)
		}
	}
	wsl := e.log(e.wsl)
	for _, want := range []string{"--install Ubuntu-24.04 --name berth-box --no-launch", "-d berth-box -u root --exec /bin/bash -c "} {
		if !strings.Contains(wsl, want) {
			t.Errorf("wsl was not run with %q:\n%s", want, wsl)
		}
	}
	// What berth pair saved; the box is this computer's.
	if err := trust.NewStore(filepath.Join(e.dir, "boxes.json")).Add(trust.Peer{Name: localName(), Address: rec.Listen, Fingerprint: e.fp, PairedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the box marked local", func() bool {
		s, err := a.client.Status(context.Background())
		return err == nil && len(s.Boxes) == 1 && s.Boxes[0].Local
	})

	// Something keeps the distribution running, so WSL doesn't stop berthd.
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(e.log(e.wsl), "-d berth-box --exec /bin/sleep infinity") {
		if time.Now().After(deadline) {
			t.Fatalf("nothing keeps berth-box running:\n%s", e.log(e.wsl))
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Again: it reconnects, installing nothing.
	_, body = uiSend(t, a, "POST", "/v1/boxes/local", tok, "")
	lines, last = streamed(t, body)
	all = strings.Join(lines, "\n")
	if last.Error != "" || strings.Contains(all, "Installing Ubuntu") || strings.Contains(all, "Copying berthd") || !strings.Contains(all, "berthd is installed and running in berth-box.") || !strings.Contains(all, "Already paired as") {
		t.Fatalf("set up again: %s", body)
	}

	_, body = uiSend(t, a, "GET", "/v1/boxes/local", tok, "")
	st = LocalBoxStatus{}
	json.Unmarshal([]byte(body), &st)
	if !st.Installed || !st.Running || st.Listen != rec.Listen || st.Box != localName() {
		t.Fatalf("after: %s", body)
	}

	// Removing it keeps the distribution, and what is in it.
	_, body = uiSend(t, a, "POST", "/v1/boxes/local/uninstall", tok, "")
	lines, last = streamed(t, body)
	all = strings.Join(lines, "\n")
	if last.Error != "" || !strings.Contains(all, "Stopping berthd and removing its service in berth-box…") || !strings.Contains(all, "Kept berth-box") {
		t.Fatalf("remove: %s", body)
	}
	if !strings.Contains(e.log(e.berth), "|forget "+localName()) || strings.Contains(e.log(e.wsl), "--unregister") || !strings.Contains(e.log(e.berthd), "uninstall\n") || e.record() != nil {
		t.Fatalf("remove did more or less than it said:\n%s", e.log(e.wsl))
	}
}

func TestRemovingTheWSLBoxWithItsDataDeletesTheDistribution(t *testing.T) {
	e := newWSLEnv(t)
	a := startAgent(t, e.dir)
	tok := uiToken(t, a)
	if _, body := uiSend(t, a, "POST", "/v1/boxes/local", tok, ""); !strings.Contains(body, `"box":`) {
		t.Fatalf("set up: %s", body)
	}
	_, body := uiSend(t, a, "POST", "/v1/boxes/local/uninstall", tok, `{"remove_data":true}`)
	if _, last := streamed(t, body); last.Error != "" || !strings.Contains(body, "Deleting berth-box") {
		t.Fatalf("remove: %s", body)
	}
	if !strings.Contains(e.log(e.wsl), "--unregister berth-box") {
		t.Fatalf("the distribution was not deleted:\n%s", e.log(e.wsl))
	}
}

func TestWithoutWSLTheBoxAsksForIt(t *testing.T) {
	e := newWSLEnv(t)
	t.Setenv("FAKE_WSL_MISSING", "1")
	a := startAgent(t, e.dir)
	tok := uiToken(t, a)
	_, body := uiSend(t, a, "GET", "/v1/boxes/local", tok, "")
	var st LocalBoxStatus
	json.Unmarshal([]byte(body), &st)
	if !st.NeedsWSL || st.Available || !strings.Contains(st.Reason, "WSL") {
		t.Fatalf("status: %s", body)
	}
	_, body = uiSend(t, a, "POST", "/v1/boxes/local", tok, "")
	if _, last := streamed(t, body); !strings.Contains(last.Error, "isn't installed") {
		t.Fatalf("set up without WSL: %s", body)
	}
	if strings.Contains(e.log(e.wsl), "--install") {
		t.Fatal("it installed a distribution without WSL")
	}
}

func TestLinuxName(t *testing.T) {
	for in, want := range map[string]string{
		`ACME\Ana`:     "ana",
		"devl":         "devl",
		"Ana Lima":     "ana-lima",
		"root":         "berth",
		"1st":          "berth",
		"":             "berth",
		`HOST\dev_ops`: "dev_ops",
	} {
		if got := linuxName(in); got != want {
			t.Errorf("linuxName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOnWindowsThisComputerCanBeABoxOnWindowsItself(t *testing.T) {
	e := newWSLEnv(t)
	for _, name := range []string{"berthd.exe", "berthd-bg.exe", "tmux.exe"} {
		writeFile(t, filepath.Join(e.dir, name), fakeBerthd("win-new"), 0o755)
	}
	wslServing = func(string) bool { return isFile(filepath.Join(e.root, "box", "serving")) }
	a := startAgent(t, e.dir)
	tok := uiToken(t, a)

	_, body := uiSend(t, a, "GET", "/v1/boxes/local", tok, "")
	var st LocalBoxStatus
	json.Unmarshal([]byte(body), &st)
	if !st.NativeAvailable || st.Native {
		t.Fatalf("before: %s", body)
	}

	_, body = uiSend(t, a, "POST", "/v1/boxes/local", tok, `{"native":true}`)
	lines, last := streamed(t, body)
	all := strings.Join(lines, "\n")
	if last.Error != "" || last.Box != localName() {
		t.Fatalf("set up: %s", body)
	}
	bin := filepath.Join(e.root, "bin")
	for _, want := range []string{"Copied berthd.exe to " + bin, "Copied tmux.exe to " + bin, "Installing berthd on Windows, listening on 127.0.0.1:", "Paired as " + localName() + "."} {
		if !strings.Contains(all, want) {
			t.Errorf("set up did not say %q:\n%s", want, all)
		}
	}
	if strings.Contains(e.log(e.wsl), "--install") {
		t.Fatal("a native box went through WSL")
	}
	rec := e.record()
	if rec == nil || !rec.Native || rec.WSL != "" || rec.Program != filepath.Join(bin, "berthd.exe") || !loopback(rec.Listen) {
		t.Fatalf("record = %+v", rec)
	}
	if want := e.root + "|install --listen " + rec.Listen + " --no-tools"; !strings.Contains(e.log(e.berthd), want+"\n") {
		t.Errorf("berthd did not run %q:\n%s", want, e.log(e.berthd))
	}
	if err := trust.NewStore(filepath.Join(e.dir, "boxes.json")).Add(trust.Peer{Name: localName(), Address: rec.Listen, Fingerprint: e.fp, PairedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the box marked local", func() bool {
		s, err := a.client.Status(context.Background())
		return err == nil && len(s.Boxes) == 1 && s.Boxes[0].Local
	})

	// Again, even asked for WSL: it stays the box it is and reconnects.
	_, body = uiSend(t, a, "POST", "/v1/boxes/local", tok, "")
	lines, last = streamed(t, body)
	all = strings.Join(lines, "\n")
	if last.Error != "" || strings.Contains(all, "Copied") || !strings.Contains(all, "berthd is installed and running.") || strings.Contains(e.log(e.wsl), "--install") {
		t.Fatalf("set up again: %s", body)
	}
	_, body = uiSend(t, a, "GET", "/v1/boxes/local", tok, "")
	st = LocalBoxStatus{}
	json.Unmarshal([]byte(body), &st)
	if !st.Native || !st.Installed || !st.Running || st.Box != localName() || st.WSL != "" {
		t.Fatalf("after: %s", body)
	}

	_, body = uiSend(t, a, "POST", "/v1/boxes/local/uninstall", tok, "")
	if _, last := streamed(t, body); last.Error != "" {
		t.Fatalf("remove: %s", body)
	}
	if !strings.Contains(e.log(e.berthd), e.root+"|uninstall\n") || isFile(filepath.Join(bin, "berthd.exe")) || e.record() != nil {
		t.Fatalf("remove left things behind:\n%s", e.log(e.berthd))
	}
}

func TestAProgramMovedAsideAndStillRunningDoesNotStopTheNextUpdate(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "new.exe"), filepath.Join(dir, "berthd.exe")
	writeFile(t, src, "v3", 0o755)
	writeFile(t, dst, "v2", 0o755)
	// v1, moved aside by the last update, can't be deleted: an agent's host
	// still runs it. A directory stands in for a file that won't go.
	if err := os.MkdirAll(filepath.Join(dst+".old", "busy"), 0o755); err != nil {
		t.Fatal(err)
	}
	did, err := copyReplacing(src, dst)
	if err != nil || !did {
		t.Fatalf("copyReplacing = %v, %v", did, err)
	}
	if b, _ := os.ReadFile(dst); string(b) != "v3" {
		t.Fatalf("dst = %q", b)
	}
	aside, _ := filepath.Glob(dst + ".old-*")
	if len(aside) != 1 {
		t.Fatalf("v2 was not moved aside under a name of its own: %v", aside)
	}
	if b, _ := os.ReadFile(aside[0]); string(b) != "v2" {
		t.Fatalf("aside = %q", b)
	}
}
