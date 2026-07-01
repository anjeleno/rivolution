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

// LoginHandler handles POST /api/v1/auth/login.
// Credentials are forwarded to rdxport.cgi as RDXPORT_COMMAND_CREATETICKET;
// on success a JWT is issued and the rdxport ticket stored server-side.
func LoginHandler(cfg *config.Config, tickets *TicketCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req loginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Username == "" || req.Password == "" {
			http.Error(w, "username and password required", http.StatusBadRequest)
			return
		}

		// POST to rdxport.cgi from our fixed IP — ticket binds to this address
		resp, err := http.PostForm(cfg.RdxportURL, url.Values{
			"COMMAND":    {"31"}, // RDXPORT_COMMAND_CREATETICKET
			"LOGIN_NAME": {req.Username},
			"PASSWORD":   {req.Password},
		})
		if err != nil {
			http.Error(w, "authentication service unavailable", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}

		var info ticketInfo
		if err := xml.NewDecoder(resp.Body).Decode(&info); err != nil || info.Ticket == "" {
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}

		expires := parseRdxportExpiry(info.Expires)
		tickets.Set(req.Username, info.Ticket, expires)

		token := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   req.Username,
				ExpiresAt: jwt.NewNumericDate(expires),
				IssuedAt:  jwt.NewNumericDate(time.Now()),
			},
		})
		signed, err := token.SignedString([]byte(cfg.JWTSecret))
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(loginResponse{
			Token:   signed,
			Expires: expires.Format(time.RFC3339),
		})
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
