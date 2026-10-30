package agent

import (
	"errors"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/cosscom/shipyard/internal/integrations"
)

// pluginToolRoutes are a plugin's one-time setup on this computer and its
// settings, for the tools it gives agents (Settings → Plugins). manageRoutes
// adds them; POST /v1/plugins/{id}/setup, which starts the setup, is one of
// its actions there.
func (a *Agent) pluginToolRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/plugins/{id}/setup", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if strings.ContainsAny(id, `/\`) || strings.HasPrefix(id, ".") {
			writeError(w, http.StatusNotFound, "no such plugin")
			return
		}
		st, err := integrations.PluginSetupStatus(r.Context(), filepath.Join(a.cfg.UserDir, "plugins"), id)
		switch {
		case errors.Is(err, integrations.ErrPluginOff), errors.Is(err, integrations.ErrNoSetup):
			writeError(w, http.StatusConflict, err.Error())
		case err != nil:
			writeError(w, http.StatusInternalServerError, err.Error())
		default:
			writeJSON(w, http.StatusOK, st)
		}
	})
	// A plugin's settings: switches it lists (GET), one turned on or off
	// at a time (PUT {"key", "value"}). They are not part of the reviewed
	// files, so changing one keeps the plugin allowed.
	mux.HandleFunc("GET /v1/plugins/{id}/settings", func(w http.ResponseWriter, r *http.Request) {
		a.pluginSettings(w, r, r.PathValue("id"))
	})
	mux.HandleFunc("PUT /v1/plugins/{id}/settings", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var req struct {
			Key   string `json:"key"`
			Value *bool  `json:"value"`
		}
		if !decodeBody(w, r, &req) {
			return
		}
		if req.Key == "" || req.Value == nil {
			writeError(w, http.StatusBadRequest, "send a key and a value")
			return
		}
		if strings.ContainsAny(id, `/\`) || strings.HasPrefix(id, ".") {
			writeError(w, http.StatusNotFound, "no such plugin")
			return
		}
		if err := integrations.SetPluginSetting(r.Context(), filepath.Join(a.cfg.UserDir, "plugins"), id, req.Key, *req.Value); err != nil {
			writePluginSettingsError(w, err)
			return
		}
		a.pluginSettings(w, r, id)
	})
}

// PluginSettingsState is what the app shows for a plugin's settings.
type PluginSettingsState struct {
	Label    string                       `json:"label,omitempty"`
	Settings []integrations.PluginSetting `json:"settings"`
}

func (a *Agent) pluginSettings(w http.ResponseWriter, r *http.Request, id string) {
	if strings.ContainsAny(id, `/\`) || strings.HasPrefix(id, ".") {
		writeError(w, http.StatusNotFound, "no such plugin")
		return
	}
	def, list, err := integrations.ListPluginSettings(r.Context(), filepath.Join(a.cfg.UserDir, "plugins"), id)
	if err != nil {
		writePluginSettingsError(w, err)
		return
	}
	if list == nil {
		list = []integrations.PluginSetting{}
	}
	writeJSON(w, http.StatusOK, PluginSettingsState{Label: def.Label, Settings: list})
}

func writePluginSettingsError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, integrations.ErrPluginOff), errors.Is(err, integrations.ErrNoSettings):
		writeError(w, http.StatusConflict, err.Error())
	case strings.HasPrefix(err.Error(), "the plugin has no setting"):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}
