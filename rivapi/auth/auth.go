package auth

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/anjeleno/rivolution/rivapi/config"
)

// TicketCache holds per-user rdxport tickets keyed by username.
// Tickets are IP-bound to the Go service's own egress address — the service
// is the single consistent caller of rdxport.cgi on behalf of each user,
// not a passthrough for browser IPs.
type TicketCache struct {
	mu      sync.RWMutex
	entries map[string]ticketEntry
}

type ticketEntry struct {
	ticket  string
	expires time.Time
}

func NewTicketCache() *TicketCache {
	return &TicketCache{entries: make(map[string]ticketEntry)}
}

func (tc *TicketCache) Set(username, ticket string, expires time.Time) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.entries[username] = ticketEntry{ticket: ticket, expires: expires}
}

// Get returns (ticket, true) if a valid, unexpired ticket exists for username.
func (tc *TicketCache) Get(username string) (string, bool) {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	e, ok := tc.entries[username]
	if !ok || time.Now().After(e.expires) {
		return "", false
	}
	return e.ticket, true
}

// Claims is the JWT payload issued to the browser client.
type Claims struct {
	jwt.RegisteredClaims
}

// rdxport XML envelope for RDXPORT_COMMAND_CREATETICKET (command 31)
type ticketInfo struct {
	Ticket  string `xml:"ticket"`
	Expires string `xml:"expires"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token   string `json:"token"`
	Expires string `json:"expires"`
}

// SessionCookieName is the HttpOnly cookie carrying the JWT for browser clients.
const SessionCookieName = "rivapi_session"

// CreateTicket forwards credentials to rdxport.cgi and returns (signedJWT, expires, error).
// Shared by LoginHandler (JSON API) and DashboardLoginHandler (browser form).
// Exported (not just package-internal) so dashboard.ModeApply can reuse it as
// a re-authentication check before switching install mode -- the same real
// Rivendell-account credential that already gates the whole dashboard is the
// thing re-checked, not a separate app-only secret or the Linux user's own
// system password (which would need a new PAM dependency and either shadow-
// group membership or a privileged helper to verify at all).
func CreateTicket(cfg *config.Config, tickets *TicketCache, username, password string) (string, time.Time, error) {
	resp, err := http.PostForm(cfg.RdxportURL, url.Values{
		"COMMAND":    {"31"}, // RDXPORT_COMMAND_CREATETICKET
		"LOGIN_NAME": {username},
		"PASSWORD":   {password},
	})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("authentication service unavailable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", time.Time{}, fmt.Errorf("invalid credentials")
	}

	var info ticketInfo
	if err := xml.NewDecoder(resp.Body).Decode(&info); err != nil || info.Ticket == "" {
		return "", time.Time{}, fmt.Errorf("invalid credentials")
	}

	expires := parseRdxportExpiry(info.Expires)
	tickets.Set(username, info.Ticket, expires)

	signed, err := IssueToken(cfg, username, expires)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, expires, nil
}

// LoginHandler handles POST /api/v1/auth/login (JSON API clients).
func LoginHandler(cfg *config.Config, tickets *TicketCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req loginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Username == "" || req.Password == "" {
			http.Error(w, "username and password required", http.StatusBadRequest)
			return
		}

		signed, expires, err := CreateTicket(cfg, tickets, req.Username, req.Password)
		if err != nil {
			if err.Error() == "invalid credentials" {
				http.Error(w, "invalid credentials", http.StatusUnauthorized)
			} else {
				http.Error(w, err.Error(), http.StatusBadGateway)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(loginResponse{
			Token:   signed,
			Expires: expires.Format(time.RFC3339),
		})
	}
}

// DashboardLoginHandler handles POST /login (browser form submission).
// Sets an HttpOnly cookie instead of returning JSON, then redirects to /.
func DashboardLoginHandler(cfg *config.Config, tickets *TicketCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username := r.FormValue("username")
		password := r.FormValue("password")
		if username == "" || password == "" {
			http.Redirect(w, r, "/login?error=1", http.StatusSeeOther)
			return
		}

		signed, _, err := CreateTicket(cfg, tickets, username, password)
		if err != nil {
			http.Redirect(w, r, "/login?error=1", http.StatusSeeOther)
			return
		}

		secure := cfg.CookieSecure
		if !secure && cfg.TrustProxyHeaders {
			secure = r.Header.Get("X-Forwarded-Proto") == "https"
		}

		// No Expires/MaxAge — session cookie only; browser discards on close.
		// The JWT inside still enforces its own expiry within a session.
		http.SetCookie(w, &http.Cookie{
			Name:     SessionCookieName,
			Value:    signed,
			Path:     "/",
			HttpOnly: true,
			Secure:   secure,
			SameSite: http.SameSiteLaxMode,
		})
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

// LogoutHandler clears the session cookie and redirects to /login.
func LogoutHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{
			Name:     SessionCookieName,
			Value:    "",
			MaxAge:   -1,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	}
}

// parseRdxportExpiry parses the <expires> value from rdxport's ticketInfo XML.
// RDXmlField serialises QDateTime as "yyyy-MM-ddTHH:mm:ss" (Qt::ISODate).
func parseRdxportExpiry(s string) time.Time {
	formats := []string{
		"2006-01-02T15:04:05",
		time.RFC3339,
	}
	for _, f := range formats {
		if t, err := time.ParseInLocation(f, s, time.Local); err == nil {
			return t
		}
	}
	// fallback: 24-hour session if the format is unexpected
	return time.Now().Add(24 * time.Hour)
}

// IssueToken creates a signed JWT for the given username and expiry.
// Used when re-issuing a token after ticket refresh.
func IssueToken(cfg *config.Config, username string, expires time.Time) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   username,
			ExpiresAt: jwt.NewNumericDate(expires),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})
	signed, err := token.SignedString([]byte(cfg.JWTSecret))
	if err != nil {
		return "", fmt.Errorf("signing token: %w", err)
	}
	return signed, nil
}
