package dashboard

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/anjeleno/rivolution/rivapi/auth"
	"github.com/anjeleno/rivolution/rivapi/config"
	"github.com/anjeleno/rivolution/rivapi/store"
)

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
	tmplLogin      = template.Must(template.ParseFS(assets, "templates/login.html"))
	tmplGroups     = pageTmpl("groups.html")
	tmplCarts      = pageTmpl("carts.html")
	tmplCartDetail = pageTmpl("cart_detail.html")
	tmplGroupsList = template.Must(template.ParseFS(assets, "templates/groups_list.html"))
	tmplCartsList  = template.Must(template.ParseFS(assets, "templates/carts_list.html"))
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

// Root redirects to /groups.
func (h *Handler) Root(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/groups", http.StatusSeeOther)
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
