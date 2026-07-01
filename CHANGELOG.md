# Changelog

Notable changes to Rivolution, a parallel Qt6 fork of Rivendell
developed independently of Fred Gleason's original project. Newest
entries first.

Pre-fork history (through 2026-06-15) is preserved unchanged in
`ChangeLog.upstream-v4`, which is no longer appended to.

## 2026-07-01 (continued, 17)

- `rivapi/store/liquidsoap_generator.go`: AAC stream output was passing
  `content_type="audio/aacp"` to `output.icecast()`, an argument name
  from an older Liquidsoap API. Liquidsoap 2.4.x's `output.icecast()`
  has no `content_type` argument at all — the equivalent is `format`.
  Fixed the generated field name; caught when the AAC stream's
  `radio.liq` failed to parse and Liquidsoap auto-restart-looped.

## 2026-07-01 (continued, 16)

- `lib/dbversion.h`: bumped `RD_VERSION_DATABASE` 378 -> 379.
  `utils/rddbmgr/updateschema.cpp` and `revertschema.cpp`: widen
  `STATIONS.JACK_VERSION` from `char(16)`/`varchar(16)` to `varchar(64)`.
  The column was sized for a real jackd's short version string; the
  PipeWire JACK shim's `jack_get_version_string()` returns a longer
  descriptive string that was silently failing to save.

## 2026-07-01 (continued, 15)

- `rdservice/rdservice.cpp`: removed `geteuid()!=0` root check (lines
  89–92). rdservice can now run as the `rd` user; the `User=rd` drop-in
  in `conf/systemd/rivendell.service.d/rivolution.conf` is now safe to
  deploy once the rebuilt binary is installed.
- `conf/systemd/pipewire-system.service` (new): system-scope PipeWire
  running as `rd`, socket at `/run/pipewire-system/pipewire-0`. Ubuntu
  26.04 ships only user-scope PipeWire units; this fills the gap.
- `conf/systemd/wireplumber-system.service` (new): system-scope
  WirePlumber bound to `pipewire-system.service`.
- `conf/systemd/rivolution-stack.target`: added `Wants=` for both new
  PipeWire services.
- `conf/systemd/rivendell.service.d/rivolution.conf` and
  `conf/systemd/liquidsoap.service`: added
  `Environment=XDG_RUNTIME_DIR=/run/pipewire-system` so caed's JACK
  calls and Liquidsoap's `input.jack()` route to the system PipeWire
  socket via `pipewire-jack`.
- `rivapi/store/service_status.go`: added PipeWire and WirePlumber to
  the managed unit list (System page).
- `docs/specs/0007-pipewire-audio-engine.md`: added Phase 1
  implementation section documenting the system-scope setup, runtime
  prerequisites, and known gap vs. Phase 2.

## 2026-07-01 (continued, 14)

- `rivapi/dashboard/templates/home.html`: new home page — four nav buttons
  (System, Broadcast, Groups, Carts). Placeholder for future system status.
- `rivapi/dashboard/handlers.go`: `Root` now renders the home page instead
  of redirecting to `/groups`. Login already redirected to `/`; landing page
  is now home rather than groups.

## 2026-07-01 (continued, 13)

- `rivapi/store/service_status.go`: switched from `systemctl is-active`
  to `systemctl show --property=ActiveState,SubState` for richer state
  reporting; added `Detail` field to `ServiceStatus` surfacing informative
  sub-states (e.g. "activating (auto-restart)", "activating (start-post)");
  added `Warn` field and `liquidsoapWarn()` health check that reads the last
  8 KB of the Liquidsoap log for a JACK device failure within the last 10
  minutes — clears automatically once PipeWire bridge is running.
- `rivapi/dashboard/templates/system_status.html`: render Detail in muted
  text and Warn as a warning line (⚠) below the state chip.
- `rivapi/dashboard/templates/broadcast.html`: moved "+ Add stream" and
  "Save & Deploy" to separate rows so they don't overlap on narrow viewports.

## 2026-07-01 (continued, 12)

- `rivapi/dashboard/templates/broadcast.html`: moved "+ Add stream"
  button from the streams section header to below the last stream card,
  paired with "Save & Deploy" in a bottom action row. New stream fields
  now appear immediately above the button when clicked.

## 2026-07-01 (continued, 11)

- `docs/specs/0010-systemd-stack-orchestration.md`: added implementation
  deviation note and deployment prerequisite warning for the
  `rivendell.service.d/rivolution.conf` drop-in. `rdservice/rdservice.cpp`
  lines 89–92 contain a hardcoded `geteuid()!=0` check that exits with
  `ExitNoPerms` when the service runs as `rd`; combined with
  `Restart=always` and a 30-second `ExecStartPost` probe this creates a
  ~32-second infinite restart cycle. The drop-in must not be deployed
  until the check is removed and `rdservice` rebuilt.

## 2026-07-01 (continued, 10)

- `rivapi/store/liquidsoap_generator.go`: `liqStreamURL()` — auto-
  generates the `url=` parameter in each `output.icecast()` call as
  `http://icecast_host:icecast_port/mount` (the direct playback URL
  Icecast renders as a "Listen Live" link). A per-stream URL override
  is still accepted for reverse-proxy deployments. `station.url` no
  longer falls through to this field — it is the station website only.
- `rivapi/dashboard/templates/broadcast.html`: per-stream "Stream URL
  override" field placeholder now shows the auto-computed value so the
  operator can see what will be generated without filling it in.

## 2026-07-01 (continued, 9)

- `rivapi/store/icecast_generator.go`: removed `<burst-on-connect>`,
  `<queue-size>`, `<client-timeout>`, `<header-timeout>`,
  `<source-timeout>` from the `<limits>` block — all were removed from
  the Icecast 2.5.0 XML schema, causing config validation failure and
  the "obsolete tags" + "did not validate" errors in the admin dashboard.
- `rivapi/store/liquidsoap_generator.go`: touch the log file
  (`O_CREATE|O_APPEND`) after creating the log directory — Liquidsoap
  opens the path on start and fails if the file doesn't exist.
- `rivapi/dashboard/templates/broadcast.html`: Icecast Admin ↗ button
  (computed URL from hostname:port, opens `/admin/` in new tab); per-
  stream computed stream URL display (`http://host:port/mount`);
  station URL field re-labeled "Station website URL" with explanatory
  note; per-stream website URL override changed from `type="url"` to
  `type="text"` (field accepts paths like `/192`, not only full URLs).

## 2026-07-01 (continued, 8)

- `rivapi/store/icecast_generator.go`: added `<mount type="normal">`
  sections to the generated icecast.xml, one per configured stream, so
  mount points are visible in the config file (previously absent).
- `rivapi/store/broadcast_config.go`: fixed default Liquidsoap log path
  from `/home/rd/logs/liquidsoap.log` (wrong) to `/home/rd/Log/liquidsoap.log`
  (correct path on the host); Liquidsoap fails on start if the path is wrong.
- `rivapi/store/liquidsoap_generator.go`: added `os.MkdirAll` for the
  configured log directory at deploy time so Liquidsoap can open its log
  immediately on first start.
- `conf/systemd/liquidsoap.service` (new): full standalone unit — the
  Ubuntu `liquidsoap` package ships no service unit, so the drop-in
  ExecStart override had nothing to apply to.
- `conf/systemd/liquidsoap.service.d/rivolution.conf`: removed `[Service]`
  ExecStart lines (now in the main unit above); kept `[Unit]` After/PartOf.

## 2026-07-01 (continued, 7)

- `rivapi/store/broadcast_config.go` (new): `BroadcastConfig` type
  (station defaults, per-stream overrides, Icecast and Liquidsoap
  config structs), `LoadBroadcastConfig` (reads JSON, returns defaults
  when file absent), `SaveBroadcastConfig` (atomic write).
- `rivapi/store/icecast_generator.go` (new): generates `icecast.xml`
  from `BroadcastConfig`; `<sources>` auto-calculated from stream
  count; installs to `/etc/icecast2/icecast.xml` via scoped
  `sudo install`.
- `rivapi/store/liquidsoap_generator.go` (new): generates `radio.liq`
  from `BroadcastConfig`; MP3 uses `%mp3`, HE-AAC v1/v2 use
  `%external` + `fdkaac` CLI with `content_type="audio/aacp"`, OGG
  uses `%ogg(%vorbis)`.
- `rivapi/config/config.go`: added `BroadcastConfigPath` /
  `RIVAPI_BROADCAST_CONFIG` (default
  `/home/rd/etc/rivolution/broadcast.json`).
- `rivapi/dashboard/handlers_broadcast.go` (new): `Broadcast` (GET
  `/broadcast`) and `BroadcastSave` (POST `/broadcast/save`). Save
  flow: write JSON → generate icecast.xml → install → generate
  radio.liq → restart icecast2 → restart liquidsoap (exit code 5
  treated as warning, not error). Result banner embedded in page.
- `rivapi/dashboard/templates/broadcast.html` (new): three
  collapsible sections — Station, Icecast, Liquidsoap — plus a
  dynamic stream list managed by Alpine.js. Add/remove streams; codec
  dropdown (MP3/HE-AAC v1/HE-AAC v2/OGG Vorbis); per-stream metadata
  override expander. Streams serialized as JSON into a hidden field
  before submit.
- `rivapi/dashboard/templates/base.html`: added Broadcast nav link.
- `rivapi/main.go`: wired `GET /broadcast`, `POST /broadcast/save`.
- `conf/sudoers.d/rivapi`: added `RIVAPI_INSTALL` alias for the
  scoped `sudo install` command that writes `icecast.xml`.
- `conf/systemd/liquidsoap.service.d/rivolution.conf`: added
  `[Service]` section with `ExecStart` override pointing to
  `/home/rd/etc/liquidsoap/radio.liq`.

## 2026-07-01 (continued, 6)

- `docs/specs/0008-broadcast-tool-suite-integration.md`: added Phase 1
  implementation plan — `BroadcastConfig` data model (station defaults,
  per-stream overrides, Icecast and Liquidsoap config structs), Icecast
  XML and Liquidsoap `.liq` generators, Save & Deploy flow, `/broadcast`
  dashboard UI design (stream list with add/remove/codec/bitrate,
  per-stream metadata overrides), `fdkaac` bundled dependency decision,
  deployment topology note (source-server vs public streaming), and
  Verification section. Critical note updated to reference the spec 0010
  stack start-order solution.

## 2026-07-01 (continued, 5)

- `conf/systemd/rivendell.service.d/rivolution.conf`,
  `conf/systemd/icecast2.service.d/rivolution.conf`,
  `conf/systemd/liquidsoap.service.d/rivolution.conf`: added
  `PartOf=rivolution-stack.target` to each. `Wants=` in the target
  propagates start only; without `PartOf=` on the services, stopping
  or restarting the target had no effect on the running units.
  `stereo-tool.service` already had this; `tailscaled` intentionally
  excluded (stopping the broadcast stack should not drop VPN).

## 2026-07-01 (continued, 4)

- `rivapi/store/service_status.go`: `ControlUnit` now detects `systemctl`
  exit code 5 ("unit not loaded") and returns a friendly error naming the
  file to copy and the `daemon-reload` command needed, rather than a raw
  process error.

- `docs/specs/0010-systemd-stack-orchestration.md`: added Deployment
  section covering manual install steps for all `conf/` files (sudoers,
  systemd units, drop-ins, udev rule), a unified-installer placeholder,
  and a deb-package placeholder. Implementation deviations section
  updated.

- `BACKLOG.md`: added entry documenting that `conf/` deployment files
  have no automated install path — lists all files needing coverage and
  what both the Ansible role and the `postinst` deb hook need to do.

- `docs/handoff/2026-07-01.md` (gitignored): session handoff — full
  account of architecture decisions, what was built, what's working,
  what needs manual deployment, the installer gap, and open items for
  the next session.

## 2026-07-01 (continued, 3)

- `rivapi/dashboard/handlers.go`: fixed Stereo Tool GUI launch — was
  hardcoding `DISPLAY=:0` which misses xRDP sessions on `:10` or other
  display numbers. `launchEnv()` now uses the process's inherited DISPLAY
  if set, otherwise auto-detects from `/tmp/.X11-unix/` (picks the first
  available socket). Returns nil (no-op with error message) if no display
  is found. Added `"os"` import.

## 2026-07-01 (continued, 2)

- `rivapi/store/service_status.go`: renamed display label for
  `rivendell.service` from "Rivendell" to "Rivolution".

- `rivapi/dashboard/handlers.go`: `SystemAction` now always returns HTTP
  200 — control errors are embedded in the returned status fragment via
  `ActionError` rather than returning a 4xx that htmx silently ignores.
  Added 400ms settle delay after a successful action so systemd state is
  current before querying. Added `StereoToolLaunch` handler (`POST
  /system/stereo-tool/launch`): starts the binary with `DISPLAY=:0` in
  the background; returns a result fragment.

- `rivapi/dashboard/templates/system_status.html`: renders `ActionError`
  as a visible banner when set.

- `rivapi/dashboard/templates/system.html`: added "Launch GUI" button for
  Stereo Tool (`POST /system/stereo-tool/launch`).

- `rivapi/main.go`: wired `POST /system/stereo-tool/launch`.

## 2026-07-01 (continued)

- `rivapi/store/stereo_tool_install.go` (new): `StereoToolArch` (detects
  server architecture via `runtime.GOARCH`), `StereoToolDownloadURL`
  (constructs the correct Thimeo URL for the arch and optional pinned
  version — `jack_64` for amd64, `jack_pi2_64` for arm64),
  `StereoToolInstalled` (checks path exists), `InstallStereoTool`
  (downloads binary, atomic temp-file+rename install, 5-minute timeout).

- `rivapi/config/config.go`: added `StereoToolPath` / `RIVAPI_STEREO_TOOL_PATH`
  (default `/home/rd/bin/stereo_tool`; `rd`-owned path avoids privilege
  escalation for dashboard-driven installs).

- `rivapi/dashboard/handlers.go`: extracted `systemData()` helper (shared
  by `System`, `SystemAction`, and populated Stereo Tool fields).
  Added `StereoToolInstall` handler (`POST /system/stereo-tool/install`):
  reads optional `version` form param, calls `store.InstallStereoTool`,
  returns `stereo_tool_result.html` fragment.

- `rivapi/dashboard/templates/system.html`: added Stereo Tool binary section
  below the service status table — shows arch, install path, install status,
  "Install Latest" button, and "Install Version" form (version input +
  submit). htmx posts to `/system/stereo-tool/install` and swaps result
  into `#stereo-tool-result`.

- `rivapi/dashboard/templates/stereo_tool_result.html` (new): install
  outcome fragment (success message or error).

- `rivapi/main.go`: wired `POST /system/stereo-tool/install`.

- `conf/systemd/stereo-tool.service`: updated `ExecStart` path to
  `/home/rd/bin/stereo_tool` to match default `StereoToolPath`.

## 2026-07-01

- `rivapi/store/service_status.go` (new): `QueryStackStatus` polls
  `systemctl is-active` for each managed stack unit and returns current
  state; `ControlUnit` runs `sudo systemctl start/stop/restart` for
  allowed units only (allowlist-validated to prevent injection). Units
  not yet installed surface as `"unknown"` rather than erroring.

- `rivapi/dashboard/handlers.go`: `System` handler (`GET /system`) and
  `SystemAction` handler (`POST /system/service/{unit}/{action}`) added.
  Full page renders via `system.html`; htmx requests return
  `system_status.html` fragment. Action buttons use `hx-confirm` for
  category-3 (audio-path) services per spec 0010 live-playout protection.

- `rivapi/dashboard/templates/system.html` (new): System page — full-stack
  Start/Stop/Restart buttons at top; status table loaded via htmx on page
  load.

- `rivapi/dashboard/templates/system_status.html` (new): htmx-swappable
  service status table with per-unit Start/Stop/Restart buttons and
  state badges.

- `rivapi/dashboard/templates/base.html`: added System nav link.

- `rivapi/dashboard/static/app.css`: added `.service-state` badge styles
  for `active`, `inactive`, `failed`, `activating`, `unknown` states.

- `rivapi/main.go`: wired `/system` GET and `/system/service/{unit}/{action}`
  POST routes under `DashboardMiddleware`.

- `conf/sudoers.d/rivapi` (new): targeted NOPASSWD rule for `rd` to run
  `systemctl start/stop/restart` for the five stack units and the target.

- `conf/systemd/rivolution-stack.target` (new): stack target; `Wants=`
  `rivendell.service`, `icecast2.service`, `liquidsoap.service`,
  `stereo-tool.service`, `tailscaled.service`.

- `conf/systemd/rivendell.service.d/rivolution.conf` (new): drop-in setting
  `User=rd`, `Group=rd`, `LimitRTPRIO=99`, `LimitRTTIME=infinity`,
  `IOSchedulingClass=realtime`, `IOSchedulingPriority=0`, plus an
  `ExecStartPost` health probe on caed's TCP port (5005). Resolves the
  root→rd permission prerequisite for PipeWire integration (Phase 1 of
  two-phase caed migration; see spec 0010).

- `conf/systemd/icecast2.service.d/rivolution.conf` (new): adds
  `After=rivendell.service`.

- `conf/systemd/liquidsoap.service.d/rivolution.conf` (new): adds
  `After=rivendell.service` and `After=icecast2.service`.

- `conf/systemd/stereo-tool.service` (new): custom unit for Stereo Tool;
  `After=liquidsoap.service`; `User=rd`; `PartOf=rivolution-stack.target`.

- `conf/udev/99-ptp.rules` (new): assigns `/dev/ptpN` to `ptp` group
  (Phase 1.5 prerequisite for non-root PTP clock access).

## 2026-06-30 (continued, 4)

- `rivapi/auth/auth.go`: dashboard session cookie is now a browser
  session cookie (no `Expires`/`MaxAge`). Browser discards it on close,
  forcing re-login on next open. JWT expiry still enforces token
  lifetime within a session.

## 2026-06-30 (continued, 3)

- `rivapi/store/carts_db.go` (new): `CartDB` — native MariaDB cart reader
  for admin users, bypassing rdxport's USER\_PERMS filter. Queries the CART
  table directly; converts TYPE int (1/2) to "audio"/"macro" string,
  FORCED\_LENGTH/AVERAGE\_LENGTH milliseconds to "HH:MM:SS.T" format, and
  nullable YEAR date column to a year string, matching rdxport XML output.

- `rivapi/store/store.go`, `rivapi/store/groups_db.go`: exported `IsAdmin`
  (was private `isAdmin`) and added it to the `GroupStore` interface so
  dashboard handlers can route admin cart queries to `CartDB` without a
  type assertion.

- `rivapi/dashboard/handlers.go`: `Carts` and `CartDetail` handlers now
  check `GroupStore.IsAdmin` and use `CartDB` for admin users (direct DB
  query) and `CartProxy` for non-admin users (rdxport). Added error checks
  to all `ExecuteTemplate` calls — previously silent failures now return
  HTTP 500.

- `rivapi/main.go`: constructs `CartDB` and passes it to `dashboard.New`.

## 2026-06-30 (continued, 2)

- `rivapi/store/groups_db.go`: admin users (`ADMIN_CONFIG_PRIV='Y'` in
  USERS table) now see all groups — queries GROUPS table directly instead
  of filtering through USER_PERMS, matching RDAdmin's behaviour. Non-admin
  users continue to see only their permitted groups via USER_PERMS.

- `rivapi/config/config.go`: rivapi now reads `/etc/rd.conf` as its
  primary config source, matching the pattern used by all other Rivendell
  binaries. DB credentials come from the `[mySQL]` section (Hostname,
  Loginname, Password, Database keys). JWT secret comes from the new
  `[dashboard]` section (JwtSecret key). Env vars still override file
  values for dev/container use. Missing or unreadable rd.conf is silent —
  falls back to env vars and hardcoded defaults.

- `conf/rd.conf-sample`: added `[dashboard]` section with `JwtSecret=`
  (generated by first-run), plus optional `StationName`, `LogoURL`,
  `AccentColor` keys for per-station dashboard branding.

- `scripts/rivolution-first-run.sh`: generates a 64-character random JWT
  signing secret and writes it to rd.conf's `[dashboard] JwtSecret` entry,
  same pattern as the existing MySQL password generation step.

## 2026-06-30 (continued)

- Added `rivapi/dashboard/` — browser-facing HTML dashboard shell (spec
  0012). Implements: cookie-based browser session (`rivapi_session`
  HttpOnly cookie carrying the same JWT already used by the JSON API;
  `DashboardLoginHandler` sets it, `LogoutHandler` clears it);
  `DashboardMiddleware` redirects unauthenticated browser requests to
  `/login` instead of returning 401; `GET /login` + `POST /login` +
  `GET /logout` routes; `GET /`, `/groups`, `/carts`, `/carts/{number}`
  dashboard views reusing existing `GroupStore`/`CartStore` interfaces;
  htmx partial-render pattern (same route returns fragment on
  `HX-Request`, full page otherwise); server-rendered Go templates with
  Pico.css + htmx + Alpine.js all vendored in
  `rivapi/dashboard/static/vendor/` (no npm/Node dependency). Base
  layout includes user-switchable light/dark mode via Alpine.js
  `themeManager()` (persists preference to `localStorage`; follows
  system default on first load) and branding slots (station name, logo,
  accent colour, configurable via env vars). Network topology fully
  configurable via `RIVAPI_TRUST_PROXY_HEADERS` and
  `RIVAPI_COOKIE_SECURE`; TLS listener available via `RIVAPI_TLS_CERT` /
  `RIVAPI_TLS_KEY` for direct Tailscale cert use.

- Added `docs/specs/0012-dashboard-foundation.md` — session model, cookie
  auth design, network topology config, template layout and routing
  conventions, branding placeholders, CSRF deferral rationale.

- Added `docs/specs/0013-rdadmin-parity-roadmap.md` — classifies all ~48
  RDAdmin management sections into four implementation buckets (rdxport
  full CRUD, rdxport read-only, native Go+DB with established pattern,
  native Go+DB new surface) and establishes build order. Includes
  Rivolution-exclusive sections (Tailscale, Station Branding, systemd
  stack control) as a separate track.

- Added `docs/specs/0014-tailscale-integration.md` — three-piece design:
  installer Ansible role (`roles/tailscale/`), dashboard Network section
  with auth-key activation + status display, MagicDNS/TLS cert
  provisioning via `tailscale cert`. Documents the hard limit that
  enabling HTTPS Certs tailnet-wide requires the Tailscale admin console,
  not the local CLI.

## 2026-06-30

- Fixed three bugs found during rivapi Phase 1 end-to-end verification:
  (1) `config/config.go`: default `RIVAPI_RDXPORT_URL` corrected to
  `http://127.0.0.1/rd-bin/rdxport.cgi` — `localhost` resolves to `::1`
  on this host, which misses rdxport.cgi's IPv4 loopback auth bypass and
  causes a 403; (2) `store/groups_db.go`: `GROUPS` query wrapped in
  `COALESCE()` for all nullable columns — `COLOR` is NULL in a fresh
  install and a bare `string` scan returns an error; (3) `store/store.go`
  and `store/carts_proxy.go`: `ForcedLength`, `AverageLength`, and `Year`
  fields changed from `int` to `string` — rdxport serialises lengths as
  `"MM:SS.S"` time strings and year as an empty string when unset, both
  of which fail XML-to-int decoding.

- Added `rivapi/` — Go REST API foundation (spec 0005, Phase 1).
  Module path `github.com/anjeleno/rivolution/rivapi`, binary name `rivapi`.
  Implements: `POST /api/v1/auth/login` (rdxport ticket acquisition + JWT
  issuance); `GET /api/v1/groups` (native MariaDB, mirrors
  `web/rdxport/groups.cpp:ListGroups` + `lib/rdgroup.cpp:RDGroup::xml()`);
  `GET /api/v1/carts` and `GET /api/v1/carts/{number}` (rdxport proxy,
  commands 6/7). Internal `GroupStore`/`CartStore` interface boundary allows
  proxy and native-DB implementations to be swapped without HTTP handler
  changes. Builds clean on ARM64 (`go build ./...`).

- Renamed `debian/control`, `debian/control.src`, and `debian/control.src2`:
  source package and all binary packages renamed from `rivendell`/`rivendell-*`
  to `rivolution`/`rivolution-*`; Maintainer updated to Anjeleno
  `<la90046@gmail.com>`; Qt5 runtime dependencies replaced with Qt6
  equivalents (`libqt5sql5-mysql` → `libqt6sql6-mysql`; `qttranslations5-l10n`
  removed — bundled with Qt6; `qt5-style-plugins` removed — no Qt6 equivalent
  needed); `python3-mysqldb` → `python3-pymysql` (Ubuntu 26.04).

## 2026-06-28

- Parameterized `scripts/rivolution-first-run.sh`: `RIVENDELL_USER`
  (default `rd`) replaces a hardcoded username, and a new
  `RIVENDELL_SKIP_DB_SETUP` flag skips rd.conf creation and the
  database create/grant/schema steps entirely, for pointing a host at
  a database that already exists elsewhere instead of creating one
  locally. Both default to this script's original unparameterized
  behavior, so existing usage is unaffected.
- Confirmed Rivolution builds natively on arm64 with no changes
  required: verified end-to-end on Ubuntu 26.04 arm64 (UTM guest on
  Apple Silicon), same build time and steps as the existing x86_64
  verification. `debian/control` already declares `Architecture: any`
  for every binary package, and neither `configure.ac` nor
  `configure_build.sh` contain any architecture-conditional logic — the
  build system was already architecture-agnostic by construction, this
  just confirms it empirically on real arm64 hardware.

## 2026-06-27

- Worked around an intermittent JVM crash in the DocBook PDF build
  (`docs/rivwebcapi`, `docs/manpages`, `docs/opsguide`, `docs/dtds`,
  `docs/apis`): `fop` was randomly segfaulting inside JIT-compiled core
  JDK methods unrelated to the document being rendered, roughly 1 run
  in 8 on this OpenJDK build. Added `JAVA_TOOL_OPTIONS=
  "-XX:-TieredCompilation"` to each directory's `fop` invocation in
  `Makefile.am`, which skips the JIT tier where the crash originates;
  confirmed clean across multiple repeated runs with the flag where the
  same input reproducibly crashed without it. See
  [`BACKLOG.md`](https://github.com/anjeleno/rivolution/blob/main/BACKLOG.md)
  for the full root-cause writeup and removal condition.
- Fixed `autoreconf -fi` failing repo-wide with `required file
  './ChangeLog' not found`: automake's default GNU strictness requires
  a literal `ChangeLog` file at the repo root, which this fork doesn't
  have under that exact name (its changelog convention is
  `CHANGELOG.md`). Added `ChangeLog` as a symlink to `CHANGELOG.md`
  rather than a second, separately-maintained file — automake's check
  is existence-only and follows symlinks, so this satisfies it with no
  duplicate content to keep in sync.

## 2026-06-26

- Confirmed this fork's local checkout had been lagging behind its own
  already-established GitHub identity: the remote had been
  `anjeleno/rivolution` all along, not a separately named
  `rivendell-v6` repo, and every push this session had already been
  landing on its `main` branch. Cloned a fresh checkout at
  `~/dev/rivolution` to match (a plain rename was avoided since
  autotools/libtool can cache the old absolute source path in
  already-generated build files); the old `~/rivendell-v6` directory
  was kept in place, unmodified, as a build-verified fallback rather
  than deleted outright.
- Fixed `docs/apis`, `docs/manpages`, `docs/dtds`, and
  `docs/rivwebcapi`'s DocBook PDF/HTML build failing on a freshly
  configured tree: their `Makefile.am` rules referenced
  `$(DOCBOOK_STYLESHEETS)` directly at `make` time, but that variable
  only ever existed inside `configure_build.sh`'s own process — it
  never persists into a separate, later `make` invocation, even when
  chained with `&&`. Now reference `$(top_srcdir)/helpers/docbook`
  instead, the symlink `configure.ac` already creates once at
  configure time, removing the dependency on shell environment state
  entirely. Also moved the stylesheet-path detection itself into
  `configure.ac`, checking the filesystem for the real file rather than
  trusting a distro-name guess in `configure_build.sh` — works for any
  distro sharing the same `docbook-xsl-ns` package layout, and warns
  clearly at configure time if no candidate path is found instead of
  failing later with a cryptic FOP error. Also added `COPYING` to
  `.gitignore`: `automake --add-missing` regenerates a stock GPLv3
  template whenever it's absent, but this project uses GPLv2
  (`LICENSES/GPLv2.txt`) and deliberately removed `COPYING` in 2021 —
  the regenerated file was stray noise from rerunning `autogen.sh`,
  not a real project file.
- Fixed three more Qt6 signal renames that silently fail to connect
  at runtime without any compiler diagnostic, found by auditing every
  `SIGNAL(...)` call site in the tree against the actual installed
  Qt6 headers: `QComboBox::activated(const QString &)` (now
  `textActivated`), `QButtonGroup::buttonClicked(int)` (now
  `idClicked`), and `QAbstractSocket::error(QAbstractSocket::
  SocketError)` (now `errorOccurred`). The `QComboBox` one is why
  `RDLibrary`'s manual "Add Cart" dialog would reject a cart number as
  "outside of the permitted range for this group" after switching
  groups in the dropdown — the auto-fill logic that recalculates the
  next free cart number for the newly-selected group never re-ran.
  Fixed at all 46 occurrences across 32 files; see
  [`docs/specs/0006-qt6-migration.md`](https://github.com/anjeleno/rivolution/blob/main/docs/specs/0006-qt6-migration.md)
  for the full file list.
- Fixed `RDWaveFile::createWave()` clobbering `errno` via an unconditional
  `unlink()` of a nonexistent `.energy` sidecar file between the actual
  file open and the caller's check of whether it succeeded, masking the
  real reason a destination file failed to open.
- Documented `mp3gain` as a required runtime dependency in [`INSTALL.md`](https://github.com/anjeleno/rivolution/blob/main/INSTALL.md)
  and the golden-image package list — without it, an MP3-to-MP3 import
  or export that requests loudness normalization silently falls back to
  a full decode/re-encode instead of the bitstream-level gain-patch,
  with no error, just a much slower conversion. Only affects
  normalization on MP3-passthrough-eligible transfers; normalization on
  any other format pair was never affected.
- Fixed `RDImportAudio::Import()` (RDLibrary's manual Import dialog)
  never actually transmitting the user's selected output format to the
  server, and the Format control being unconditionally disabled in
  Import mode by original design. Added an explicit, opt-in "Override
  library default format" checkbox so manual imports can request MP3
  passthrough deliberately, consistent with the Dropbox/`rdimport`
  format-override controls, and reworked the dialog's layout so the
  checkbox and Format row sit in the actual Import-zone instead of
  overlapping the divider line meant for the Export section.
- Fixed RDLibrary's "Add" cart flow deleting a newly-imported cart's
  audio (both the database row and the file in `/var/snd`) whenever the
  Edit Cart dialog was closed any way other than the explicit OK button
  — now checks for already-persisted audio before allowing any rollback,
  regardless of how the dialog was closed.
- Updated [`INSTALL.md`](https://github.com/anjeleno/rivolution/blob/main/INSTALL.md)'s generic prerequisites list from "Qt5 Toolkit,
  v5.9 or better" to Qt6, listing the actual modules `configure.ac`
  requires and the verified-working version.
- Marked RHEL/CentOS/Fedora/Rocky support as deliberately abandoned,
  with CentOS losing upstream support being the deciding factor —
  the existing build-system code (`configure.ac`'s distro-detection
  branch, `rivendell.spec.in`, `conf/rivendell-rhel.pam`, the `make
  rpm` target) is left in place as a starting point for anyone who
  wants to pick it back up, just no longer tested or developed
  against. Removed a [`BACKLOG.md`](https://github.com/anjeleno/rivolution/blob/main/BACKLOG.md)
  entry about an unverified RHEL stylesheet path accordingly, and
  reframed the related [`KNOWN_ISSUES.md`](https://github.com/anjeleno/rivolution/blob/main/KNOWN_ISSUES.md) mention to match.
- Dropped a personal name from `RD_COPYRIGHT_NOTICE` (`lib/rd.h`,
  shown in RDAdmin's System Info dialog and every CLI tool's
  `--version` output) — only the original author's credit belongs
  there — and updated its year range to 2002–2026.

## 2026-06-24

- Fixed `ripcd` never processing any client's login handshake:
  `connect(...,SIGNAL(mapped(int)),...)` against a `QSignalMapper`
  silently fails to connect under Qt6, since the bare `mapped(int)`
  signal was disambiguated into `mappedInt`/`mappedString`/
  `mappedObject` and no longer exists on the class. This broke
  `ripcd`'s per-connection read routing specifically, which in turn
  caused `RDLibrary`'s group/category list to come up empty and
  `rdimport`'s dropbox-watch mode to never start scanning after
  launch — both depend on the same post-login signal chain. Fixed at
  all 52 occurrences across 32 files; see
  [`docs/specs/0006-qt6-migration.md`](https://github.com/anjeleno/rivolution/blob/main/docs/specs/0006-qt6-migration.md)
  for the full file list and why this evaded the original migration's
  build-clean verification.
- Replaced launcher and in-app icons for `rdadmin`, `rdairplay`,
  `rdcatch`, `rdlibrary`, `rdlogedit`, and `rdlogmanager` (PNG, `.ico`,
  and `.xpm` sets, plus the `RDIconEngine`-embedded window icon for
  each) — the previous set made several modules hard to tell apart at
  a glance. Also added dedicated icons for `rdalsaconfig` and
  `rddbconfig`, which previously had no icon of their own and silently
  reused the generic Rivendell icon and `rdadmin`'s icon respectively.
- Fixed `RDAudioImport::runImport()` and `RDAudioStore::runStore()`
  treating any HTTP 200 from `rdxport.cgi` as success regardless of
  what the response body actually contained. Both relied on
  `RDWebResult::readXml()`/`ParseInt()`, two ad-hoc line-scanning
  parsers that quietly returned success/zero on unrecognizable input
  instead of signaling a parse failure — so a dead or misconfigured CGI
  endpoint (Apache serving the binary itself instead of executing it,
  for instance) looked exactly like a successful import or a genuinely
  empty audio store. `readXml()` now requires a real `<RDWebResult>`
  root tag before extracting fields, and both callers now treat a
  parse failure as a real error instead of defaulting to success. This
  bug predates this fork; found while diagnosing a dropbox import that
  reported success and deleted its source file despite never actually
  storing any audio.

## 2026-06-23

- Published this fork's first public landing page at
  [rivolution.dev](https://rivolution.dev/).
- Qt6 migration complete: `./configure && make` now succeeds
  end-to-end against Qt6 on Ubuntu 26.04. Beyond the patterns already
  logged below (2026-06-22), full-build verification surfaced 18 more
  distinct Qt6 API removals/changes the original grep sweep missed —
  `QString::sprintf()`→`asprintf()` (57 occurrences, 20 files);
  `QPalette::Background`/`Foreground`→`Window`/`WindowText` (169
  occurrences, 39 files); `QFontMetrics::width(QString)`→
  `horizontalAdvance()` (77 occurrences, 23 files);
  `Qt::TextColorRole`/`BackgroundColorRole`; `Qt::MidButton`;
  `QDate::shortDayName()` and siblings; `QDesktopWidget` removed
  entirely (→ `QScreen`); `QMouseEvent`/`QDropEvent` position
  accessors; `QWheelEvent::orientation()`/`delta()`; `QVariant::Type`
  on `QMimeData`'s virtual interface; `QTextStream::setCodec()`
  removed (Core5Compat split); a second `QList::swap(int,int)` fix
  idiom (`swapItemsAt`) alongside the first; a `QMap`/`QMultiMap`
  iterator-type split; a missing `QFile` include; `QWidget::enterEvent`'s
  widened `QEnterEvent*` signature; `QDateTime(const QDate&)`; and
  `QLabel::pixmap()`'s value-vs-pointer return change. Full detail in
  [`docs/specs/0006-qt6-migration.md`](https://github.com/anjeleno/rivolution/blob/main/docs/specs/0006-qt6-migration.md).
- Version bumped to `6.0.0int0` (from `4.4.1int3`) in
  `versions/PACKAGE_VERSION`. `versions/README.txt` gained a short
  explanation of the existing `intN` pre-release suffix convention
  (inherited from upstream, not new) and a note that
  `debian/changelog` is regenerated from `debian/changelog.src` by
  `autogen.sh`, not hand-edited.
- `debian/changelog.src`'s maintainer line updated to this fork's own
  identity, separate from upstream's.
- Ubuntu 26.04 build-compatibility fixes unrelated to Qt6 itself:
  MusicBrainz pkg-config module names renamed (`libmusicbrainz5`→
  `libmusicbrainz5cc`, `libcoverart`→`libcoverartcc`), with the
  matching `librd_la_LIBADD` linkage added in `lib/Makefile.am`;
  ImageMagick detection tries the unversioned `Magick++` pkg-config
  name first, falling back to the old `Magick++-6.Q16` name (Ubuntu
  26.04 ships ImageMagick 7 only, under which the old name doesn't
  exist); `LT_OUTPUT` added to `configure.ac` since libtool ≥2.4.7 no
  longer generates `./libtool` as a side effect of
  `AC_PROG_LIBTOOL`/`LT_INIT`, which the existing rpath workaround
  needs present at configure time.
- Fixed: a fresh database connection (RDDB Config, `rdadmin`, the
  `panel_copy`/`rdcatch_copy` importers) failed with `QSqlDatabase: can
  not load requested driver`. The actual Qt6 MySQL/MariaDB driver
  plugin names are `QMYSQL`/`QMARIADB`; `lib/rd.h`'s
  `DEFAULT_MYSQL_DRIVER`, `conf/rd.conf-sample`'s `Driver=` line,
  `docs/manpages/rd.conf.xml`, and both importers all still referenced
  the legacy Qt3-era name `QMYSQL3`, which doesn't exist as a loadable
  driver under Qt6 (or, in fact, modern Qt5 — this was already stale,
  just never hit until now). All five corrected to `QMYSQL`.

## 2026-06-22

- Established this fork's public identity as Rivolution: the GitHub
  repository was created under that name
  ([`anjeleno/rivolution`](https://github.com/anjeleno/rivolution)),
  distinct from "rivendell-v6," the working name used locally and in
  conversation for some time afterward.
- Qt6 migration (in progress, `feature/qt6-migration`, not yet merged):
  `configure.ac` now requires Qt6 (`Qt6Core`/`Qt6Widgets`/`Qt6Gui`/
  `Qt6Network`/`Qt6Sql`/`Qt6Xml`/`Qt6WebEngineWidgets`) instead of Qt5,
  with `QT_DISABLE_DEPRECATED_BEFORE=0x060000` added as a build-time
  completeness check. `moc`/`uic`/`rcc`/`lupdate`/`lrelease` detection
  rewritten for Qt6's real packaging (no `-qt5`-style suffix
  convention, unsuffixed binaries outside `PATH`, and a `qtchooser`
  trap that silently resolves to an old Qt5 install if not handled
  explicitly). `QRegExp` replaced with `QRegularExpression` in the four
  files using it; `QString::KeepEmptyParts`/`SkipEmptyParts` replaced
  with `Qt::KeepEmptyParts`/`SkipEmptyParts` everywhere (94 occurrences,
  49 files); every `Makefile.am`'s `-std=c++11` bumped to `-std=c++17`
  (Qt6's own hard minimum). `QWebView` replaced with `QWebEngineView`
  in `RDAirPlay`'s message-display widget, including a real behavioral
  fix (`QWebEnginePage` has no `mainFrame()` — scrollbar hiding moves to
  a `QWebEngineSettings::ShowScrollBars` page setting instead). See
  [`docs/specs/0006-qt6-migration.md`](https://github.com/anjeleno/rivolution/blob/main/docs/specs/0006-qt6-migration.md) and
  [`docs/specs/0009-qtwebengine-migration.md`](https://github.com/anjeleno/rivolution/blob/main/docs/specs/0009-qtwebengine-migration.md).
- Fixed: MP3 gain-patch normalization (added 2026-06-21) silently never
  applied any gain shift. The requested level was read as hundredths of
  a dB, but every consumer of this setting elsewhere in the pipeline
  (`RDAudioConvert`'s own normalization, and `rdimport`'s own conversion
  before sending it over the wire) has always used plain whole dB —
  e.g. a Dropbox configured for -13dBFS was read as -0.13dB, rounding
  the computed gain-patch step to zero. `mp3gain` still ran and rewrote
  some header bytes, so the import completed normally with no error,
  just a still-unnormalized file. See
  [`docs/specs/0004-mp3-gain-patch.md`](https://github.com/anjeleno/rivolution/blob/main/docs/specs/0004-mp3-gain-patch.md).
- Added the configured Target Audio Format (PCM16/PCM24/MPEG Layer 2/
  MPEG Layer 3) to the Dropbox-flags dump at the top of `rdimport.log`,
  alongside the other already-logged per-Dropbox settings.

## 2026-06-21

- Added MP3 gain-patch normalization: a same-format MP3-to-MP3 import
  that requests normalization (the common case for most Dropboxes) can
  now still take a fast path — the requested gain is patched directly
  into each frame's `global_gain` field via `mp3gain`, instead of always
  falling through to a full decode/re-encode. Falls back to the existing
  conversion path whenever the patch isn't cleanly applicable. New
  runtime dependency: `mp3gain` (packaged for Ubuntu and Debian, amd64
  and arm64). See [`docs/specs/0004-mp3-gain-patch.md`](https://github.com/anjeleno/rivolution/blob/main/docs/specs/0004-mp3-gain-patch.md).
- Fixed: MP3 passthrough (import) ignored a Dropbox's configured
  normalization/autotrim level whenever the source was already MP3 and
  the target format was also MP3 — the only acknowledgment was a syslog
  warning, never actually applied. Normalization/autotrim now requires
  falling through to the full decode/process/re-encode path, since
  neither is possible on a byte-for-byte passthrough copy. See
  [`docs/specs/0003-mp3-waveform-energy.md`](https://github.com/anjeleno/rivolution/blob/main/docs/specs/0003-mp3-waveform-energy.md).
- Fixed two more bugs in the new MP3 waveform/peak energy feature, found
  during pre-build review: peaks computed during MP3 import/encoding
  could be undercounted (a signed-value comparison ignored negative-going
  excursions), and a same-format passthrough import could persist a
  permanently-empty peak chunk, leaving that cut's waveform blank
  forever with no recovery. See [`docs/specs/0003-mp3-waveform-energy.md`](https://github.com/anjeleno/rivolution/blob/main/docs/specs/0003-mp3-waveform-energy.md).
- Fixed generated helper scripts (`helpers/install_python.sh`,
  `helpers/rdi18n_helper.sh`, `xdg/install_usermode.sh`, `build_debs.sh`)
  losing their executable bit whenever `make` triggers automake's
  per-file regeneration via `config.status`, instead of only a full
  `./configure` run. The `chmod` is now part of each file's own
  `AC_CONFIG_FILES` recipe in `configure.ac`, so it reruns on every
  regeneration path.

## 2026-06-20

- Added real MP3 (MPEG Layer III) waveform/peak energy display: actual
  decoded peak data via `libmad`, persisted to the file's own `LEVL`
  chunk so repeat views don't re-decode from scratch. Previously MP3
  cuts had no real waveform in "Edit Markers" at all. See
  [`docs/specs/0003-mp3-waveform-energy.md`](https://github.com/anjeleno/rivolution/blob/main/docs/specs/0003-mp3-waveform-energy.md).

## 2026-06-18

- Fixed: MP3 passthrough (import and export) could produce a file
  whose real sample rate doesn't match the system's output rate,
  which `caed`'s MPEG playback path doesn't resample — audible as
  pitch/speed-shifted ("helium") playback. Passthrough now requires
  the source's real sample rate to match the system rate; otherwise it
  falls through to the existing, correct conversion path. See
  [`docs/specs/0001-mp3-import-format.md`](https://github.com/anjeleno/rivolution/blob/main/docs/specs/0001-mp3-import-format.md).

## 2026-06-17

- Added segue back-timing: when the outgoing element in a segue has
  "No fade on segue out" checked, the next element's start is now
  delayed (when needed) so its intro lands exactly when the outgoing
  element's tail finishes, instead of firing instantly at the segue
  marker regardless of how much lead-in the next element has. No
  effect when "No fade on segue out" is unchecked. See
  [`docs/specs/0002-segue-backtiming.md`](https://github.com/anjeleno/rivolution/blob/main/docs/specs/0002-segue-backtiming.md).
- Added selectable MP3 (MPEG Layer III) as an import coding format,
  alongside the existing PCM16/PCM24/MPEG Layer II options: a new
  `--audio-format=<0|1|2|3>` flag on `rdimport`, a matching override on
  the web import service (`rdxport.cgi`), a per-Dropbox "Target Audio
  Format" setting in RDAdmin, and an MP3 entry in the host-level default
  format dropdown. See [`docs/specs/0001-mp3-import-format.md`](https://github.com/anjeleno/rivolution/blob/main/docs/specs/0001-mp3-import-format.md).
- Added a true passthrough import mode: whenever the source file is
  genuinely MP3 and the target format is also MP3, the server always
  copies the file directly instead of decoding and re-encoding it
  through LAME — unconditional, no flag or setting needed, since
  there's never a reason to re-encode an MP3 to MP3.
- Fixed: `utils/rdimport/rdimport.cpp`'s local format switch was missing
  a PCM24 case (present in the web import path and the RDAdmin UI but
  not the CLI tool) — added for consistency.
- Schema: added `DROPBOXES.CODING_FORMAT` (database schema version
  377 → 378).
- Fixed: the new "Target Audio Format" label on the Dropbox editor was
  clipped on its left edge (right-aligned text in a box too narrow for
  it) — widened and shifted the dropdown over to make room.
- Fixed: passthrough import failures (e.g. a write/access error) showed
  the nonsensical "Audio Converter Error: OK" instead of a real message,
  because the new error-exit calls left out the audio converter error
  code. Now reports a correct error.
- Added the same MP3-to-MP3 passthrough optimization to audio export
  (RDLibrary's per-cut "Import/Export" dialog, and anywhere else that
  uses the `rdxport.cgi` export service): exporting an already-MP3 cut
  back to MP3 now copies the file directly instead of re-encoding it,
  as long as the export is a plain, full-length, unmodified copy (no
  trimming, forced-length speed adjustment, normalization, or embedded
  metadata requested — any of those still go through the normal export
  path exactly as before).
