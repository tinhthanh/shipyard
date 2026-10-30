package agent

import (
	"context"
	"net/http"
	"sync"

	"github.com/cosscom/shipyard/internal/winsys"
)

// This computer's box where it isn't a berthd service on the computer
// itself, as it is on macOS and Linux (localbox.go). On Windows it is one
// of two: in WSL (wslbox.go), or berthd on Windows itself (nativebox.go,
// Labs). localbox.go asks platformBox, which is nil elsewhere, and keeps its
// own way otherwise; what the Windows boxes remember and report rides along
// in the structs below, embedded in localbox.go's.

// platformRecord is what localBoxRecord keeps about a Windows box.
type platformRecord struct {
	// WSL is the distribution the box runs in (wslbox.go), and Listen where
	// it listens; Program is then its path in there.
	WSL    string `json:"wsl,omitempty"`
	Listen string `json:"listen,omitempty"`
	// Native: berthd runs on Windows itself (nativebox.go).
	Native bool `json:"native,omitempty"`
}

// platformStatus is what LocalBoxStatus says about a Windows box.
type platformStatus struct {
	// NeedsWSL: the box runs in WSL, which isn't installed yet (POST
	// /v1/boxes/local/wsl installs it, as an administrator).
	NeedsWSL bool `json:"needs_wsl,omitempty"`
	// WSL is the distribution the box runs in.
	WSL string `json:"wsl,omitempty"`
	// The box can instead run berthd on Windows itself (Labs):
	// NativeAvailable says the app carries what that needs, and Native that
	// this computer's box is one.
	NativeAvailable bool `json:"native_available,omitempty"`
	Native          bool `json:"native,omitempty"`
}

// platformLocal is the agent's state for a Windows box: what keeps the WSL
// box's distribution running, which lasts as long as runCtx, the agent's.
type platformLocal struct {
	keepMu  sync.Mutex
	keepWSL context.CancelFunc
	runCtx  context.Context
}

// platformBoxKind is this computer's box done another way than a service.
type platformBoxKind interface {
	status(ctx context.Context) LocalBoxStatus
	setUp(ctx context.Context, native bool, say sayFunc) (string, error)
	remove(ctx context.Context, removeData bool, say sayFunc) error
	// refresh updates the box after an app update, and says whether it
	// was one of this kind's.
	refresh(ctx context.Context, rec *localBoxRecord) bool
}

// platformBox is this computer's other kind of box, or nil.
func (a *Agent) platformBox() platformBoxKind {
	if localGOOS == "windows" {
		return windowsBox{a}
	}
	return nil
}

// bundledBerthdName is the berthd the app carries for this computer's box:
// on Windows the Linux one, for WSL.
func bundledBerthdName() string {
	if localGOOS == "windows" {
		return wslBerthdName
	}
	return "berthd"
}

// setUpLocalBoxAs is setUpLocalBox; on Windows, native asks for berthd on
// Windows itself rather than in WSL.
func (a *Agent) setUpLocalBoxAs(ctx context.Context, native bool, say sayFunc) (string, error) {
	if p := a.platformBox(); p != nil {
		return p.setUp(ctx, native, say)
	}
	return a.setUpLocalBoxSystem(ctx, say)
}

type windowsBox struct{ a *Agent }

func (w windowsBox) status(ctx context.Context) LocalBoxStatus { return w.a.wslBoxStatus(ctx) }

// setUp keeps a box set up already the kind it is: switching means removing
// it first.
func (w windowsBox) setUp(ctx context.Context, native bool, say sayFunc) (string, error) {
	if rec := w.a.localRecord(); rec != nil {
		native = rec.Native
	}
	if native {
		return w.a.setUpNativeBox(ctx, say)
	}
	return w.a.setUpWSLBox(ctx, say)
}

func (w windowsBox) remove(ctx context.Context, removeData bool, say sayFunc) error {
	return w.a.removeWSLBox(ctx, removeData, say)
}

func (w windowsBox) refresh(ctx context.Context, rec *localBoxRecord) bool {
	switch {
	case rec.WSL != "":
		w.a.refreshWSLBox(ctx, rec)
	case rec.Native:
		w.a.refreshNativeBox(ctx, rec)
	default:
		return false
	}
	return true
}

// platformBoxRoutes are the Windows box's own: POST /v1/boxes/local/wsl asks
// Windows to install WSL, which shows its administrator prompt. It answers
// once the prompt was accepted; the install goes on in a window of its own.
func (a *Agent) platformBoxRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/boxes/local/wsl", func(w http.ResponseWriter, r *http.Request) {
		if localGOOS != "windows" {
			writeError(w, http.StatusBadRequest, "WSL is for this computer's box on Windows")
			return
		}
		if err := winsys.Elevate("wsl.exe", "--install --no-distribution"); err != nil {
			writeError(w, http.StatusConflict, "Windows did not start the WSL install: "+err.Error())
			return
		}
		forgetWSLPresent()
		writeJSON(w, http.StatusAccepted, map[string]bool{"started": true})
	})
}
