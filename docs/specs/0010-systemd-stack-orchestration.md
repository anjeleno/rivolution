# 0010 — Systemd stack orchestration

**Date:** 2026-06-30
**Revised:** 2026-07-01

## Goal

Define how the full Rivolution broadcast stack — Rivendell (including
`caed`), Icecast, Liquidsoap, Stereo Tool, and Tailscale — is
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
It is not a background service and has no place in the systemd stack.
Its audio path — VLC output → Rivendell input — is established by
WirePlumber persistent routing policy (spec 0007/0008 territory).
When VLC is launched, WirePlumber automatically reconnects its audio
port to the correct Rivendell input. No unit file required.

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
  dashboard process itself. Runs as `rd`, starts after `network.target`
  and `mariadb.service`, reads credentials from an `EnvironmentFile=`.
  `WantedBy=multi-user.target`. Binary path TBD at install time (likely
  `/usr/local/bin/rivapi`). During development, started manually:
  `cd ~/dev/rivolution/rivapi && go build -o rivapi . && ./rivapi`.
- New: `/etc/systemd/system/rivolution-stack.target` — groups the full
  broadcast stack; `Wants=` for `rivendell.service`, `icecast2.service`,
  `liquidsoap.service`, `stereo-tool.service`, `tailscaled.service`.
- New: `/etc/systemd/system/rivendell.service.d/rivolution.conf` —
  drop-in setting `User=rd`, `Group=rd`, `LimitRTPRIO=99`,
  `LimitRTTIME=infinity`, `IOSchedulingClass=realtime`,
  `IOSchedulingPriority=0`, plus an `ExecStartPost=` health probe
  polling `caed`'s TCP port (5005) for readiness.
- New: `/etc/systemd/system/icecast2.service.d/rivolution.conf` —
  drop-in adding `After=rivendell.service`.
- New: `/etc/systemd/system/liquidsoap.service.d/rivolution.conf` —
  drop-in adding `After=icecast2.service` and `After=rivendell.service`.
- New: `/etc/systemd/system/stereo-tool.service` — custom unit for
  Stereo Tool; `After=liquidsoap.service`; `User=rd`.
- New: `/etc/sudoers.d/rivapi` — targeted rule: `rd` may run
  `systemctl start/stop/restart` for `rivolution-stack.target`,
  `rivendell.service`, `icecast2.service`, `liquidsoap.service`,
  `stereo-tool.service`, `tailscaled.service` with `NOPASSWD`.
- New: `/etc/udev/rules.d/99-ptp.rules` — assigns `/dev/ptpN` to `ptp`
  group (prerequisite for Phase 1.5 PTP clock access as `rd`).
- Modified: `rivapi/` — `store/service_status.go` (unit state polling,
  controlled action execution); `dashboard/handlers.go` + templates
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

```
sudo cp conf/systemd/icecast2.service.d/rivolution.conf /etc/systemd/system/icecast2.service.d/
```

```
sudo cp conf/systemd/liquidsoap.service.d/rivolution.conf /etc/systemd/system/liquidsoap.service.d/
```

```
sudo cp conf/systemd/stereo-tool.service /etc/systemd/system/
```

```
sudo cp conf/udev/99-ptp.rules /etc/udev/rules.d/
```

```
sudo systemctl daemon-reload
```

```
sudo udevadm control --reload-rules && sudo udevadm trigger
```

```
sudo systemctl enable rivolution-stack.target
```

### Unified installer (`anjeleno/rivolution-unified-installer`)

A new Ansible role (or tasks in an existing role) must copy all files
above, set correct permissions, run `daemon-reload` and `udevadm trigger`,
and enable the target and services. This role must also build and install
the `rivapi` binary and install `rivapi.service`. See
[BACKLOG.md](https://github.com/anjeleno/rivolution/blob/main/BACKLOG.md)
for the full task list.

### Debian package

The long-term deployment path is a `rivolution` deb package that installs
all conf files via standard package `postinst` hooks. The package must
handle: sudoers rule install + `chmod 440`, systemd unit copies +
`daemon-reload` + `systemctl enable`, udev rule + `udevadm trigger`, and
the `rivapi` binary at a standard system path. This is a separate
packaging effort not yet started.

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
3. Reboot: WirePlumber routing policy written via the dashboard is
   present and applied after a cold reboot with no operator action.
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
