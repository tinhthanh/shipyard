package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/user"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/cosscom/shipyard/internal/identity"
	"github.com/cosscom/shipyard/internal/winsys"
)

// This computer as a box, on Windows. berthd and the agents it runs need
// tmux and a Unix shell, so the box is a WSL distribution of its own,
// berth-box (Ubuntu 24.04), that Shipyard installs and looks after: a user
// named after this one, tmux and git, and the Linux berthd the app carries
// (berthd-linux-amd64 beside berth.exe) as that user's systemd service,
// listening on 127.0.0.1 in there. WSL forwards Windows' loopback to it, so
// this laptop dials it on 127.0.0.1, as a Mac dials its own box, and nothing
// opens to the network. WSL stops a distribution nothing is using, berthd
// and all, so the agent keeps one process running in it
// (keepWSLBoxRunning). Installing WSL itself is the one step that needs an
// administrator: POST /v1/boxes/local/wsl asks Windows for one.

const (
	wslDistro     = "berth-box"
	wslImage      = "Ubuntu-24.04"
	wslBerthdName = "berthd-linux-amd64"
)

var errNoWSL = errors.New("this computer's box runs in WSL, which isn't installed yet. Install it once (Windows asks for an administrator, and may ask to restart), then set this computer up again")

// wslCmd is wsl.exe with args, without a console window, its output in
// UTF-8 rather than UTF-16. Tests replace it.
var wslCmd = func(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "wsl.exe", args...)
	cmd.Env = append(os.Environ(), "WSL_UTF8=1")
	winsys.HideWindow(cmd)
	return cmd
}

// wslServing is whether the box's berthd answers on addr. Tests replace it.
var wslServing = func(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

// wslText is what wsl.exe printed, without the NULs and carriage returns
// some of its messages carry whatever WSL_UTF8 says.
func wslText(b []byte) string {
	return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(string(b), "\x00", ""), "\r", ""))
}

// wslRun runs wsl.exe and returns what it printed, or an error with it.
func wslRun(ctx context.Context, args ...string) (string, error) {
	out, err := wslCmd(ctx, args...).CombinedOutput()
	text := wslText(out)
	if err != nil {
		if text == "" {
			text = err.Error()
		}
		return text, fmt.Errorf("wsl %s: %s", strings.Join(args, " "), text)
	}
	return text, nil
}

// wslPresent remembers, for a minute, whether WSL is installed: the status
// asks on every look at the Boxes settings.
var wslPresent struct {
	sync.Mutex
	at time.Time
	ok bool
}

func wslInstalled(ctx context.Context) bool {
	wslPresent.Lock()
	defer wslPresent.Unlock()
	if time.Since(wslPresent.at) < time.Minute {
		return wslPresent.ok
	}
	_, err := wslRun(ctx, "--status")
	wslPresent.ok, wslPresent.at = err == nil, time.Now()
	return wslPresent.ok
}

// forgetWSLPresent makes the next look ask WSL again.
func forgetWSLPresent() {
	wslPresent.Lock()
	wslPresent.at = time.Time{}
	wslPresent.Unlock()
}

func wslHasDistro(ctx context.Context) (bool, error) {
	out, err := wslRun(ctx, "--list", "--quiet")
	if err != nil {
		// No distribution at all is an error to some versions of wsl.exe.
		if strings.Contains(strings.ToLower(out), "no installed distributions") {
			return false, nil
		}
		return false, err
	}
	for _, l := range strings.Split(out, "\n") {
		if strings.EqualFold(strings.TrimSpace(l), wslDistro) {
			return true, nil
		}
	}
	return false, nil
}

var linuxUserName = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

// wslUser is the Linux user the box runs as: this Windows user's name, as
// Linux allows it.
func wslUser() string {
	name := ""
	if u, err := user.Current(); err == nil {
		name = u.Username
	}
	return linuxName(name)
}

// linuxName is a Windows account name ("DOMAIN\Name") as a Linux user name,
// or berth when it can't be one.
func linuxName(name string) string {
	if i := strings.LastIndexAny(name, `\/`); i >= 0 {
		name = name[i+1:]
	}
	name = strings.ToLower(name)
	name = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			return r
		}
		return '-'
	}, name)
	if !linuxUserName.MatchString(name) || name == "root" {
		return "berth"
	}
	return name
}

// wslRootSetup runs as root in the distribution, again on each set up: the
// user, systemd and the user as the one wsl.exe starts, tmux and git, and
// lingering, so the user's services run with no one logged in.
const wslRootSetup = `set -e
u="$1"
if ! id "$u" >/dev/null 2>&1; then
  useradd -m -u 1000 -s /bin/bash "$u" 2>/dev/null || useradd -m -s /bin/bash "$u"
fi
printf '[boot]\nsystemd=true\n[user]\ndefault=%s\n' "$u" > /etc/wsl.conf
if ! command -v tmux >/dev/null 2>&1 || ! command -v git >/dev/null 2>&1; then
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -q
  apt-get install -y -q tmux git curl ca-certificates
fi
loginctl enable-linger "$u"
echo "$(id -un "$u") $(getent passwd "$u" | cut -d: -f6)"
`

// wslAsUser runs a program in the distribution as the box's user, with the
// runtime directory systemctl --user needs (a command run through wsl.exe
// has none of its own).
func wslAsUser(u string, prog string, args ...string) []string {
	return append([]string{"-d", wslDistro, "-u", u, "--exec", "/bin/bash", "-c", `export XDG_RUNTIME_DIR=/run/user/$(id -u); exec "$@"`, "bash", prog}, args...)
}

// wslBerthdOutput runs the box's berthd and returns what it printed.
func wslBerthdOutput(ctx context.Context, u, prog string, args ...string) (string, error) {
	cmd := wslCmd(ctx, wslAsUser(u, prog, args...)...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := wslText([]byte(stderr.String())); msg != "" {
			return "", errors.New(strings.TrimPrefix(msg, "berthd: "))
		}
		return "", err
	}
	return wslText(out), nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// wslCopyBerthd puts the app's berthd at prog in the distribution unless the
// same one is there, and says whether it did.
func wslCopyBerthd(ctx context.Context, src, u, prog string, say sayFunc) (bool, error) {
	want, err := fileSHA256(src)
	if err != nil {
		return false, err
	}
	if have, err := wslRun(ctx, "-d", wslDistro, "-u", u, "--exec", "sha256sum", prog); err == nil && strings.HasPrefix(have, want) {
		return false, nil
	}
	from, err := wslRun(ctx, "-d", wslDistro, "--exec", "wslpath", "-a", "-u", src)
	if err != nil {
		return false, err
	}
	say("Copying berthd into %s…", wslDistro)
	if _, err := wslRun(ctx, "-d", wslDistro, "-u", u, "--exec", "install", "-D", "-m", "755", from, prog); err != nil {
		return false, err
	}
	return true, nil
}

// setUpWSLBox is setUpLocalBox on Windows.
func (a *Agent) setUpWSLBox(ctx context.Context, say sayFunc) (string, error) {
	src, err := a.bundledBerthd()
	if err != nil {
		return "", err
	}
	forgetWSLPresent()
	if !wslInstalled(ctx) {
		return "", errNoWSL
	}
	have, err := wslHasDistro(ctx)
	if err != nil {
		return "", err
	}
	if !have {
		say("Installing %s in WSL as %s, for this computer's box: a one-time download of a few hundred MB…", strings.ReplaceAll(wslImage, "-", " "), wslDistro)
		if _, err := wslRun(ctx, "--install", wslImage, "--name", wslDistro, "--no-launch"); err != nil {
			return "", err
		}
		// wsl.exe can say it installed what it didn't (run as another
		// user, say): believe the list.
		if have, err = wslHasDistro(ctx); err != nil || !have {
			return "", fmt.Errorf("WSL did not install %s; installing it again from a terminal (wsl --install %s --name %s) says why", wslDistro, wslImage, wslDistro)
		}
	}
	u := wslUser()
	say("Preparing %s: the user %s, tmux and git…", wslDistro, u)
	out, err := wslRun(ctx, "-d", wslDistro, "-u", "root", "--exec", "/bin/bash", "-c", wslRootSetup, "bash", u)
	if err != nil {
		return "", err
	}
	lines := strings.Split(out, "\n")
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) != 2 || !strings.HasPrefix(fields[1], "/") {
		return "", fmt.Errorf("preparing %s printed %q", wslDistro, out)
	}
	userHome := fields[1]
	prog := userHome + "/.local/bin/berthd"
	a.startWSLKeepAlive()

	copied, err := wslCopyBerthd(ctx, src, u, prog, say)
	if err != nil {
		return "", err
	}
	listen := ""
	if rec := a.localRecord(); rec != nil && rec.WSL != "" && loopback(rec.Listen) {
		listen = rec.Listen
	}
	running := listen != "" && wslServing(listen)
	if !running && listen != "" && !portFree(listen) {
		say("Something else is using %s now; berthd moves to another port.", listen)
		listen = ""
	}
	if listen == "" {
		if listen, err = freeLoopbackPort(a.localPort()); err != nil {
			return "", err
		}
	}
	if copied || !running {
		say("Installing the berthd service in %s, listening on %s: this computer only…", wslDistro, listen)
		cmd := wslCmd(ctx, wslAsUser(u, prog, "install", "--listen", listen, "--no-tools")...)
		if err := streamBerthd(cmd, say, "install"); err != nil {
			return "", err
		}
		if !waitServing(ctx, listen, 20*time.Second) {
			return "", fmt.Errorf("berthd in %s does not answer on %s", wslDistro, listen)
		}
	} else {
		say("berthd is installed and running in %s.", wslDistro)
	}

	run := func(ctx context.Context, args ...string) (string, error) {
		return wslBerthdOutput(ctx, u, prog, args...)
	}
	rec := &localBoxRecord{Program: prog, Home: userHome + "/.config/berth", Owned: true, WSL: wslDistro, Listen: listen}
	name, err := a.pairWith(ctx, run, listen, rec, say)
	if err != nil {
		return "", err
	}
	a.sync()
	a.checkAll(ctx)
	return name, nil
}

func waitServing(ctx context.Context, addr string, d time.Duration) bool {
	end := time.Now().Add(d)
	for {
		if wslServing(addr) {
			return true
		}
		if time.Now().After(end) {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(300 * time.Millisecond):
		}
	}
}

// wslBoxStatus is localBoxStatus on Windows.
func (a *Agent) wslBoxStatus(ctx context.Context) LocalBoxStatus {
	st := LocalBoxStatus{Supported: true, Name: localName(), WSL: wslDistro}
	if _, err := a.bundledBerthd(); err != nil {
		st.Reason = err.Error()
	} else if !wslInstalled(ctx) {
		st.NeedsWSL = true
		st.Reason = errNoWSL.Error()
	} else {
		st.Available = true
	}
	st.NativeAvailable = a.nativeBundle() == nil
	if rec := a.localRecord(); rec != nil && rec.Native {
		st.Native, st.WSL = true, ""
		st.Installed, st.Owned = true, true
		st.Program, st.Listen = rec.Program, rec.Listen
		st.Running = wslServing(rec.Listen)
		if fp, err := identity.ParseFingerprint(rec.Fingerprint); err == nil {
			if peer, ok, _ := a.boxes.Trusted(fp); ok {
				st.Box = peer.Name
			}
		}
	}
	if rec := a.localRecord(); rec != nil && rec.WSL != "" {
		st.Installed, st.Owned = true, true
		st.Program, st.Listen = rec.Program, rec.Listen
		st.Running = wslServing(rec.Listen)
		if fp, err := identity.ParseFingerprint(rec.Fingerprint); err == nil {
			if peer, ok, _ := a.boxes.Trusted(fp); ok {
				st.Box = peer.Name
			}
		}
	}
	return st
}

// removeWSLBox is removeLocalBox on Windows. With removeData the whole
// distribution goes, and with it everything in it: the box's state, and
// repositories cloned there.
func (a *Agent) removeWSLBox(ctx context.Context, removeData bool, say sayFunc) error {
	rec := a.localRecord()
	if rec != nil && rec.Native {
		return a.removeNativeBox(ctx, rec, removeData, say)
	}
	if rec == nil || rec.WSL == "" {
		return errors.New("this computer is not set up as a box")
	}
	if fp, err := identity.ParseFingerprint(rec.Fingerprint); err == nil {
		if peer, ok, _ := a.boxes.Trusted(fp); ok {
			say("Forgetting %s on this laptop…", peer.Name)
			if _, err := a.cli(ctx, "forget", peer.Name).Output(); err != nil {
				return errors.New(cliError(err))
			}
			a.sync()
		}
	}
	if removeData {
		a.stopWSLKeepAlive()
		say("Deleting %s, the WSL distribution the box ran in, and everything in it…", rec.WSL)
		if _, err := wslRun(ctx, "--unregister", rec.WSL); err != nil {
			return err
		}
	} else {
		say("Stopping berthd and removing its service in %s…", rec.WSL)
		u := wslUser()
		cmd := wslCmd(ctx, wslAsUser(u, rec.Program, "uninstall")...)
		if err := streamBerthd(cmd, say, "uninstall"); err != nil {
			return err
		}
		a.stopWSLKeepAlive()
		wslRun(ctx, "--terminate", rec.WSL)
		say("Kept %s; setting this computer up again uses it.", rec.WSL)
	}
	return a.saveLocalRecord(nil)
}

// refreshWSLBox is refreshLocalBox for a box in WSL: after an app update,
// the new berthd goes into the distribution and its service restarts.
func (a *Agent) refreshWSLBox(ctx context.Context, rec *localBoxRecord) {
	src, err := a.bundledBerthd()
	if err != nil {
		return
	}
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
	if a.local.seen == stamp || !wslInstalled(ctx) {
		return
	}
	say := func(format string, args ...any) { a.cfg.Log.Printf("this computer's box: "+format, args...) }
	u := wslUser()
	copied, err := wslCopyBerthd(ctx, src, u, rec.Program, say)
	if err != nil {
		say("%v", err)
		return
	}
	if copied {
		cmd := wslCmd(ctx, wslAsUser(u, rec.Program, "install", "--keep-listen", "--no-tools")...)
		if err := streamBerthd(cmd, say, "install"); err != nil {
			say("reinstalling berthd: %v", err)
			return
		}
		a.checkSoon()
	}
	a.local.seen = stamp
}

// keepWSLBoxRunning keeps the box's distribution running while the agent
// does, once this computer is set up as a box.
func (a *Agent) keepWSLBoxRunning(ctx context.Context) {
	if localGOOS != "windows" {
		return
	}
	a.local.keepMu.Lock()
	a.local.runCtx = ctx
	a.local.keepMu.Unlock()
	if rec := a.localRecord(); rec != nil && rec.WSL != "" {
		a.startWSLKeepAlive()
	}
}

// startWSLKeepAlive runs sleep in the distribution, and again when it ends,
// so WSL doesn't stop it for want of use.
func (a *Agent) startWSLKeepAlive() {
	a.local.keepMu.Lock()
	defer a.local.keepMu.Unlock()
	if a.local.keepWSL != nil {
		return
	}
	parent := a.local.runCtx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	a.local.keepWSL = cancel
	go func() {
		wait := time.Second
		for ctx.Err() == nil {
			start := time.Now()
			err := wslCmd(ctx, "-d", wslDistro, "--exec", "/bin/sleep", "infinity").Run()
			if ctx.Err() != nil {
				return
			}
			if time.Since(start) > time.Minute {
				wait = time.Second
			}
			a.cfg.Log.Printf("this computer's box: what keeps %s running stopped (%v); again in %s", wslDistro, err, wait)
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
			wait = min(2*wait, time.Minute)
		}
	}()
}

func (a *Agent) stopWSLKeepAlive() {
	a.local.keepMu.Lock()
	defer a.local.keepMu.Unlock()
	if a.local.keepWSL != nil {
		a.local.keepWSL()
		a.local.keepWSL = nil
	}
}
