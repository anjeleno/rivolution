# Spec 0012 — Dashboard foundation: session model and template architecture

## Problem

The Go API (`rivapi/`, spec 0005) is designed for API clients using
`Authorization: Bearer <jwt>` headers. A browser-based dashboard
requires a different session model: cookies the browser manages
automatically, without JavaScript having to store or forward a token.
The dashboard also introduces a server-rendered HTML layer (Go
templates, htmx, Alpine.js) that has no precedent in the current
`rivapi/` code and needs its file layout and routing convention locked
before any screen is built.

## Goals

1. Cookie-based browser session that reuses the existing JWT mechanism
   without introducing a new session library.
2. Fully configurable network topology — no hardcoded assumptions about
   which reverse proxy (if any) sits in front of `rivapi`.
3. A `rivapi/dashboard/` subtree that all subsequent dashboard PRs
   extend, with vendored frontend assets and no npm/Node dependency.
4. Branding placeholders baked into the base layout from the first PR
   so they never have to be retrofitted.

## Auth: cookie-wrapped JWT

`rivapi/auth/auth.go`'s `LoginHandler` already issues a valid signed
JWT. For the browser path:

- Add a companion `DashboardLoginHandler` (or a content-negotiation
  branch inside the same handler keyed on `Content-Type`/`Accept`)
  that, instead of returning `{"token":"..."}` JSON, sets an
  `HttpOnly`, `SameSite=Lax` cookie named `rivapi_session` carrying
  the same JWT value, then redirects to `/`.
- Extend `rivapi/auth/middleware.go` to check the `rivapi_session`
  cookie as a fallback when no `Authorization: Bearer` header is
  present. JWT validation logic is identical in both paths; only the
  source of the token string differs.
- A `GET /logout` handler clears the cookie and redirects to `/login`.
- Zero new Go dependencies — the existing `golang-jwt/jwt/v5` and
  `go-chi/chi/v5` handle everything.

### CSRF

Read-only (GET) requests are safe. Write paths (Phase 2 onward) require
CSRF protection. The standard pattern with htmx: generate a per-session
CSRF token, inject it via a `<meta>` tag, configure htmx's
`hx-headers` to include it on every non-GET request, validate
server-side in the relevant handlers. This is a Phase 2 requirement —
not needed for the read-only shell PR.

## Network topology: fully configurable

Production installs vary: plain HTTP on a local network, behind an
existing Apache vhost, or reached directly over a Tailscale tailnet
(HTTP or HTTPS). `rivapi` stays a plain-HTTP listener in all cases;
the two new config env vars control cookie security:

| Var | Default | Meaning |
|-----|---------|---------|
| `RIVAPI_TRUST_PROXY_HEADERS` | `false` | When `true`, read `X-Forwarded-Proto` from a fronting reverse proxy; mark cookie `Secure` if that header says `https`. |
| `RIVAPI_COOKIE_SECURE` | `false` | Force-set the `Secure` cookie flag regardless of proxy headers. Use when `rivapi` is reached directly over HTTPS (e.g. Tailscale with a provisioned cert). |

When neither flag is set the cookie is transmitted over plain HTTP —
correct for LAN deployments where TLS terminates elsewhere or is absent
entirely.

For Tailscale-specific provisioning (auth key activation, MagicDNS,
cert provisioning), see spec 0014.

## File layout

```
rivapi/
  config/
    config.go          ← add two new env vars here
  auth/
    auth.go            ← add DashboardLoginHandler + cookie-setting helpers
    middleware.go      ← extend to accept rivapi_session cookie
  dashboard/
    handlers.go        ← HTML route handlers (call existing store interfaces)
    templates/
      base.html        ← layout with nav shell and Branding slots
      login.html
      groups.html
      groups_list.html ← htmx fragment (inner content only)
      carts.html
      carts_list.html
      cart_detail.html
    static/
      vendor/
        htmx.min.js           ← pinned version, vendored
        alpine.min.js         ← pinned version, vendored
        pico.min.css          ← pinned version, vendored
      app.css                 ← site-level overrides (minimal)
  api/
    groups.go          ← unchanged
    carts.go           ← unchanged
  store/
    ...                ← unchanged
  main.go              ← wire new dashboard routes
```

`rivapi/dashboard/` is distinct from the repo-root `web/` directory
(C++ rdxport.cgi source tree). Source location has no effect on runtime
accessibility.

## Branding

`base.html` receives a `Branding` struct:

```go
type Branding struct {
    StationName string // default: "Rivolution"
    LogoURL     string // default: "" (CSS text-only fallback renders cleanly)
    AccentColor string // default: Pico.css primary color
}
```

For the shell PR, values come from three env vars (`RIVAPI_STATION_NAME`,
`RIVAPI_LOGO_URL`, `RIVAPI_ACCENT_COLOR`) with the Rivolution defaults
when unset. A real per-station branding editor (persisted to the
database, uploaded logo file) is a Bucket-D item for a later PR (see
spec 0013) — the template slots exist from day one so nothing needs to
be retrofitted later.

## Routing convention

- Dashboard HTML at `/login`, `/`, `/groups`, `/carts`, `/carts/{number}`.
- JSON API unchanged at `/api/v1/...` — continues to accept
  `Authorization: Bearer` header; cookie auth is additive.
- Every dashboard handler checks the `HX-Request` header. If set,
  it renders only the inner content template (the `*_list.html` /
  `*_detail.html` fragment). If absent, it wraps the same fragment in
  `base.html` for a full-page response. This is htmx's standard
  partial-render pattern and avoids duplicate routes.

## Reuse

Dashboard handlers call the same `store.GroupStore` and `store.CartStore`
interfaces already wired in `main.go`. Only the output serialization
changes (HTML fragment vs JSON). No new store code is needed for the
shell PR.

## Phase 1 scope (shell PR)

- Login page + cookie-setting handler + logout.
- `GET /groups` — full page and htmx fragment, calls `GroupStore.ListGroups`.
- `GET /carts` — full page and htmx fragment, calls `CartStore.ListCarts`.
- `GET /carts/{number}` — cart detail, calls `CartStore.GetCart`.
- Base layout with nav shell (Groups, Carts, logout) and branding slots.
- Vendored Pico.css, htmx, Alpine.js in `dashboard/static/vendor/`.

Write paths (create/edit/delete for any entity) are Phase 2+, consistent
with spec 0005's staged approach.

## Verification

1. `go build ./...` from `rivapi/` succeeds.
2. Start `rivapi` with `RIVAPI_JWT_SECRET`, `RIVAPI_DB_PASSWORD` set.
3. Open `http://<host>:8080/login` in a browser — login form renders.
4. Submit valid credentials — cookie is set, redirect lands on `/` or
   `/groups`, group list matches `GET /api/v1/groups` JSON output.
5. Navigate to `/carts` — cart list renders.
6. Click a cart — detail page renders.
7. Navigate to `/logout` — cookie cleared, redirect to `/login`, `/groups`
   returns 302 to `/login`.
8. Confirm `GET /api/v1/groups` with a valid `Authorization: Bearer`
   header still returns JSON unmodified (cookie auth is additive,
   not a replacement).
