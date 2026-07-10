package auth

import (
	"crypto/subtle"
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
// Used by LoginHandler (JSON API clients presenting real Rivendell
// credentials). No longer used by DashboardLoginHandler or the /mode
// re-authentication check -- both switched to CheckDashboardPassword
// on 2026-07-09; see that function's comment for why.
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

// DashboardUsername is the only account the dashboard's own login gate
// accepts. Deliberately fixed, not looked up anywhere -- see
// CheckDashboardPassword's comment for why there's exactly one account
// here rather than a user table.
const DashboardUsername = "admin"

// CheckDashboardPassword reports whether password is the dashboard's
// own credential, checked in constant time.
//
// The dashboard's login gate is deliberately independent of both
// Rivendell's own user database and the Linux system account -- not
// PAM (which would need a new dependency and either shadow-group
// membership or a privileged helper just to check a password) and,
// as of 2026-07-09, also not rdxport.cgi's CREATETICKET command,
// after real testing found it returns a valid ticket for the "admin"
// Rivendell account regardless of what password is submitted. That's
// a genuine bug in Rivendell's own C++ authentication path (or in how
// the default admin row's PASSWORD column gets seeded), not anything
// under this package's control, and not something to route the
// dashboard's own login gate through while it's unresolved. See
// docs/handoff/2026-07-09.md for the investigation.
//
// Both DashboardLoginHandler and LoginHandler still call CreateTicket
// afterward, passing cfg.JWTSecret as the rdxport password rather than
// whatever the caller submitted -- rdxport.cgi's check isn't providing
// real protection either way right now, so there's no added risk in
// still using it purely to obtain a genuine ticket for anything
// downstream that needs one (api/carts.go's ListCarts/GetCart, etc.).
// The actual access-control decision already happened here, not there.
func CheckDashboardPassword(cfg *config.Config, password string) bool {
	if cfg.JWTSecret == "" {
		return false // an unconfigured secret must never mean "no password required"
	}
	return subtle.ConstantTimeCompare([]byte(password), []byte(cfg.JWTSecret)) == 1
}

// LoginHandler handles POST /api/v1/auth/login (JSON API clients).
// Same gate as DashboardLoginHandler -- see CheckDashboardPassword's
// comment. API clients authenticate with the same dashboard credential,
// not a separate Rivendell account.
func LoginHandler(cfg *config.Config, tickets *TicketCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req loginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Username == "" || req.Password == "" {
			http.Error(w, "username and password required", http.StatusBadRequest)
			return
		}
		if req.Username != DashboardUsername || !CheckDashboardPassword(cfg, req.Password) {
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}

		expires := time.Now().Add(24 * time.Hour)
		signed, _, err := CreateTicket(cfg, tickets, req.Username, cfg.JWTSecret)
		if err != nil {
			signed, err = IssueToken(cfg, req.Username, expires)
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
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
//
// CheckDashboardPassword is the real gate -- rdxport.cgi's own
// CREATETICKET check isn't a meaningful security boundary right now
// (see that function's comment), so it can't be trusted to lock the
// front door on its own. But since it isn't providing any real
// protection either way, there's no additional risk in still calling
// it afterward, once the real gate has already passed, purely to
// obtain a genuine rdxport ticket -- api/carts.go's ListCarts/GetCart
// and anything else that needs one still work this way. If rdxport is
// unreachable or still broken, ticket acquisition fails gracefully and
// login still succeeds (those features degrade, dashboard access
// doesn't) -- the password check already happened and doesn't depend
// on rdxport.cgi at all.
func DashboardLoginHandler(cfg *config.Config, tickets *TicketCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username := r.FormValue("username")
		password := r.FormValue("password")
		if username != DashboardUsername || !CheckDashboardPassword(cfg, password) {
			http.Redirect(w, r, "/login?error=1", http.StatusSeeOther)
			return
		}

		expires := time.Now().Add(24 * time.Hour)
		signed, _, err := CreateTicket(cfg, tickets, username, cfg.JWTSecret)
		if err != nil {
			// rdxport ticket acquisition failed -- fine, the real gate
			// already passed. Issue the session on its own so dashboard
			// access still works; ticket-dependent features (cart/group
			// browsing) just won't until rdxport is reachable again.
			signed, err = IssueToken(cfg, username, expires)
			if err != nil {
				http.Redirect(w, r, "/login?error=1", http.StatusSeeOther)
				return
			}
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
