package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/anjeleno/rivolution/rivapi/auth"
	"github.com/anjeleno/rivolution/rivapi/store"
)

// ListCarts handles GET /api/v1/carts.
// Optional query param: ?group=<GROUP_NAME>
// Proxies to rdxport.cgi RDXPORT_COMMAND_LISTCARTS (command 6).
func ListCarts(carts store.CartStore, tickets *auth.TicketCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username := auth.UsernameFromContext(r.Context())
		ticket, ok := tickets.Get(username)
		if !ok {
			http.Error(w, "session expired, please log in again", http.StatusUnauthorized)
			return
		}
		groupName := r.URL.Query().Get("group")
		result, err := carts.ListCarts(r.Context(), ticket, groupName)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if result == nil {
			result = []store.Cart{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}

// GetCart handles GET /api/v1/carts/{number}.
// Proxies to rdxport.cgi RDXPORT_COMMAND_LISTCART (command 7).
func GetCart(carts store.CartStore, tickets *auth.TicketCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username := auth.UsernameFromContext(r.Context())
		ticket, ok := tickets.Get(username)
		if !ok {
			http.Error(w, "session expired, please log in again", http.StatusUnauthorized)
			return
		}
		raw := chi.URLParam(r, "number")
		n, err := strconv.ParseUint(raw, 10, 32)
		if err != nil {
			http.Error(w, "invalid cart number", http.StatusBadRequest)
			return
		}
		result, err := carts.GetCart(r.Context(), ticket, uint(n))
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if result == nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}
