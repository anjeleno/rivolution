package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	"github.com/anjeleno/rivolution/rivapi/config"
)

type contextKey int

const usernameKey contextKey = 0

// Middleware validates the JWT and injects the username into the request context.
// Accepts either an Authorization: Bearer <jwt> header (API clients) or the
// rivapi_session HttpOnly cookie (browser clients). Header takes precedence.
func Middleware(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := ""
			if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
				raw = strings.TrimPrefix(h, "Bearer ")
			} else if c, err := r.Cookie(SessionCookieName); err == nil {
				raw = c.Value
			}
			if raw == "" {
				http.Error(w, "missing or invalid Authorization header", http.StatusUnauthorized)
				return
			}

			token, err := jwt.ParseWithClaims(raw, &Claims{}, func(t *jwt.Token) (interface{}, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
				}
				return []byte(cfg.JWTSecret), nil
			})
			if err != nil || !token.Valid {
				http.Error(w, "invalid or expired token", http.StatusUnauthorized)
				return
			}

			claims, ok := token.Claims.(*Claims)
			if !ok || claims.Subject == "" {
				http.Error(w, "invalid token claims", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), usernameKey, claims.Subject)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// DashboardMiddleware is like Middleware but redirects to /login instead of
// returning a 401, making it suitable for browser-facing dashboard routes.
func DashboardMiddleware(cfg *config.Config) func(http.Handler) http.Handler {
	inner := Middleware(cfg)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Detect whether inner middleware would reject — we can't wrap it
			// cleanly, so we re-read the token here to make the redirect decision.
			raw := ""
			if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
				raw = strings.TrimPrefix(h, "Bearer ")
			} else if c, err := r.Cookie(SessionCookieName); err == nil {
				raw = c.Value
			}
			if raw == "" {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
			// Delegate token validation to the shared middleware; if it sends a
			// 401 the browser will see it (htmx will surface it as an error).
			// For full-page navigations this is acceptable for now.
			inner(next).ServeHTTP(w, r)
		})
	}
}

// UsernameFromContext returns the authenticated username injected by Middleware.
func UsernameFromContext(ctx context.Context) string {
	v, _ := ctx.Value(usernameKey).(string)
	return v
}
