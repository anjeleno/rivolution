package api

import (
	"encoding/json"
	"net/http"

	"github.com/anjeleno/rivolution/rivapi/auth"
	"github.com/anjeleno/rivolution/rivapi/store"
)

// ListGroups handles GET /api/v1/groups.
// Returns all groups the authenticated user has permission to see,
// sourced natively from MariaDB (mirrors web/rdxport/groups.cpp:Xport::ListGroups).
func ListGroups(groups store.GroupStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username := auth.UsernameFromContext(r.Context())
		result, err := groups.ListGroups(r.Context(), username)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if result == nil {
			result = []store.Group{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}
