package main

import (
	"database/sql"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	_ "github.com/go-sql-driver/mysql"

	"github.com/anjeleno/rivolution/rivapi/api"
	"github.com/anjeleno/rivolution/rivapi/auth"
	"github.com/anjeleno/rivolution/rivapi/config"
	"github.com/anjeleno/rivolution/rivapi/dashboard"
	"github.com/anjeleno/rivolution/rivapi/store"
)

func main() {
	cfg := config.Load()
	if cfg.JWTSecret == "" {
		log.Fatal("RIVAPI_JWT_SECRET must be set")
	}

	db, err := sql.Open("mysql", cfg.DSN())
	if err != nil {
		log.Fatalf("cannot open database: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatalf("cannot connect to database: %v", err)
	}

	tickets := auth.NewTicketCache()
	groupStore := store.NewGroupDB(db)
	cartStore := store.NewCartProxy(cfg.RdxportURL)
	cartDB := store.NewCartDB(db)
	dash := dashboard.New(cfg, groupStore, cartStore, cartDB, tickets)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// JSON API — Bearer token auth
	r.Post("/api/v1/auth/login", auth.LoginHandler(cfg, tickets))
	r.Group(func(r chi.Router) {
		r.Use(auth.Middleware(cfg))
		r.Get("/api/v1/groups", api.ListGroups(groupStore))
		r.Get("/api/v1/carts", api.ListCarts(cartStore, tickets))
		r.Get("/api/v1/carts/{number}", api.GetCart(cartStore, tickets))
	})

	// Dashboard — browser cookie auth
	r.Get("/login", dash.LoginPage)
	r.Post("/login", auth.DashboardLoginHandler(cfg, tickets))
	r.Get("/logout", auth.LogoutHandler())

	// Static assets (vendored CSS/JS + app.css)
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServerFS(dashboard.Assets())))

	r.Group(func(r chi.Router) {
		r.Use(auth.DashboardMiddleware(cfg))
		r.Get("/", dash.Root)
		r.Get("/groups", dash.Groups)
		r.Get("/carts", dash.Carts)
		r.Get("/carts/{number}", dash.CartDetail)
		r.Get("/system", dash.System)
		r.Post("/system/service/{unit}/{action}", dash.SystemAction)
		r.Post("/system/stereo-tool/install", dash.StereoToolInstall)
		r.Post("/system/stereo-tool/launch", dash.StereoToolLaunch)
		r.Get("/broadcast", dash.Broadcast)
		r.Post("/broadcast/save", dash.BroadcastSave)
		r.Get("/patchbay", dash.Patchbay)
		r.Post("/patchbay/connect", dash.PatchbayConnect)
		r.Post("/patchbay/disconnect", dash.PatchbayDisconnect)
		r.Post("/patchbay/save", dash.PatchbaySave)
	})

	// Patchbay link reconciler: PipeWire links don't survive either
	// endpoint's restart, and WirePlumber's own declarative target-metadata
	// mechanism doesn't apply to JACK-bridged ports (verified 2026-07-01 —
	// see docs/specs/0007-pipewire-audio-engine.md), so persistence is done
	// here instead: poll the saved link set and force the live graph to
	// match it exactly (see ReconcileLinks). 30s, not 5s: this is now
	// authoritative over the whole graph once anything has been saved, so
	// the interval doubles as how long an unsaved ad-hoc test connection
	// survives before being torn back out -- long enough to actually listen
	// and decide, short enough to still self-heal promptly after a reboot.
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if err := store.ReconcileLinks(store.DesiredLinksPath); err != nil {
				log.Printf("patchbay reconcile: %v", err)
			}
		}
	}()

	log.Printf("rivapi listening on %s", cfg.ListenAddr)
	if cfg.TLSCert != "" && cfg.TLSKey != "" {
		log.Printf("TLS enabled (cert: %s)", cfg.TLSCert)
		if err := http.ListenAndServeTLS(cfg.ListenAddr, cfg.TLSCert, cfg.TLSKey, r); err != nil {
			log.Fatal(err)
		}
	} else {
		if err := http.ListenAndServe(cfg.ListenAddr, r); err != nil {
			log.Fatal(err)
		}
	}
}
