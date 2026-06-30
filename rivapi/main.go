package main

import (
	"database/sql"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	_ "github.com/go-sql-driver/mysql"

	"github.com/anjeleno/rivolution/rivapi/api"
	"github.com/anjeleno/rivolution/rivapi/auth"
	"github.com/anjeleno/rivolution/rivapi/config"
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

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Post("/api/v1/auth/login", auth.LoginHandler(cfg, tickets))

	r.Group(func(r chi.Router) {
		r.Use(auth.Middleware(cfg))
		r.Get("/api/v1/groups", api.ListGroups(groupStore))
		r.Get("/api/v1/carts", api.ListCarts(cartStore, tickets))
		r.Get("/api/v1/carts/{number}", api.GetCart(cartStore, tickets))
	})

	log.Printf("rivapi listening on %s", cfg.ListenAddr)
	if err := http.ListenAndServe(cfg.ListenAddr, r); err != nil {
		log.Fatal(err)
	}
}
