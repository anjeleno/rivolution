# 0010 — Systemd stack orchestration

**Date:** 2026-06-30
**Revised:** 2026-07-02

## Goal

Define how the full Rivolution broadcast stack — Rivendell (including
`caed`), Icecast, Liquidsoap, VLC, Stereo Tool, and Tailscale — is
orchestrated as a reliable, correctly ordered set of systemd units with
guaranteed startup sequencing and live-playout-safe reconfiguration,
managed exclusively through the Go dashboard
(`0005-go-api-foundation.md`). No operator should ever need to touch a
systemd unit file or restart a service from a terminal.

## Background

### Current state

External tools (Icecast, Liquidsoap, JACK patches, Stereo Tool) are
launched via desktop shortcuts and scripts in a specific, manually
maintained order. Race conditions occur when services start before their
dependencies are ready to accept connections — a timing failure, not an
ordering failure, and not fixable by boot-time scripts. No mechanism
exists to apply configuration changes without potentially interrupting
live audio.

The unified installer (`anjeleno/rivolution-unified-installer`) lays
down all packages and configuration files but provides no orchestration:
every installed service starts independently, with no inter-service
dependency knowledge. The result is a working set of parts that don't
reliably communicate at startup or after a reboot.

### `caed` permissions: two-phase migration

`caed` currently runs as root — a legacy practice from early Rivendell
that is incompatible with PipeWire, whose system-wide daemon runs under
the `rd` user account. A root-owned process cannot reach a system-scope
PipeWire socket set up under a different identity.

`caed` is not a standalone systemd unit. The upstream `rivendell.service`
unit runs `rdservice`, which spawns `caed`, `ripcd`, `rdcatchd`, and
other daemons as child processes. All inherit root. The migration to
non-root happens in two phases:

**Phase 1 (this spec's implementation):** Write a drop-in override for
`rivendell.service` that sets `User=rd`, `Group=rd`. All subprocesses
inherit the change. This is safe — the `rd` account is the standard
Rivendell service account, and Rivendell's own permission model is
designed around it. `LimitRTPRIO=99` is added at this level so that
`caed`'s real-time scheduling needs are met without requiring root.

**Phase 1.5 (immediately after Phase 1 is verified, not deferred):**
Split `caed` out as its own standalone `caed.service` unit with its own
`User=`, `Group=`, and resource limits. Requires modifying `rdservice`
to skip spawning `caed` when the standalone unit is active. This is the
target state: `caed` gets per-process isolation; other rdservice children
retain their existing resource profile.

The `rd` user account is the standard service account on all properly
installed Rivendell systems; the installer creates it unconditionally.
Unit files in this spec hardcode `rd`. A configurable service account
name is a future installer concern, not a Phase 1 requirement.

### VLC audio routing: not a systemd stack concern

VLC is used ad-hoc to capture live audio and route it into Rivendell.
It is not a background service and has no place in the systemd stack, but must be included in the dependencies. Its audio path — VLC output → Rivendell input — is established by WirePlumber persistent routing policy (spec 0007/0008 territory). When VLC is launched, WirePlumber automatically reconnects its audio port to the correct Rivendell input. No unit file required.

### The race condition, precisely

systemd's `After=` / `Requires=` chains guarantee that unit B's start
is not attempted until unit A's unit state is `active`. But `active`
means the process launched — not that it is ready to accept connections.
A service that appears `active` in `systemctl status` can still be in
its own startup phase, with its listening socket not yet open or its
internal state not yet initialized. A dependent service that connects
immediately on its own start will fail intermittently depending on
actual process initialization time, producing exactly the race condition
this spec must eliminate.

The correct fix is **readiness signaling**: services with downstream
dependents must signal systemd explicitly when they are fully
initialized and accepting clients, via `Type=notify` (the service calls
`sd_notify(0, "READY=1")`) or an `ExecStartPost=` health-probe loop that
polls until the service's socket or API endpoint responds.

## Design

### Service ordering and `rivolution-stack.target`

A custom systemd target, `rivolution-stack.target`, groups the full
broadcast stack and is the unit the dashboard starts, stops, and
monitors.

**The target file is stable infrastructure — it is never regenerated
or rebuilt when operator configuration changes.** It declares `Wants=`
for every service the stack may run. Whether a given service actually
runs is controlled entirely by `systemctl enable` / `systemctl disable`
(or `systemctl mask` for hard prevention). The dashboard calls enable/
disable + start/stop for immediate effect; it never rewrites the target
file itself. The one exception is when a new custom unit file must be
*created* (e.g., a new per-station AES67 bridge unit) — in that case
the dashboard writes the new unit file and calls `systemctl daemon-reload`
transparently. This model is safe for live stations: a misconfigured
unit that fails to start does not bring down other units in the target.

#### Phase 1 dependency chain

```
rivendell.service  (caed + ripcd + rdcatchd + others, User=rd via drop-in)
  ├── icecast2.service         (After=rivendell.service)
  │     └── liquidsoap.service (After=icecast2.service)
  │           └── stereo-tool.service (After=liquidsoap.service)
  └── [RDAirPlay managed as a child of rdservice, not a separate unit]

tailscaled.service  (independent — no audio-path dependency)
```

Icecast must be running before Liquidsoap because Liquidsoap pushes the
encoded stream to Icecast's source port. `rivendell.service` must be
active before Icecast and Liquidsoap because Liquidsoap reads audio from
Rivendell's audio ports. Stereo Tool processes the signal downstream of
Liquidsoap.

Tailscale has no dependency on the audio path and is managed
independently within the target. Its ordering relative to the audio
chain is irrelevant.

#### Phase 1.5 dependency chain (after caed split)

Once `caed` is extracted into its own unit:

```
pipewire-system.service
  └── wireplumber-system.service (After=, Requires=)
        └── caed.service         (After=, Requires=; User=rd, LimitRTPRIO=99)
              ├── icecast2.service
              │     └── liquidsoap.service
              │           └── stereo-tool.service
              └── [rivendell.service minus caed: ripcd, rdcatchd, etc.]

tailscaled.service  (independent)
```

### Readiness signaling

Every service in the chain that has a downstream dependent must signal
readiness before those dependents start:

- **`rivendell.service` (Phase 1):** `rdservice` does not implement
  `sd_notify`. Add an `ExecStartPost=` health probe that polls `caed`'s
  TCP port (5005) until it accepts a connection. Once that succeeds,
  `rivendell.service` is considered fully ready. This is the proxy for
  caed readiness while caed lives inside rdservice.
- **`icecast2.service`:** Verify the upstream unit's readiness mechanism.
  If absent, add an `ExecStartPost=` HTTP probe against
  `http://localhost:8000/status.xsl`.
- **`liquidsoap.service`:** Verify or add a readiness probe.
- **`stereo-tool.service`:** Custom unit; include a probe appropriate to
  Stereo Tool's API or port.
- **`caed.service` (Phase 1.5):** Add native `sd_notify(0, "READY=1")`
  support to `cae/cae.cpp` at the point caed has connected to PipeWire
  and is accepting protocol connections. The `ExecStartPost` probe is
  the interim fallback until that lands.
- **`pipewire-system.service` / `wireplumber-system.service` (Phase 1.5):**
  Verify upstream units use `Type=notify` on Ubuntu 26.04; add probes if
  not.

### Live-playout protection

**The dashboard never restarts `rivendell.service`, or any audio-path
service, as an implicit side effect of writing configuration.** Every
config change the dashboard makes falls into exactly one of three
categories:

**1. Zero-disruption — applies immediately, no restart of any audio
service:**
- WirePlumber routing policy changes: `wpctl` command plus a policy
  file update. Applies live to the running graph.
- Icecast mount/metadata/password changes: restart `icecast2.service`
  only. Liquidsoap reconnects automatically after Icecast restarts.
- Liquidsoap script-only changes that don't alter source or sink
  definitions: restart `liquidsoap.service` only.

**2. Deferred — config written now, applied on next stack restart:**
- Any change to `caed`'s own configuration (audio card assignments,
  PipeWire connection parameters).
- Any change to PipeWire's system-wide config.
- Dashboard marks these as "pending — takes effect on next restart" and
  shows a non-disruptive banner. The operator chooses when to restart.

**3. Explicit user action only — requires warning and confirmation:**
- Full stack restart (`rivolution-stack.target` stop/start).
- `rivendell.service` restart in isolation (interrupts all audio).
- Any restart that touches the live playout path.

The dashboard's confirmation dialog for category 3 must state plainly
that live audio will be interrupted and must offer the option to cancel.

### RDAirPlay restart behavior

Restarting `rivendell.service` also restarts RDAirPlay (it is a child of
rdservice). If a full stack restart is requested, the dashboard makes
this consequence explicit in the confirmation dialog. When individual
external services (Icecast, Liquidsoap) are restarted in isolation,
RDAirPlay is unaffected.

After Phase 1.5 (caed split), restarting `caed.service` in isolation
will leave RDAirPlay in a degraded state (lost daemon connection) —
the dashboard must surface this visibly. The "also restart RDAirPlay"
toggle (default: off) applies to that phase.

### WirePlumber as a system service (Phase 1.5 prerequisite)

WirePlumber normally starts as part of a desktop user's session
(`systemctl --user`). For the Phase 1.5 dependency chain it must run
at system scope:

- Unit: `wireplumber-system.service` (available in the `wireplumber`
  package alongside the per-user unit).
- Config: `/etc/wireplumber/` (system-scope policy files), not
  `~/.config/wireplumber/` (which is session-scoped and unavailable
  before login).
- The dashboard generates and manages files in `/etc/wireplumber/` when
  saving routing policy. Operators never edit these files directly.

### Dashboard as sole interface

The Go API (`rivapi/`) manages systemd state through:

- **Reading:** `systemctl is-active <unit>` polled on-demand, surfaced
  as service health indicators in the dashboard. Units that are not yet
  installed return `unknown` and are displayed as such — the dashboard
  handles this gracefully.
- **Writing configuration:** generates or updates the appropriate config
  file for each service, then applies the minimal restart the change
  category requires.
- **Applying systemd changes:** writes drop-in overrides to
  `/etc/systemd/system/<unit>.d/override.conf` for unit-level changes,
  then calls `systemctl daemon-reload` followed by the appropriate
  restart. Never edits package-owned base unit files directly.
- **Never:** exposes a raw "restart service" button for audio-path
  services without the warning and confirmation described above.

The dashboard requires the `rivapi` process to run with sufficient
privilege to call `systemctl` for the relevant units via a targeted
sudoers rule (`rd` user can start/stop/restart specific units without
password). Raw `sudo` with unrestricted access is not acceptable; the
privilege must be scoped to exactly the units Rivolution manages.
`systemctl is-active` and `systemctl status` do not require privilege
and are called directly.

## Files

### Phase 1 (this implementation)

- New: `/etc/systemd/system/rivapi.service` — systemd unit for the Go
  dashboard process itself. Runs as `rd`, starts after `network.target`,
  `mariadb.service`, and (see deviations below) `pipewire-system.service`.
  No `EnvironmentFile=` — reads DB credentials and the dashboard JWT
  secret from `/etc/rd.conf`, same as every other Rivendell binary.
  `WantedBy=multi-user.target`. Binary installed to `/usr/local/bin/rivapi`.
- New: `/etc/systemd/system/rivolution-stack.target` — groups the full
  broadcast stack; `Wants=` for `rivendell.service`, `icecast2.service`,
  `liquidsoap.service`, `stereo-tool.service`, `tailscaled.service`, and
  (see deviations below) `pipewire-system.service`/
  `wireplumber-system.service`.
- New: `/etc/systemd/system/rivendell.service.d/rivolution.conf` —
  drop-in setting `User=rd`, `Group=rd`, `LimitRTPRIO=99`,
  `LimitRTTIME=infinity`, `IOSchedulingClass=realtime`,
  `IOSchedulingPriority=0`, `Environment=XDG_RUNTIME_DIR=/run/pipewire-system`
  (see deviations below), plus an `ExecStartPost=` health probe polling
  `caed`'s TCP port (5005) for readiness.
- New: `/etc/systemd/system/icecast2.service.d/rivolution.conf` —
  drop-in adding `After=rivendell.service`.
- New: `/etc/systemd/system/pipewire-system.service` — system-scope
  PipeWire, running as `rd`, `RuntimeDirectory=pipewire-system` (creates
  `/run/pipewire-system/`), socket at `/run/pipewire-system/pipewire-0`.
  Required in Phase 1, not deferred to Phase 1.5 — see deviations below.
- New: `/etc/systemd/system/wireplumber-system.service` — system-scope
  WirePlumber, bound to `pipewire-system.service`, same
  `XDG_RUNTIME_DIR`. Also required in Phase 1.
- New: `/etc/systemd/system/liquidsoap.service` — full standalone unit,
  not a drop-in (see deviations below): `ExecStart`,
  `Restart=on-failure`, `RestartSec=5`,
  `Environment=XDG_RUNTIME_DIR=/run/pipewire-system`.
- New: `/etc/systemd/system/liquidsoap.service.d/rivolution.conf` —
  drop-in adding `After=icecast2.service`, `After=rivendell.service`
  (the `[Unit]` ordering stanzas only; `ExecStart` lives in the full
  unit above, not here).
- New: `/etc/systemd/system/stereo-tool.service` — custom unit for
  Stereo Tool; `After=liquidsoap.service`, `After=pipewire-system.service`;
  `User=rd`; `Environment=XDG_RUNTIME_DIR=/run/pipewire-system`;
  `-p 8079` on `ExecStart` to expose its headless web config UI.
- New: `/etc/sudoers.d/rivapi` — targeted rule: `rd` may run
  `systemctl start/stop/restart` for `rivolution-stack.target`,
  `rivendell.service`, `icecast2.service`, `liquidsoap.service`,
  `stereo-tool.service`, `tailscaled.service`, `pipewire-system.service`,
  `wireplumber-system.service` with `NOPASSWD`, plus `RIVAPI_INSTALL`
  (`install -o root -g icecast -m 640 <staging> /etc/icecast2/icecast.xml`)
  and `RIVAPI_SYSTEMCTL` (rebuild/restart `rivapi.service` itself, for
  `scripts/rivapi-rebuild.sh`).
- New: `/etc/udev/rules.d/99-ptp.rules` — assigns `/dev/ptpN` to `ptp`
  group (prerequisite for Phase 1.5 PTP clock access as `rd`).
- New: `/etc/ld.so.conf.d/00-pipewire-jack-<multiarch-triplet>.conf` —
  see deviations below; load-bearing for JACK clients to actually reach
  PipeWire's JACK bridge instead of a real `libjack-jackd2-0`, if present.
- New: `rivapi/store/patchbay.go`, `dashboard/handlers_patchbay.go`,
  `dashboard/templates/patchbay.html` — `/patchbay`, the visual
  PipeWire/WirePlumber connect/disconnect UI backed by `pw-link`, with
  reconciler-based persistence (`SaveDesiredLinks`/`LoadDesiredLinks`/
  `ReconcileLinks` + a 5s background poll loop). Not part of the
  original Phase 1 plan — added once WirePlumber's own declarative
  routing policy was confirmed not to apply to JACK-bridged ports (see
  deviations below). This is the actual persistent-routing mechanism;
  no static ALSA/WirePlumber config file is deployed.
- Modified: `rivapi/` — `store/service_status.go` (unit state polling,
  controlled action execution, `Detail`/`Warn` fields for sub-state and
  JACK-health surfacing); `dashboard/handlers.go` + templates
  (`/system` route: per-service status indicators, start/stop/restart
  buttons, category-3 confirmation dialogs).

### Phase 1.5 (caed split — immediately after Phase 1 verified)

- New: `/etc/systemd/system/caed.service` — standalone unit: `User=rd`,
  `Group=rd`, `LimitRTPRIO=99`, `LimitRTTIME=infinity`,
  `IOSchedulingClass=realtime`, `IOSchedulingPriority=0`,
  `After=wireplumber-system.service`, `Requires=wireplumber-system.service`.
  Includes `ExecStartPost=` health probe on port 5005 until native
  `sd_notify` lands.
- Modified: `rdservice` (`rdservice/startup.cpp`) — skip spawning `caed`
  when `caed.service` is present and enabled; detect this at runtime or
  via a compile-time option.
- Modified: `rivendell.service.d/rivolution.conf` — remove the caed
  readiness probe (now handled by `caed.service` itself); update
  `After=` to reflect the new chain.
- Modified: `cae/cae.cpp` — add `sd_notify(0, "READY=1")` at the point
  `caed` has connected to PipeWire and is accepting protocol connections.
- New: `pipewire-system.service` / `wireplumber-system.service` drop-ins
  (if readiness probes are needed after verification).

## Deployment

The conf files in this spec (`conf/sudoers.d/`, `conf/systemd/`,
`conf/udev/`) are the source of truth in this repository. They are not
auto-deployed; they must be installed by one of the following paths.

### Manual installation (development)

Copy each file to its system destination and apply post-install steps:

```
sudo cp conf/sudoers.d/rivapi /etc/sudoers.d/rivapi
```

```
sudo chmod 440 /etc/sudoers.d/rivapi
```

```
sudo cp conf/systemd/rivolution-stack.target /etc/systemd/system/
```

```
sudo cp conf/systemd/rivendell.service.d/rivolution.conf /etc/systemd/system/rivendell.service.d/
```

> **Prerequisite:** the `rivendell.service.d/rivolution.conf` drop-in
> sets `User=rd`. This will not work until `rdservice` is rebuilt with
> the `geteuid()!=0` check removed from
> `rdservice/rdservice.cpp:89–92`. Installing the drop-in against an
> unmodified binary causes `rdservice` to exit immediately with
> "this service requires root" and enter an infinite restart loop
> (`Restart=always` + a 30-second `ExecStartPost` probe = ~32s per
> cycle). Do not install this drop-in until the rebuilt binary is
> confirmed deployed.

```
sudo cp conf/systemd/icecast2.service.d/rivolution.conf /etc/systemd/system/icecast2.service.d/
```

```
sudo cp conf/systemd/pipewire-system.service /etc/systemd/system/
```

```
sudo cp conf/systemd/wireplumber-system.service /etc/systemd/system/
```

```
sudo cp conf/systemd/liquidsoap.service /etc/systemd/system/
```

```
sudo cp conf/systemd/liquidsoap.service.d/rivolution.conf /etc/systemd/system/liquidsoap.service.d/
```

```
sudo cp conf/systemd/stereo-tool.service /etc/systemd/system/
```

```
sudo cp conf/systemd/rivapi.service /etc/systemd/system/
```

```
sudo cp conf/udev/99-ptp.rules /etc/udev/rules.d/
```

> **Load-bearing step, easy to skip silently:** rename the
> `pipewire-jack` ld.so.conf.d entry so it sorts before the standard
> multiarch conf file — otherwise a real `libjack-jackd2-0`, if
> present, always wins SONAME resolution and PipeWire audio silently
> never connects despite every service reporting `active`. See
> "Implementation deviations" above for the full explanation.

```
sudo mv /etc/ld.so.conf.d/pipewire-jack-$(dpkg-architecture -qDEB_HOST_MULTIARCH).conf /etc/ld.so.conf.d/00-pipewire-jack-$(dpkg-architecture -qDEB_HOST_MULTIARCH).conf
```

```
sudo ldconfig
```

```
sudo systemctl daemon-reload
```

```
sudo udevadm control --reload-rules && sudo udevadm trigger
```

```
sudo rddbmgr --modify
```

```
sudo systemctl enable --now pipewire-system.service wireplumber-system.service
```

```
sudo systemctl enable --now rivapi.service
```

```
sudo systemctl enable rivolution-stack.target
```

After first boot, open the dashboard's `/patchbay` page once to connect
and save the `caed` → Stereo Tool → Liquidsoap chain — the only
remaining manual step, and it's a browser action, not a terminal
command (see "Implementation deviations" above).

### Unified installer (`anjeleno/rivolution-unified-installer`)

**This subsection is stale and needs its own pass, not done here.** The
`broadcast_advanced` Ansible role this originally assumed would own
conf-file deployment was removed entirely 2026-07-01 — its
Icecast/Liquidsoap/VLC config generation moved into `rivapi` itself
(specs 0007/0008), and its seed database was hardcoded to a single
host name, not general-purpose (`BACKLOG.md` has the full removal
writeup). Whether the unified installer still needs its own role for
package installation + conf placement, or should simply install the
Debian package below once it exists, is an open question — tracked in
`BACKLOG.md`, not decided here.

### Debian package

The long-term deployment path is a `rivolution` deb package whose
`postinst` performs everything the "Manual installation" steps above do
by hand, so a `.deb` install ends up with the identical, fully verified
working state — no separate broadcast/runtime package, no manual
post-install checklist beyond the one `/patchbay` browser action.

**Current state (2026-07-02):** `debian/control`, `debian/rules`, and
`debian/postinst` all predate specs 0007/0008/0010/0012 entirely —
none of them reference `icecast2`, `liquidsoap`, `pipewire`, `fdkaac`,
`vlc`, or `rivapi` in any form, and `postinst` still provisions the old
pre-fork `rivendell` system user (uid 150) rather than the `rd`-user
model this spec and 0007 are built around. A separate, narrower effort
(getting `dpkg-buildpackage` to produce a working *core* `.deb` at all
— unrelated packaging bugs, not a design gap) is in progress on
`debian/rivolution-branding-fix` as of this writing. This section
describes the follow-up work once that lands.

**Package boundary:** fold into the existing `rivolution` package
rather than a new `rivolution-broadcast` package. `rivolution`'s own
`Description` already promises "a complete radio broadcast automation
solution" — the broadcast/PipeWire/dashboard layer is core to that
promise now, not an optional add-on, and a single package means a
single `postinst` to reason about.

**New `debian/control.src` dependencies** — add to `rivolution`'s
`Depends`: `icecast2`, `liquidsoap`, `fdkaac`, `vlc`, `vlc-plugin-jack`,
`pipewire`, `wireplumber`, `pipewire-jack | libjack-jackd2-0`. Add
`golang-go` to `Build-Depends` so `rivapi` compiles as part of the
package build instead of via the manual `scripts/rivapi-rebuild.sh`
workflow.

**New `debian/rules` build step:** `go build -o rivapi/rivapi
rivapi/...` (or a `cd rivapi && go build` recipe line) alongside the
existing autotools `build:` recipe, installing the resulting binary to
`/usr/local/bin/rivapi` per `rivapi.service`'s `ExecStart=`.

**`postinst` additions** (idempotent, matching the existing
`test ! -e X` style already used for `/etc/rd.conf`/`/var/snd`/etc.):

1. Copy every unit/drop-in listed under "Files" above
   (`rivolution-stack.target`, `rivendell.service.d/rivolution.conf`,
   `icecast2.service.d/rivolution.conf`, `pipewire-system.service`,
   `wireplumber-system.service`, `liquidsoap.service` +
   `liquidsoap.service.d/rivolution.conf`, `stereo-tool.service`,
   `rivapi.service`) to `/etc/systemd/system/`.
2. Install `conf/sudoers.d/rivapi` to `/etc/sudoers.d/rivapi`,
   `chmod 440`.
3. Install `conf/udev/99-ptp.rules` to `/etc/udev/rules.d/`, then
   `udevadm control --reload-rules && udevadm trigger`.
4. Apply the `ld.so.conf.d` rename fix (see "Implementation
   deviations" above) + `ldconfig`. Must be idempotent — check the
   target filename doesn't already exist before renaming, since a
   `.deb` upgrade re-runs `postinst` on an already-fixed system.
5. `daemon-reload`, then `enable --now` `pipewire-system.service` and
   `wireplumber-system.service` *before* `rivendell.service` is
   (re)started — `caed`'s JACK driver needs the socket present at
   startup, not just eventually.
6. Run `rddbmgr --modify` for the schema bump (378 → 379, see
   "Implementation deviations" above). **Upgrade ordering matters**:
   on a fresh install this runs before anything using the schema
   starts, safe by construction. On an upgrade of an already-running
   system, `postinst` must run the migration *before* restarting any
   Rivendell binary that was just replaced with the new
   `RD_VERSION_DATABASE` — restarting first would hard-block on the
   version mismatch check (`rdcoreapplication.cpp:240`). The existing
   `postinst`'s final `systemctl restart rivendell` call must move
   after this step, not before.
7. `enable --now rivapi.service`, then `enable rivolution-stack.target`
   (matching "Manual installation" above — the target itself is
   enabled, not started immediately, consistent with existing
   `postinst` behavior for `rivendell.service`).
8. **Not automated:** `/patchbay` connection setup. `postinst` cannot
   meaningfully click through a browser UI, and it's the one step this
   spec's own goal (deviations above) treats as acceptable to leave to
   the operator — document it in the package's `NEWS`/post-install
   message (`debian/rivolution.postinst`'s final echo, or a
   `debconf` note), not silently skipped.

**Not in scope for `postinst` at all:** `conf/alsa/rd.asoundrc` —
superseded by `/patchbay`'s reconciler (see "Implementation
deviations" above); nothing to deploy.

## Verification

### Phase 1

1. After applying the `rivendell.service` drop-in and running
   `systemctl daemon-reload && systemctl restart rivendell.service`:
   confirm `rivendell.service` shows `active (running)` and `ps aux`
   shows `caed`, `ripcd`, `rdcatchd` running as `rd`, not root.
2. Dashboard `/system` page: all managed units display their current
   state; start/stop/restart buttons work; category-3 actions show a
   confirmation dialog before proceeding.
3. `systemctl start rivolution-stack.target` brings up all installed
   stack services in order without manual intervention.
4. Icecast-only config change restarts only `icecast2.service` — audio
   path unaffected.

### Phase 1.5

1. Cold boot: every service in `rivolution-stack.target` reaches
   `active` in correct order with no manual intervention.
2. `systemd-analyze critical-chain rivolution-stack.target` confirms
   each service's activation time follows its predecessor's ready
   signal, not just its start time.
3. Reboot: a patch saved via `/patchbay` before reboot is silently
   reconnected by the reconciler poll loop after a cold reboot with no
   operator action (see "Implementation deviations" — this replaced the
   originally planned WirePlumber declarative routing policy, which
   doesn't apply to JACK-bridged ports).
4. `caed` restart isolation: restarting `caed.service` does not
   automatically restart RDAirPlay; dashboard surfaces RDAirPlay's
   degraded state.

## Implementation deviations

- **2026-07-01:** Spec originally described `caed.service` as a
  standalone unit from the start. Revised to a two-phase approach after
  confirming `caed` is managed as a child process of `rdservice`
  inside `rivendell.service`. Phase 1 uses a `rivendell.service` drop-in
  to prove the `User=rd` permission model; Phase 1.5 does the caed split
  immediately after.
- **2026-07-01:** VLC removed from the systemd stack. Its audio routing
  (VLC output → Rivendell input) is handled by WirePlumber persistent
  policy, not a service unit.
- **2026-07-01:** Stereo Tool and Tailscale added to the stack unit list.
  Stereo Tool requires a custom unit file (`stereo-tool.service`);
  Tailscale's upstream unit (`tailscaled.service`) needs only a `Wants=`
  entry in the target.
- **2026-07-01:** Phase 1 `User=rd` drop-in **requires a C++ change
  before deployment.** `rdservice/rdservice.cpp:89–92` contains a hard
  `geteuid()!=0` check that logs "this service requires root" and exits
  with `ExitNoPerms` (status 11) immediately, before any initialization.
  With the drop-in installed and `Restart=always` + `RestartSec=2` in
  the base unit, the 30-second `ExecStartPost=` caed probe creates a
  ~32-second restart cycle (service exits → probe runs its 30s timeout
  → restart fires → repeat). The fix is to remove lines 89–92 from
  `rdservice/rdservice.cpp` and rebuild `rdservice` before deploying the
  drop-in. The drop-in source at
  `conf/systemd/rivendell.service.d/rivolution.conf` is the correct
  target state; it must not be copied to `/etc/systemd/system/` until
  the rebuilt binary is in place. Tailscale intentionally excluded from
  `PartOf=rivolution-stack.target` — stopping the broadcast stack must
  not kill the VPN.
- **2026-07-01:** `rivapi.service` implemented without an
  `EnvironmentFile=` — unnecessary. `rivapi/config/config.go` already
  reads DB credentials and the dashboard JWT secret from `/etc/rd.conf`
  (the standard Rivendell config file, same as every other Rivendell
  binary), and `scripts/rivolution-first-run.sh` already populates
  `[dashboard] JwtSecret` there automatically. `RIVAPI_*` env vars still
  override individual values if ever needed, per the existing
  `config.Load()` precedence. Also added `scripts/rivapi-rebuild.sh`
  (build + install + restart in one command) and a matching
  `RIVAPI_SYSTEMCTL`/`RIVAPI_INSTALL` sudoers addition, replacing the
  manual `cd rivapi && go build -o rivapi . && ./rivapi` dev workflow
  noted in `BACKLOG.md`.
- **2026-07-01 (later the same day):** `pipewire-system.service` and
  `wireplumber-system.service` turned out to be a **Phase 1** dependency,
  not Phase 1.5 as the dependency-chain diagram above shows. `caed`
  reaches PipeWire via its existing JACK driver (through the
  `pipewire-jack` bridge package) even before the Phase 1.5 native
  `pw_stream` rewrite — that JACK driver needs a running system-scope
  PipeWire socket the moment `rivendell.service` starts with
  `User=rd`. Both units, plus `Environment=XDG_RUNTIME_DIR=/run/pipewire-system`
  on `rivendell.service.d/rivolution.conf`, `liquidsoap.service`, and
  `stereo-tool.service`, were required to get real audio flowing in
  Phase 1, and are now part of Phase 1's own `Wants=` chain in
  `rivolution-stack.target`.
- **2026-07-01 (later the same day):** Ubuntu 26.04's `pipewire` package
  ships **only user-scope** systemd units
  (`/usr/lib/systemd/user/pipewire.service`) — the spec's original
  assumption of a `pipewire-system.service` "available alongside the
  default per-user unit" was wrong; no such unit exists upstream. Both
  `pipewire-system.service` and `wireplumber-system.service` were
  written from scratch as custom units for this project, not adapted
  from an existing system-scope unit.
- **2026-07-01 (later the same day):** Ubuntu's `liquidsoap` package
  ships **no systemd unit at all**. `conf/systemd/liquidsoap.service`
  had to be authored as a full standalone unit (`ExecStart`,
  `Restart=on-failure`, `RestartSec=5`), not the drop-in this spec
  originally planned; `liquidsoap.service.d/rivolution.conf` now only
  carries the `[Unit]` `After=`/ordering stanzas.
- **2026-07-01 (later the same day):** A real, silent JACK-connection
  failure was traced to `ldconfig`'s alphabetic processing of
  `/etc/ld.so.conf.d/*.conf`: the standard multiarch conf
  (`aarch64-linux-gnu.conf`) sorts before
  `pipewire-jack-aarch64-linux-gnu.conf`, so a pre-existing real
  `libjack-jackd2-0` (pulled in as a dependency of something else, e.g.
  Liquidsoap) always won the dynamic linker's SONAME lookup — the
  `pipewire-jack` shim was never actually loaded by anything, and both
  `caed` and Liquidsoap logged "unable to communicate with JACK server"
  even with PipeWire confirmed up and `XDG_RUNTIME_DIR` set correctly.
  Fixed by renaming the conf file so it sorts first:
  `pipewire-jack-<triplet>.conf` → `00-pipewire-jack-<triplet>.conf`,
  then `ldconfig`. This ordering fix must ship as part of any packaged
  install — without it, PipeWire audio silently doesn't work despite
  every service reporting `active`.
- **2026-07-01 (later the same day):** Deviation entry above ("VLC
  routing... handled by WirePlumber persistent policy") turned out to
  be wrong once actually implemented. WirePlumber's declarative
  `target.node` metadata mechanism does not apply to JACK-bridged ports
  — verified empirically three independent ways (no link forms, an
  internal WirePlumber script throws a Lua exception on these nodes,
  and the metadata itself is keyed by an ephemeral `node.id` that
  changes on every restart anyway). Replaced by `/patchbay`
  (`rivapi/store/patchbay.go`'s `SaveDesiredLinks`/`LoadDesiredLinks`/
  `ReconcileLinks` plus a 5-second background poll loop in `main.go`
  that re-applies any saved link missing from the live graph) — a
  reconciler running inside `rivapi` itself, not a WirePlumber policy
  file. Verified surviving a real Liquidsoap restart with the saved
  patch silently reconnecting. This is also the actual, complete answer
  to Stereo Tool's routing (`rivendell_0:playout_0L/R` → Stereo Tool →
  `liquidsoap:in_0/1`): no static `conf/alsa/rd.asoundrc` needs
  deploying by the package at all. The one remaining manual step after
  install is a one-time UI action — open `/patchbay`, connect the
  chain, click "Save current patch" — not a terminal command, so it
  stays consistent with this spec's "no operator should ever need to
  touch a systemd unit file or restart a service from a terminal" goal.
- **2026-07-01 (later the same day):** `STATIONS.JACK_VERSION`
  (`char(16)`) was too narrow for the PipeWire JACK shim's
  `jack_get_version_string()` output (`"v3.1.6.2 (using PipeWire
  1.6.2)"`, 31 characters) — `caed` logged a non-fatal "Data too long
  for column" error on every connect. Fixed with a schema migration,
  `RD_VERSION_DATABASE` 378 → 379 (`lib/dbversion.h`,
  `utils/rddbmgr/{updateschema,revertschema}.cpp`), widening the column
  to `varchar(64)`. Any install/upgrade path (manual, unified installer,
  or the Debian package below) must run `rddbmgr --modify` as part of
  provisioning — safe to run without disrupting an already-running
  stack, but required before the *next* restart of any Rivendell binary
  once the new schema version is compiled in, since a schema/binary
  version mismatch hard-blocks startup
  (`rdcoreapplication.cpp:240`).
