package dashboard

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/anjeleno/rivolution/rivapi/auth"
	"github.com/anjeleno/rivolution/rivapi/config"
	"github.com/anjeleno/rivolution/rivapi/store"
)

type systemData struct {
	baseData
	Services         []store.ServiceStatus
	Target           string
	StereoToolPath   string
	StereoToolArch   string
	StereoToolExists bool
	ActionError      string // non-empty when the last control action failed
}

type stereoToolResultData struct {
	Success bool
	Message string
}

//go:embed templates/* static/*
var assets embed.FS

// Assets returns the embedded static subtree rooted at "static/",
// so http.FileServerFS serves /vendor/pico.min.css etc. without the "static/" prefix.
func Assets() fs.FS {
	sub, err := fs.Sub(assets, "static")
	if err != nil {
		panic(err)
	}
	return sub
}

// pageTmpl parses base.html + a single page template together. Each call
// returns an independent template.Template so that each page's {{define "page"}}
// is the only definition of that name in its set, avoiding the Go template
// pitfall where parsing multiple {{define "page"}} blocks in one set silently
// drops all but the last one parsed.
func pageTmpl(page string) *template.Template {
	return template.Must(template.ParseFS(assets, "templates/base.html", "templates/"+page))
}

// Pre-parsed template sets — one per page so {{define "page"}} never conflicts.
var (
	tmplLogin        = template.Must(template.ParseFS(assets, "templates/login.html"))
	tmplHome         = pageTmpl("home.html")
	tmplGroups       = pageTmpl("groups.html")
	tmplCarts        = pageTmpl("carts.html")
	tmplCartDetail   = pageTmpl("cart_detail.html")
	tmplSystem       = pageTmpl("system.html")
	tmplBroadcast    = pageTmpl("broadcast.html")
	tmplGroupsList       = template.Must(template.ParseFS(assets, "templates/groups_list.html"))
	tmplCartsList        = template.Must(template.ParseFS(assets, "templates/carts_list.html"))
	tmplSystemStatus     = template.Must(template.ParseFS(assets, "templates/system_status.html"))
	tmplStereoToolResult = template.Must(template.ParseFS(assets, "templates/stereo_tool_result.html"))
)

// Branding holds per-station display values passed to every template.
type Branding struct {
	StationName string
	LogoURL     string
	AccentColor string
}

type baseData struct {
	Branding  Branding
	PageTitle string
	ActiveNav string
}

// Handler bundles the dependencies shared by all dashboard route handlers.
type Handler struct {
	cfg     *config.Config
	groups  store.GroupStore
	carts   store.CartStore
	cartDB  *store.CartDB
	tickets *auth.TicketCache
	brand   Branding
}

func New(cfg *config.Config, groups store.GroupStore, carts store.CartStore, cartDB *store.CartDB, tickets *auth.TicketCache) *Handler {
	return &Handler{
		cfg:     cfg,
		groups:  groups,
		carts:   carts,
		cartDB:  cartDB,
		tickets: tickets,
		brand: Branding{
			StationName: cfg.StationName,
			LogoURL:     cfg.LogoURL,
			AccentColor: cfg.AccentColor,
		},
	}
}

func (h *Handler) base(pageTitle, activeNav string) baseData {
	return baseData{Branding: h.brand, PageTitle: pageTitle, ActiveNav: activeNav}
}

func isHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// LoginPage handles GET /login.
func (h *Handler) LoginPage(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Branding Branding
		Error    string
	}{
		Branding: h.brand,
	}
	if r.URL.Query().Get("error") != "" {
		data.Error = "Invalid username or password."
	}
	if err := tmplLogin.ExecuteTemplate(w, "login.html", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// Root serves the dashboard home page.
func (h *Handler) Root(w http.ResponseWriter, r *http.Request) {
	if err := tmplHome.ExecuteTemplate(w, "base", h.base("Home", "home")); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// Groups handles GET /groups (full page) and GET /groups?partial=1 (htmx fragment).
func (h *Handler) Groups(w http.ResponseWriter, r *http.Request) {
	username := auth.UsernameFromContext(r.Context())
	if r.URL.Query().Get("partial") == "1" || isHTMX(r) {
		groups, err := h.groups.ListGroups(r.Context(), username)
		if err != nil {
			http.Error(w, "error loading groups", http.StatusInternalServerError)
			return
		}
		if err := tmplGroupsList.ExecuteTemplate(w, "groups_list.html", struct{ Groups []store.Group }{groups}); err != nil {
			http.Error(w, "template error", http.StatusInternalServerError)
		}
		return
	}
	if err := tmplGroups.ExecuteTemplate(w, "base", h.base("Groups", "groups")); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// Carts handles GET /carts[?group=NAME] (full page and htmx fragment).
func (h *Handler) Carts(w http.ResponseWriter, r *http.Request) {
	group := r.URL.Query().Get("group")
	username := auth.UsernameFromContext(r.Context())

	if r.URL.Query().Get("partial") == "1" || isHTMX(r) {
		var (
			carts []store.Cart
			err   error
		)
		if h.groups.IsAdmin(r.Context(), username) {
			carts, err = h.cartDB.ListCarts(r.Context(), group)
		} else {
			ticket, ok := h.tickets.Get(username)
			if !ok {
				http.Error(w, "session expired, please log in again", http.StatusUnauthorized)
				return
			}
			carts, err = h.carts.ListCarts(r.Context(), ticket, group)
		}
		if err != nil {
			http.Error(w, "error loading carts", http.StatusInternalServerError)
			return
		}
		if err := tmplCartsList.ExecuteTemplate(w, "carts_list.html", struct{ Carts []store.Cart }{carts}); err != nil {
			http.Error(w, "template error", http.StatusInternalServerError)
		}
		return
	}
	if err := tmplCarts.ExecuteTemplate(w, "base", struct {
		baseData
		GroupFilter string
	}{h.base("Carts", "carts"), group}); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (h *Handler) systemData() (systemData, error) {
	statuses, err := store.QueryStackStatus()
	if err != nil {
		return systemData{}, err
	}
	return systemData{
		baseData:         h.base("System", "system"),
		Services:         statuses,
		Target:           store.StackTarget,
		StereoToolPath:   h.cfg.StereoToolPath,
		StereoToolArch:   store.StereoToolArch(),
		StereoToolExists: store.StereoToolInstalled(h.cfg.StereoToolPath),
	}, nil
}

// System handles GET /system (full page and htmx status fragment).
func (h *Handler) System(w http.ResponseWriter, r *http.Request) {
	data, err := h.systemData()
	if err != nil {
		http.Error(w, "error querying service status", http.StatusInternalServerError)
		return
	}
	if isHTMX(r) || r.URL.Query().Get("partial") == "1" {
		if err := tmplSystemStatus.ExecuteTemplate(w, "system_status.html", data); err != nil {
			http.Error(w, "template error", http.StatusInternalServerError)
		}
		return
	}
	if err := tmplSystem.ExecuteTemplate(w, "base", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// SystemAction handles POST /system/service/{unit}/{action}.
// Always returns 200 with the updated status fragment. Errors from the
// control action are embedded in the fragment (ActionError field) so the
// user sees the problem instead of a silent no-op. A brief settle delay
// gives systemd time to reflect the new state before we query it.
func (h *Handler) SystemAction(w http.ResponseWriter, r *http.Request) {
	unit := chi.URLParam(r, "unit")
	action := chi.URLParam(r, "action")

	var actionErr string
	if err := store.ControlUnit(unit, action); err != nil {
		actionErr = err.Error()
	} else {
		// Give systemd ~400ms to settle before querying the new state.
		time.Sleep(400 * time.Millisecond)
	}

	data, _ := h.systemData()
	data.ActionError = actionErr
	if err := tmplSystemStatus.ExecuteTemplate(w, "system_status.html", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// StereoToolLaunch handles POST /system/stereo-tool/launch.
// Starts the Stereo Tool GUI on the local X display in the background.
func (h *Handler) StereoToolLaunch(w http.ResponseWriter, r *http.Request) {
	result := stereoToolResultData{Success: true, Message: "Stereo Tool launched."}
	if !store.StereoToolInstalled(h.cfg.StereoToolPath) {
		result.Success = false
		result.Message = "Binary not found at " + h.cfg.StereoToolPath + " — install it first."
	} else {
		env := launchEnv()
		if env == nil {
			result.Success = false
			result.Message = "No X11 display found — cannot launch GUI."
		} else {
			cmd := exec.Command(h.cfg.StereoToolPath)
			cmd.Env = env
			if err := cmd.Start(); err != nil {
				result.Success = false
				result.Message = "Launch failed: " + err.Error()
			}
			// Detach: we don't wait for the GUI process.
		}
	}
	if err := tmplStereoToolResult.ExecuteTemplate(w, "stereo_tool_result.html", result); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// launchEnv builds an environment suitable for starting a GUI process.
// It inherits the current process environment, then ensures DISPLAY is set:
// using the process's own DISPLAY if present, otherwise auto-detecting the
// first available X11 socket in /tmp/.X11-unix. Returns nil if no display
// can be found.
func launchEnv() []string {
	env := os.Environ()

	// Check if DISPLAY is already present in the inherited environment.
	for _, e := range env {
		if strings.HasPrefix(e, "DISPLAY=") && len(e) > 8 {
			return env // already set, use as-is
		}
	}

	// Auto-detect: scan /tmp/.X11-unix for the first available socket.
	// Files are named X0, X10, etc.; the display is ":N".
	entries, err := os.ReadDir("/tmp/.X11-unix")
	if err != nil || len(entries) == 0 {
		return nil
	}
	for _, e := range entries {
		name := e.Name()
		if len(name) > 1 && name[0] == 'X' {
			display := ":" + name[1:]
			return append(env, "DISPLAY="+display)
		}
	}
	return nil
}

// StereoToolInstall handles POST /system/stereo-tool/install.
// Downloads and installs the Stereo Tool binary, then returns an install
// result fragment that replaces the stereo-tool-result div.
func (h *Handler) StereoToolInstall(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	version := strings.TrimSpace(r.FormValue("version"))

	result := stereoToolResultData{Success: true}
	if err := store.InstallStereoTool(h.cfg.StereoToolPath, version); err != nil {
		result.Success = false
		result.Message = err.Error()
	} else {
		url := store.StereoToolDownloadURL(version)
		if version == "" {
			result.Message = "Latest build installed from " + url
		} else {
			result.Message = "Version " + version + " installed from " + url
		}
	}

	if err := tmplStereoToolResult.ExecuteTemplate(w, "stereo_tool_result.html", result); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// CartDetail handles GET /carts/{number}.
func (h *Handler) CartDetail(w http.ResponseWriter, r *http.Request) {
	username := auth.UsernameFromContext(r.Context())

	numberStr := chi.URLParam(r, "number")
	if numberStr == "" {
		http.NotFound(w, r)
		return
	}
	var number uint
	if _, err := fmt.Sscanf(numberStr, "%d", &number); err != nil {
		http.NotFound(w, r)
		return
	}

	var (
		cart *store.Cart
		err  error
	)
	if h.groups.IsAdmin(r.Context(), username) {
		cart, err = h.cartDB.GetCart(r.Context(), number)
	} else {
		ticket, ok := h.tickets.Get(username)
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		cart, err = h.carts.GetCart(r.Context(), ticket, number)
	}
	if err != nil || cart == nil {
		http.NotFound(w, r)
		return
	}
	if err := tmplCartDetail.ExecuteTemplate(w, "base", struct {
		baseData
		Cart *store.Cart
	}{h.base(fmt.Sprintf("Cart %d", number), "carts"), cart}); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}
