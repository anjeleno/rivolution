# 0010 — Systemd stack orchestration

**Date:** 2026-06-30

## Goal

Define how the full Rivolution broadcast stack — PipeWire, WirePlumber,
`caed`, Icecast, Liquidsoap — is orchestrated as a reliable, correctly
ordered set of systemd units with guaranteed startup sequencing and
live-playout-safe reconfiguration, managed exclusively through the Go
dashboard (`0005-go-api-foundation.md`). No operator should ever need
to touch a systemd unit file or restart a service from a terminal.

## Background

### Current state

External tools (Icecast, Liquidsoap, JACK patches) are launched via
desktop shortcuts and scripts in a specific, manually maintained order.
Race conditions occur when services start before their dependencies are
ready to accept connections — a timing failure, not an ordering failure,
and not fixable by boot-time scripts. No mechanism exists to apply
configuration changes without potentially interrupting live audio.

The unified installer (`anjeleno/rivolution-unified-installer`) lays
down all packages and configuration files but provides no orchestration:
every installed service starts independently, with no inter-service
dependency knowledge. The result is a working set of parts that don't
reliably communicate at startup or after a reboot.

### `caed` permissions: prerequisite for PipeWire integration

`caed` currently runs as root — a legacy practice from early Rivendell
that is incompatible with PipeWire, whose session daemon runs under the
`rd` user account. A root-owned process cannot reach a user-session
PipeWire socket by default (`PIPEWIRE_RUNTIME_DIR` is not visible to
root). The decision to run `caed` as the `rd` user is already locked in
`0007-pipewire-audio-engine.md`; implementing it is a prerequisite for
the PipeWire portions of this spec and must land in the same PR that
adds the `caed.service` drop-in override. The required changes are
already listed in the [Files](#files) section below. Services that depend
on the JACK/PipeWire path (Liquidsoap, persistent patch connections)
also benefit from this change since they currently work by coincidence
when JACK runs under the same root environment as `caed`.

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

The correct fix is **readiness signaling**: `caed` and PipeWire must
signal systemd explicitly when they are fully initialized and accepting
clients, so that no dependent service is started until that signal
arrives. This requires `Type=notify` in the service unit (the service
calls `sd_notify(0, "READY=1")` at the point it is actually ready) or,
where modifying the daemon for `sd_notify` isn't yet practical, a
`ExecStartPost=` health-probe loop that polls until the service's socket
or API endpoint responds.

## Design

### Service ordering and `rivolution-stack.target`

A custom systemd target, `rivolution-stack.target`, groups the full
broadcast stack and is the unit the dashboard starts, stops, and
monitors.

**The target file is stable infrastructure — it is never regenerated
or rebuilt when operator configuration changes.** It declares `Wants=`
for every service the stack may run (Icecast, Liquidsoap, Stereo Tool,
PipeWire, WirePlumber, `caed`). Whether a given service actually runs
is controlled entirely by `systemctl enable` / `systemctl disable` (or
`systemctl mask` for hard prevention). The dashboard calls enable/
disable + start/stop for immediate effect; it never rewrites the target
file itself. The one exception is when a new custom unit file must be
*created* (e.g., a new per-station AES67 bridge unit) — in that case
the dashboard writes the new unit file and calls `systemctl daemon-reload`
transparently. This model is safe for live stations: a misconfigured
unit that fails to start does not bring down other units in the target.

Service dependencies within the target:

```
pipewire-system.service
  └── wireplumber-system.service (After=, Requires=)
        └── caed.service (After=, Requires=)
              ├── icecast2.service (After=)
              └── rivendell-airplay.service (After=, if managed)
                    └── liquidsoap.service (After=)
```

Icecast must be running before Liquidsoap because Liquidsoap pushes the
encoded stream to Icecast's source port — Liquidsoap's own startup fails
or produces connection errors if Icecast isn't already accepting
connections. `caed` must be ready (not just started) before Liquidsoap
because Liquidsoap reads audio from Rivendell's PipeWire ports.

### Readiness signaling

Every service in the chain that has a downstream dependent must use
`Type=notify` or an equivalent readiness mechanism:

- **`pipewire-system.service`** — the upstream PipeWire package already
  ships this unit; verify it uses `Type=notify` on Ubuntu 26.04. If not,
  a `ExecStartPost` probe polling the PipeWire socket until it accepts
  a connection is the fallback.
- **`wireplumber-system.service`** — same: verify or add a health probe.
- **`caed.service`** — `caed` does not currently implement `sd_notify`.
  Until it does, `ExecStartPost` with a loop that polls `rdcae` or the
  `caed` TCP port until it responds is the required approach. Adding
  native `sd_notify` support to `caed` is the correct long-term fix and
  should happen as part of this spec's implementation, not deferred.
- **`icecast2.service`** — Icecast's own unit should be verified;
  a simple `ExecStartPost` HTTP probe against the status endpoint
  (`http://localhost:8000/status.xsl`) is sufficient if native notify
  isn't available.

### Live-playout protection

**The dashboard never restarts `caed`, PipeWire, or RDAirPlay as an
implicit side effect of writing configuration.** This is a first-class
design principle, not a preference. Every config change the dashboard
makes falls into exactly one of three categories:

**1. Zero-disruption — applies immediately, no restart of any audio
service:**
- WirePlumber routing policy changes: `wpctl` command plus a policy
  file update. Applies live to the running graph.
- Icecast mount/metadata/password changes: restart `icecast2.service`
  only. The audio path (caed → PipeWire → Liquidsoap → Icecast) is
  unaffected; Liquidsoap reconnects automatically after Icecast restarts.
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
- `caed` restart in isolation (interrupts all audio).
- Any restart that touches the live playout path.

The dashboard's confirmation dialog for category 3 must state plainly
that live audio will be interrupted and must offer the option to cancel.

### RDAirPlay restart behavior

Restarting `caed` does not implicitly restart `rivendell-airplay.service`
or any other Rivendell GUI application. If a full stack restart is
requested, the dashboard offers an explicit toggle: "also restart
RDAirPlay" (default: off). When RDAirPlay is excluded, a `caed` restart
will leave RDAirPlay in a degraded state (lost daemon connection) until
RDAirPlay itself is restarted separately — the dashboard must surface
this state visibly rather than silently.

### WirePlumber as a system service

WirePlumber normally starts as part of a desktop user's session
(`systemctl --user`). For this design it must run at system scope:

- Unit: `wireplumber-system.service` (available in the `wireplumber`
  package alongside the per-user unit).
- Config: `/etc/wireplumber/` (system-scope policy files), not
  `~/.config/wireplumber/` (which is session-scoped and unavailable
  before login).
- The dashboard generates and manages files in `/etc/wireplumber/` when
  saving routing policy. Operators never edit these files directly.

### Dashboard as sole interface

The Go API (`rivapi/`) manages systemd state through:

- **Reading:** `systemctl is-active <unit>` / `systemctl status <unit>`
  polled at a reasonable interval, surfaced as service health indicators
  in the dashboard.
- **Writing configuration:** generates or updates the appropriate config
  file for each service (WirePlumber policy, Icecast XML, Liquidsoap
  `.liq`), then applies the minimal restart the change category requires.
- **Applying systemd changes:** writes drop-in overrides to
  `/etc/systemd/system/<unit>.d/override.conf` for unit-level changes,
  then calls `systemctl daemon-reload` followed by the appropriate restart.
  Never edits package-owned base unit files directly.
- **Never:** exposes a raw "restart service" button for audio-path
  services without the warning and confirmation described above.

The dashboard requires the `rivapi` process to run with sufficient
privilege to call `systemctl` for the relevant units — either via a
targeted sudoers rule (`rivapi` user can restart specific units without
password) or via systemd's own D-Bus policy granting the `rivapi`
service unit control over named units. Raw `sudo` with unrestricted
access is not acceptable; the privilege must be scoped to exactly the
units Rivolution manages.

## Files

- New: `/etc/systemd/system/rivapi.service` — systemd unit for the Go
  dashboard process itself. Runs as `rd`, starts after `network.target`
  and `mariadb.service`, reads credentials from an `EnvironmentFile=`
  (path TBD at installation time). `WantedBy=multi-user.target`. The
  binary path is wherever `make install` places it (TBD; likely
  `/usr/local/bin/rivapi` or `/usr/bin/rivapi`). During development,
  started manually: `cd ~/dev/rivolution/rivapi && go build -o rivapi . && ./rivapi`.
- New: `/etc/systemd/system/rivolution-stack.target`
- New: `/etc/udev/rules.d/99-ptp.rules` (assigns `/dev/ptpN` to
  `ptp` group)
- New or modified: `caed.service` drop-in override — changes `User=`,
  `Group=`, `LimitRTPRIO=`, `LimitRTTIME=`, `IOSchedulingClass=`,
  `IOSchedulingPriority=`, adds `After=wireplumber-system.service`,
  `Requires=wireplumber-system.service`, and an `ExecStartPost` health
  probe until native `sd_notify` support is added to `caed`.
- New: `icecast2.service` drop-in adding `After=caed.service`.
- New: `liquidsoap.service` drop-in adding `After=icecast2.service`
  and `After=caed.service`.
- Modified: `rivapi/` — systemd/WirePlumber management code, service
  health polling, config file generation per tool, dashboard API
  endpoints for service state and config.
- Modified: `cae/cae.cpp` — add `sd_notify(0, "READY=1")` at the point
  `caed` has successfully connected to PipeWire and is accepting `caed`
  protocol connections from clients. Required to retire the `ExecStartPost`
  health-probe workaround.

## Verification

1. Cold boot on a stock ARM64 Ubuntu 26.04 install with no manual
   configuration: every service in `rivolution-stack.target` reaches
   `active` in correct order with no manual intervention.
2. Timing confirmation: use `systemd-analyze critical-chain
   rivolution-stack.target` to confirm each service's activation time
   follows its predecessor's ready signal, not just its start time.
3. Live audio playing through RDAirPlay: confirm an Icecast-only config
   change (mount rename, password change) restarts only `icecast2.service`
   and does not interrupt audio.
4. Reboot: confirm WirePlumber routing policy written via the dashboard
   is present and applied after a cold reboot with no operator action.
5. RDAirPlay restart isolation: confirm a `caed` restart does not
   automatically restart RDAirPlay, and the dashboard correctly surfaces
   RDAirPlay's degraded state.

## Implementation deviations

None yet — implementation has not started.
