package dashboard

import (
	"errors"
	"net/http"
	"strings"

	"github.com/anjeleno/rivolution/rivapi/auth"
	"github.com/anjeleno/rivolution/rivapi/store"
)

type modePageData struct {
	baseData
	Status      store.ModeStatus
	ApplyError  string
	Steps       []string
	JustApplied bool
}

func (h *Handler) modePageData(r *http.Request) (modePageData, error) {
	cfg, err := store.LoadModeConfig(store.ModeConfigPath)
	if err != nil {
		return modePageData{}, err
	}
	return modePageData{
		baseData: h.base(r, "Mode", "mode"),
		Status:   store.QueryModeStatus(cfg),
	}, nil
}

// Mode handles GET /mode.
func (h *Handler) Mode(w http.ResponseWriter, r *http.Request) {
	data, err := h.modePageData(r)
	if err != nil {
		http.Error(w, "error loading mode status: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tmplMode.ExecuteTemplate(w, "base", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// ModeApply handles POST /mode/apply: parses the requested mode + any
// remote connection details, saves it as the new intent, then actually
// switches the box over to it. Always re-renders the full page with a
// step-by-step log, whether it succeeded or stopped partway through --
// same pattern as BroadcastSave, since an operator switching modes needs
// to see exactly how far this got, not just success/failure.
func (h *Handler) ModeApply(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	cfg := store.ModeConfig{
		Mode:                store.InstallMode(strings.TrimSpace(r.FormValue("mode"))),
		RemoteMySQLHost:     strings.TrimSpace(r.FormValue("remote_mysql_host")),
		RemoteMySQLUser:     strings.TrimSpace(r.FormValue("remote_mysql_user")),
		RemoteMySQLPassword: r.FormValue("remote_mysql_password"),
		RemoteMySQLDatabase: strings.TrimSpace(r.FormValue("remote_mysql_database")),
		RemoteNFSHost:       strings.TrimSpace(r.FormValue("remote_nfs_host")),
	}

	render := func(applyErr error, steps []string) {
		data, err := h.modePageData(r)
		if err != nil {
			http.Error(w, "template error", http.StatusInternalServerError)
			return
		}
		data.Steps = steps
		if applyErr != nil {
			data.ApplyError = applyErr.Error()
		} else {
			data.JustApplied = true
		}
		if err := tmplMode.ExecuteTemplate(w, "base", data); err != nil {
			http.Error(w, "template error", http.StatusInternalServerError)
		}
	}

	switch cfg.Mode {
	case store.ModeStandalone, store.ModeServer, store.ModeClient:
	default:
		render(errors.New("choose a valid mode (standalone, server, or client)"), nil)
		return
	}

	// Re-authentication gate: switching mode can take a live station's
	// database/audio store offline, so require the operator to re-enter
	// their password immediately before it happens, not just rely on
	// their existing session cookie -- the same real Rivendell-account
	// credential that already gates the whole dashboard (auth.CreateTicket,
	// the same check LoginHandler/DashboardLoginHandler make), not a
	// separate secret. Checked before anything is saved or touched.
	username := auth.UsernameFromContext(r.Context())
	confirmPassword := r.FormValue("confirm_password")
	if confirmPassword == "" {
		render(errors.New("re-enter your password to confirm this mode switch"), nil)
		return
	}
	if _, _, err := auth.CreateTicket(h.cfg, h.tickets, username, confirmPassword); err != nil {
		render(errors.New("password confirmation failed — mode was not changed"), nil)
		return
	}

	// Save intent first, before attempting anything, so a partial failure
	// still leaves a record of what was actually requested -- same
	// reasoning as BroadcastSave saving the JSON config before generating
	// icecast.xml/radio.liq.
	if err := store.SaveModeConfig(cfg, store.ModeConfigPath); err != nil {
		render(err, nil)
		return
	}

	steps, err := store.ApplyMode(cfg)
	render(err, steps)
}
