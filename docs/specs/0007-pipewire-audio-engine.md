# 0007 — PipeWire audio engine: native AES67 and unified cross-driver routing

**Date:** 2026-06-21

## Goal

Add native PipeWire support to `caed`, specifically to deliver: (1)
native AES67 audio-over-IP support, and (2) real-time, any-to-any audio
routing across every backend `caed` supports (ALSA, JACK, AudioScience
HPI, and the new PipeWire/AES67 path), with no exclusivity between
them. This touches `caed`, the highest-risk code in this project
(per [`BACKLOG.md`](https://github.com/anjeleno/rivolution/blob/main/BACKLOG.md)), and is scoped accordingly.

## Background

### Current driver architecture

`caed` initializes the ALSA, JACK, and AudioScience HPI drivers
unconditionally at startup (`cae/cae.cpp:224-227`):

```cpp
MakeDriver(&next_card,RDStation::Hpi);
MakeDriver(&next_card,RDStation::Alsa);
MakeDriver(&next_card,RDStation::Jack);
```

Each card independently declares its driver type
(`RDStation::cardDriver()`, `lib/rdstation.h:103`), and per-card
operations dispatch to the owning driver via `GetDriver(cardnum)`
(`cae/cae.cpp:1627-1635`).

**Cross-driver routing does not exist today.** Every audio route is
driver-private: a JACK-backed card's ports can only be patched to other
ports on that same JACK driver instance (`cae/driver_jack.cpp:115-126`
only connects JACK-to-JACK). `GetDriver()`'s dispatch never bridges
across driver instances, and `lib/rdmatrix.cpp` handles GPIO/control
switching, not audio signal routing. This is why, operationally, a
Rivendell installation behaves as if it is locked to one driver at a
time even though multiple driver instances can technically run in the
same process — there is no missing configuration option, there is a
missing cross-driver bridge.

There is no existing AES67 implementation anywhere in this codebase.

See `ARCHITECTURE.md`'s "`caed` audio driver layer" section for a
deeper trace of the per-driver card/port model (ALSA auto-probes every
system device; JACK registers exactly one card per process) and the
exact existing routing mechanism (`setPassthroughLevel()`'s per-card
gain matrix, plus JACK's startup-only `JackSessionSetup()` patch list)
— useful background for the cross-driver routing decision below.

### Why PipeWire

PipeWire's core architecture unifies ALSA, JACK-compatible clients,
Bluetooth, and RTP-based sources/sinks into a single audio graph with
any-to-any patchable routing between them — this directly addresses
the cross-driver routing gap above, not just an AES67 feature gap.

PipeWire 1.1 and later includes native AES67 support: dedicated
`module-rtp-sink`, `module-rtp-source`, and `module-rtp-sap` modules,
PTP clock synchronization via `linuxptp`, and SAP-based stream
discovery — the same discovery mechanism used by Dante, giving a real
basis for interoperability with existing AES67/Dante hardware on a
network.

Confirmed against real package archives: Ubuntu 24.04 (`noble`) ships
PipeWire 1.0.5, below the AES67 support threshold. **Ubuntu 26.04
(`resolute`) ships PipeWire 1.6.2**, comfortably past it — this is the
same OS version this project is already targeting for the Qt6
migration (`0006-qt6-migration.md`).

PipeWire's AES67 path also depends on `linuxptp` 4.0 or later (the
version that added the state-query API PipeWire reads clock state
through). Confirmed against the same archive: Ubuntu 26.04 ships
`linuxptp` 4.4-0ubuntu1, also past that floor — no separate PPA or
backport needed for either dependency.

`pipewire-jack`, PipeWire's JACK-API compatibility layer, would let an
existing JACK client work against a PipeWire server with no source
changes. This spec does not rely on that shim for the new AES67
driver — native support is wanted specifically — but it remains a
relevant fact for any future evaluation of running existing JACK-based
tooling (e.g. `qjackctl`-adjacent workflows) against a PipeWire server
during a transition period. Note: the `pipewire-jack` path is gated on
`caed` first migrating off root to the `rd` user (see
[`0010-systemd-stack-orchestration.md`](0010-systemd-stack-orchestration.md),
"caed permissions: prerequisite for PipeWire integration") — a
root-owned process cannot reach the `rd`-user PipeWire session socket
regardless of whether `pipewire-jack` is installed.

### Vendor interoperability

AES67 is an interoperability layer, not a single vendor's protocol —
compatibility with any given AoIP ecosystem holds only at the boundary
where that vendor's gear is actually operating in genuine AES67 mode,
not against that vendor's full proprietary feature set:

- **RAVENNA** — clean at the audio/clock layer: RAVENNA was one of the
  primary technical contributors to the AES67 standard itself, so
  RAVENNA devices speak genuine AES67 natively, with no mode switch or
  gateway required for the actual stream. **Discovery is a separate
  caveat, not covered by that claim:** RAVENNA's native discovery
  mechanism is zeroconf, while PipeWire's AES67 support discovers
  streams via SAP — confirmed against a January 2025 PipeWire/AES67
  technical talk, which also notes a Windows-only `RAV2SAP` bridge
  tool exists specifically because these two don't interoperate
  natively. A RAVENNA device won't necessarily appear in this spec's
  auto-discovery UX (see "Routing UI" below) unless that specific
  device is also configured to announce via SAP, or some bridge is
  added. Needs more research as implementation develops — in
  particular, how common dual zeroconf/SAP announcement actually is
  across deployed RAVENNA hardware.
- **Axia/Telos Alliance (Livewire+)** — also clean: modern xNode
  hardware ships with AES67 as a first-class, built-in stream mode
  alongside native Livewire+, coexisting on the same network fabric.
- **Wheatstone (WheatNet-IP)** — AES67 is exposed at the Blade
  hardware boundary (e.g. Blade 4), not as an inherent property of the
  WheatNet-IP fabric as a whole. WheatNet-IP itself remains a separate
  proprietary network; the Blade is what speaks AES67 in and out of
  it. Practically fine, just not "the whole network is AES67."
- **Dante** — real, binding limitations, not just a mode flag:
  Dante-to-Dante traffic on the same network always uses Dante's
  native proprietary transport, regardless of whether AES67 mode is
  enabled on both ends — AES67 mode only governs Dante's interop
  boundary with non-Dante gear. AES67 mode itself is capped at 48kHz
  only, a maximum of 32 streams, and no redundancy; on some chipsets
  (Brooklyn II, HC, Broadway) the third-party AES67 stream being
  subscribed to must additionally be in the `239.x/16` multicast
  address range. None of this is a PipeWire-side limitation — it's
  intrinsic to how Dante itself implements AES67 mode, confirmed
  against Audinate's own documentation.

### Privilege model: resolved decision

The privilege mismatch between `caed` (historically root) and PipeWire
(normally per-user) is a closed architectural decision.

**Decision: `caed` runs as the `rd` user, not root.** Running as root
was legacy practice — everything root provided for `caed` is available
to a non-root user with the right systemd unit configuration and group
memberships, with no loss of functionality:

- **Real-time scheduling** — `LimitRTPRIO=99`, `LimitRTTIME=infinity`,
  `IOSchedulingClass=realtime`, and `IOSchedulingPriority=0` in `caed`'s
  systemd unit provide the same scheduling priority root's `RLIMIT_RTPRIO`
  did, without elevated privilege.
- **Audio hardware access** — `rd` is a member of the `audio` group;
  ALSA and the PipeWire-native paths both respect this.
- **PTP clock device** — `/dev/ptpN` is root-owned by default. A udev
  rule assigns it to a dedicated `ptp` group with `rd` as a member,
  giving `ptp4l` and `caed` direct access without root.

**PipeWire runs system-wide**, not per-user. A `pipewire-system.service`
unit (available alongside the default per-user unit in the `pipewire`
package) starts PipeWire as a system-level service accessible to any
`audio`-group member, including `caed`. WirePlumber also runs
system-wide (`wireplumber-system.service`) — routing policy must survive
across logins and reboots, not be owned by whoever happens to be logged
into the desktop. The previously-documented issue in
`ElvishArtisan/rivendell` issue #823 (root `caed` failing to reach a
per-user JACK/PipeWire instance) is resolved by this decision: `caed`
runs as the same user class that the system-wide PipeWire instance
serves.

The `pipewire-aes67.conf` dedicated-process deployment pattern (noted
in the Background section's reference data) is superseded — a single
system-wide PipeWire instance managing all four backends together is
exactly what this spec's cross-driver routing requirement demands. A
separate AES67-only process would reintroduce the inter-process routing
gap this design eliminates.

Full systemd unit design — startup ordering, readiness signaling,
live-playout-safe reconfiguration — is covered in
[`docs/specs/0010-systemd-stack-orchestration.md`](0010-systemd-stack-orchestration.md).

## Design

### Scope of the new driver

A new `cae/driver_pipewire.cpp`, built against PipeWire's native client
API (not the `pipewire-jack` compatibility shim), providing AES67/RTP
support specifically — following the same `MakeDriver`/`GetDriver`
registration pattern the three existing drivers already use.

### PTP grandmaster clock

AES67 requires a PTP grandmaster somewhere on the network; the new
driver should not require the operator to provision one separately as
a precondition just to get a working default. `ptp4l` can run as a
grandmaster sourced purely from the host's own system clock — no GPS
or PTP-hardware-timestamping NIC required — so `caed`'s PipeWire setup
should default to launching and managing its own local-clock `ptp4l`
grandmaster automatically, with the AES67 path otherwise unusable out
of the box.

That "no special NIC required" framing is the optimistic end of real
guidance, not the whole picture — worth being honest about rather than
letting it stand unqualified. A January 2025 PipeWire/AES67 technical
talk treats a PTP-hardware-timestamping-capable NIC (verified via
`ethtool -T`, showing `hardware-transmit`/`hardware-receive`/a real PTP
hardware clock) as the baseline expectation for a real deployment, with
specific recommended hardware for stations that don't already have one
(Intel i210 over PCIe; ASIX AX88279 over USB, which needs an
out-of-tree vendor module). Both things can be true — informal field
reports of software-only timestamping holding sync reliably, *and*
careful technical guidance defaulting to assuming hardware timestamping
— but this spec should say so explicitly rather than implying no NIC
consideration exists at all. Needs more research/field testing as
implementation develops, ideally on the actual hardware this gets
deployed on.

The same source also names concrete AES67-profile `ptp4l` tuning this
spec doesn't currently capture, beyond just "run a grandmaster": a
faster Sync message interval than stock `ptp4l` defaults to
(`logSyncInterval -3`), and DSCP QoS marking on both PTP and RTP
traffic (`dscp_event 46` / `dscp_general 34` — Expedited Forwarding for
PTP, Assured Forwarding 41 for audio RTP, the same marking scheme
already confirmed against vendor docs in "Vendor interoperability"
above). Without that marking, AES67 audio has no real protection from
being delayed or dropped by other traffic on a busy network switch —
this isn't cosmetic tuning, it's load-bearing for reliability at scale.
Needs to be folded into whatever config `caed` generates/manages for
its own `ptp4l` instance; exact mechanics still need working out.

This must not assume Rivendell's clock stays master forever: PTP's
Best Master Clock Algorithm means any better-quality clock already
present on the network (a dedicated GPS-locked grandmaster appliance,
or another device already configured as one) will automatically be
elected over it. This is correct, expected behavior to design around,
not a failure mode to suppress — Rivendell's own clock is a sane
default/fallback, not a forced master. The accuracy tradeoff is real
but acceptable at this project's scale: a software-only clock achieves
microsecond-to-low-millisecond accuracy, sufficient for audio
sample-clock alignment in a broadcast-station deployment, not the
nanosecond-grade precision of dedicated PTP hardware — consistent with
informal field reports of software-only PTP grandmasters (run on
ordinary server hardware, no special NIC) holding AES67 sync reliably
over a standard switched network.

### Cross-driver routing requirement

This spec's actual requirement is that ports on **all four** backends
(ALSA, JACK, HPI, PipeWire/AES67) be patchable to ports on any of the
others, in real time, with no exclusivity. This is the core
deliverable, not an optional extension of "add a fourth driver."

A concrete operational case this requirement has to cover, not just an
abstract any-to-any claim: live monitoring — a cue channel, driving
studio monitors — has to keep working, including when the source being
monitored is an AES67/PipeWire stream that needs to land on a plain
analog output. An AES67 source feeding an analog cue channel is exactly
the cross-driver routing this section already commits to, so it isn't
new scope on its own, but it deserves to be named explicitly rather
than left implicit, since "monitoring" tends to carry assumptions (low
latency, multiple simultaneous independent mixes, talkback) that plain
any-to-any patching doesn't automatically guarantee. Whether cue/studio
monitoring needs anything beyond ordinary patching — a dedicated
low-latency path, mix-minus behavior, more than one independent monitor
mix active at once — needs more research as this gets designed in
detail; not resolved here.

**Decision: PipeWire becomes the system's actual routing substrate for
all four backends, not just the new one.** Each existing driver keeps
its own native hardware-acquisition code exactly as today — HPI keeps
calling `RDHPISoundCard`/`RDHPIPlayStream`/`RDHPIRecordStream`, ALSA
keeps owning its hardware — but the *transport layer* changes: each
driver becomes a PipeWire stream client (`pw_stream`) rather than
doing its own private I/O. No bridge process, and no driver becomes
dead code superseded by a parallel path.

- **ALSA**: leans on PipeWire's own existing ALSA backend instead of
  `driver_alsa.cpp`'s current direct `snd_pcm_open` calls.
- **HPI**: the one case requiring genuinely new code — becomes a
  `pw_stream` client wrapping its existing vendor API calls, the same
  kind of work the new AES67 driver itself needs to do anyway, just
  with HPI's hardware underneath instead of network RTP.
- **JACK**: `driver_jack.cpp` is retired outright, not refactored.
  Once `caed` is itself a native PipeWire client, any external
  JACK-only application can already reach `caed`'s ports through the
  system's own `pipewire-jack` compatibility layer with no Rivendell
  code involved — PipeWire is what's actually serving the JACK
  protocol system-wide at that point, so a Rivendell-maintained JACK
  driver has no remaining purpose. The exact retirement mechanics
  (whether any thin compatibility shim is needed for existing
  `rd.conf` JACK settings) are implementation detail, not decided here.

**This makes PipeWire a mandatory runtime dependency for `caed` to
function at all** — not an optional fourth backend alongside
ALSA/HPI/JACK, but the substrate every backend's transport runs
through. That is a deliberate, accepted cost of this fork's
modernization mandate, consistent with `v6` already being the
no-compromise branch while the original fork continues to serve
anyone who needs to stay on older infrastructure — not a new category
of risk this decision introduces on its own. Two concrete consequences
worth naming rather than leaving implicit:
- It forecloses a real, currently-practiced deployment pattern:
  minimal/stripped installs that deliberately omit PipeWire entirely
  to reduce a playout box's footprint. That pattern stops being
  possible under this decision, full stop.
- It widens the blast radius of the privilege/session risk already
  noted above. Today that mismatch only matters once AES67/PipeWire is
  actually in use; under this decision it becomes a precondition for
  `caed` to start at all, on every installation, regardless of whether
  that station ever touches AES67.

**Two alternatives were considered and rejected, not just unconsidered
— worth keeping on record so this ground doesn't get re-litigated
without the reasoning that closed it:**
- A bridging layer that left every existing driver's internals fully
  untouched (no transport-layer change at all) was the conservative
  option, and was rejected specifically because it leaves PipeWire/AES67
  as one driver among four with its own special-case bridge, rather
  than the first-class substrate this decision commits to. It remains
  the lower-risk fallback if the transport-layer refactor proves
  harder than expected during implementation.
- Having PipeWire absorb hardware directly via its own ALSA backend
  (superseding the existing ALSA/HPI drivers rather than refactoring
  their transport layer) was rejected because it doesn't have a clean
  answer for HPI at all — AudioScience's hardware speaks a proprietary
  vendor API, not ALSA, so that approach would have needed a separate
  carve-out for HPI anyway, undermining its own main appeal of
  architectural uniformity.

**Both questions this raised are now resolved, not just identified:**

- `setPassthroughLevel` survives as a real operation, not a
  compatibility shim. It is a *gain* control between two ports on the
  same card (an intra-node property), a different concept from
  *connecting* two ports (an inter-node link — the new thing PipeWire's
  graph actually adds). PipeWire nodes carry their own volume/gain
  properties independent of link topology, so this method gets
  reimplemented to set a PipeWire property rather than demoted to a
  wrapper.
- The one existing static patch mechanism in production use today,
  JACK's `[JackSession]` `rd.conf` config consumed by the now-retired
  `DriverJack::JackSessionSetup()`, gets **no automated migration —
  because the whole point of this work is to make that mechanism
  unnecessary, not to carry it forward in a new shape.** The entire
  objective of this modernization is to replace the complexity of the
  old persistent-patch model, not reimplement it under a new name.
  Installing the system, opening the dashboard, and seeing every
  source and destination across all four backends already there,
  ready to drag-and-drop into place, is the feature — not a consolation
  for losing something. A handful of static patches taking a few
  minutes to re-draw on a system that auto-discovers its own I/O isn't
  a cost worth building a migration tool to avoid; it's the
  demonstration that the simpler model actually works. Operators moving
  from a JACK-based install re-establish their routing by using the
  thing this spec was built to deliver, not by porting forward the
  thing it replaces.

Persistence for the *new* system is a real, durable mechanism, not
absent — it lives in WirePlumber, PipeWire's own session/policy manager.
WirePlumber manages link policy via its own config (declarative rules,
changeable at runtime via `wpctl`, stored so it survives reboots) — this
is genuine persistence, not a desktop-session artifact. The dashboard's
own ad hoc graph edits (a live drag-to-connect) are a *separate*,
genuinely temporary layer unless explicitly promoted into WirePlumber's
policy — most likely by the dashboard generating WirePlumber's policy
config directly and triggering a reload, treating that config as a
render target it owns rather than a file an operator edits by hand.
This split (temporary live-graph state vs. persistent WirePlumber
policy) is inherent to how PipeWire and WirePlumber are designed to
interact, not a Rivendell limitation, and the dashboard's UI must make
that explicit rather than let a patch silently revert on reboot with no
visible explanation.

This must satisfy the same end requirement regardless: one unified,
real-time-patchable graph spanning all four backends.

### Routing UI

The routing-matrix interface — expected to be the Go web dashboard
(`0005-go-api-foundation.md`) — is intended to fully replace `qjackctl`
as the patchbay UI, not coexist alongside it. One interface for the
entire any-to-any patchbay across all four backends.

**This has no existing pattern to extend anywhere in this codebase.**
Confirmed by auditing every module that talks to `caed`
(`rdairplay`, `rdpanel`, `rdcatch`, `rdlibrary`): routing/passthrough
operations (`setPassthroughVolume`, `setClockSource`, etc.) are called
*only* from `rdadmin`, and even there only to write static config to
the database — never to the live daemon. No GUI in this codebase has
ever exposed a live routing control. The Go dashboard's patchbay is a
wholly new UX, not a modernization of an existing one, and the wire
protocol it would call into doesn't have the necessary commands yet
either (see `ARCHITECTURE.md`'s `caed` network protocol notes) — both
the UI and the protocol underneath it need to be designed from
scratch.

**Required, not optional, parts of that new UX:**
- Continuous auto-discovery of all I/O across all four backends, not
  a one-time scan at first launch. Every source and destination
  should already be present and named the moment the dashboard is
  opened, and that has to stay true across every reboot and every
  topology change after that — a card added, a card removed, an AES67
  stream appearing or disappearing on the network — detected and
  reflected automatically. No terminal, no config file, no manual
  rescan action, ever, for the dashboard to learn what's actually
  there. PipeWire's own graph already emits exactly these add/remove
  events; the dashboard subscribes to them rather than polling or
  requiring a refresh.

  One real precondition this doesn't eliminate: PipeWire's SAP-based
  discovery (`module-rtp-sap`) needs an explicit network interface
  configured before it listens for anything at all, plus stream-match
  rules before it acts on what it hears — confirmed against PipeWire's
  own module documentation. That's not "no config file, ever" in the
  literal sense at the PipeWire layer, but it must still be true at the
  operator-facing layer: this one-time network-scope setup (interface,
  multicast scope) is required, not optional, to live as a setting
  inside the Go dashboard itself, generated into whatever PipeWire
  config it manages. Opening a terminal or hand-editing a PipeWire
  config file is not an acceptable fallback for this, not even for
  initial setup — the dashboard owns this end-to-end or it doesn't ship.
  Needs more research once the dashboard's own settings surface is
  designed, but the constraint itself isn't open for revision.
- An explicit persistent/temporary state on every patch, not an
  implicit one. Every ad hoc connection the operator makes starts
  temporary by default; making it survive a reboot is a deliberate,
  visible action (promoting it into WirePlumber's policy config), never
  an assumption. The UI must say so plainly — this is how PipeWire and
  WirePlumber are designed to interact, not a Rivendell limitation, and
  an operator should never be surprised by a patch silently reverting.
- One dashboard, not three places. Once `setPassthroughLevel`-style
  gain control is reimplemented as a PipeWire node/port property (see
  above), it belongs in the same view as the patchbay, not a separate
  screen — gain and routing are two properties of the same graph
  objects the dashboard already has to render to do patching at all.
  The goal is one master control surface, not gain in one place and
  routing in another.
- A curated, named graph, not a raw one — stated here as an explicit
  objective, not an aspiration. A raw PipeWire graph exposes internal
  cruft (monitor ports, loopback nodes) no broadcast operator should
  have to parse. Achieving "zero knowledge of the underlying plumbing"
  requires deliberate work: good `node.description`/port metadata set
  by `driver_pipewire.cpp` and every refactored driver, and filtered
  dashboard views that hide non-Rivendell plumbing. This does not fall
  out automatically from building on PipeWire — it has to be designed
  and built.

### Core/CPU affinity tuning

A real, designed feature, not solely an investigation: automatic
detection of available CPU cores/resources at startup, so a station
migrating to smaller hardware does not silently degrade or choke, plus
a full manual override. Surfaced through the Go web dashboard. This is
scoped alongside the PipeWire driver work given the shared `caed`
surface area, but is not the top development priority within this
spec.

### Related, lower-priority investigation

Per-thread `SCHED_FIFO` scheduling for ALSA audio callback threads was
implemented and then disabled (`cae/driver_alsa.cpp:1589`, wrapped in
`#if 0`) — only daemon-wide `SCHED_FIFO` (`cae/cae.cpp:255-267`, gated
by `rd.conf`'s `useRealtime()`) is currently active. Investigating why
the per-thread version was disabled and whether re-enabling it
addresses real-time scheduling issues independent of the PipeWire work
is worth doing in parallel — it is unrelated to and does not block this
spec's AES67/routing work.

## Files

- New: `cae/driver_pipewire.{cpp,h}`.
- Modified, transport layer refactored onto `pw_stream` per the
  cross-driver routing decision above: `cae/cae.cpp`,
  `cae/driver_alsa.cpp`, `cae/driver_hpi.cpp`, `lib/rdstation.{cpp,h}`.
- Removed: `cae/driver_jack.{cpp,h}` — retired per the decision above,
  not carried forward.
- `configure.ac`: add PipeWire client library detection; remove JACK
  client library detection once `driver_jack.{cpp,h}` is actually gone.
- **`cae/cae_server.{h,cpp}` and `lib/rdcae.{h,cpp}`** — the existing
  24-command wire protocol between `caed` and every client has no
  commands for cross-driver routing or AES67/PTP status today; new
  command codes and matching `RDCae` client methods are required, not
  optional plumbing. See `ARCHITECTURE.md`'s `caed` network protocol
  notes for the existing command set this needs to extend.
- **`utils/rddbmgr/create.cpp` and `updateschema.cpp`** — `AUDIO_CARDS`
  has no network-shaped columns at all (no multicast address, PTP
  domain, SAP session name, or per-stream sample rate); a real schema
  migration (new columns or a new child table) is required before any
  AES67 config can persist.

## Verification

1. AES67 stream send/receive verified against at least one other real
   AES67/Dante-capable device on the same network, confirming SAP
   discovery and PTP sync actually interoperate, not just that
   PipeWire's modules load.
2. Cross-driver routing verified directly: patch a live signal from an
   ALSA-backed input to a JACK-backed output (or any other cross-driver
   pair) and confirm audio actually flows, not just that both nodes
   appear in the same graph.
3. Existing ALSA/JACK/HPI playout paths re-verified unaffected for
   configurations that don't use the new routing/AES67 functionality at
   all — this spec must not regress current single-driver deployments.

## Phase 1 implementation (system-scope PipeWire, 2026-07-01)

Before the full caed driver rewrite (see Files section above), Phase 1
establishes the system-scope PipeWire foundation that everything else
builds on. It does not touch caed's audio driver code. The full caed
rewrite (driver_pipewire.cpp, transport-layer refactor, AES67) follows
as Phase 2 once Phase 1 is verified end-to-end.

### What Phase 1 delivers

- `rdservice/rdservice.cpp:89–92`: removed the `geteuid()!=0` exit block
  that prevented rdservice (and its children caed, ripcd, rdcatchd) from
  running as the `rd` user. Enables the `User=rd` drop-in in
  `conf/systemd/rivendell.service.d/rivolution.conf`.
- `conf/systemd/pipewire-system.service`: system-scope PipeWire running as
  `rd`, socket at `/run/pipewire-system/pipewire-0`. Ubuntu 26.04 ships
  only user-scope PipeWire units; this is the system-scope equivalent
  required for broadcast services to reach PipeWire without a logged-in
  user session.
- `conf/systemd/wireplumber-system.service`: system-scope WirePlumber
  bound to `pipewire-system.service`. Routing policy from the dashboard
  goes to `/etc/wireplumber/` (system-scope), not `~/.config/wireplumber/`
  (which is session-scoped and unavailable before login).
- `conf/systemd/rivolution-stack.target`: added `Wants=` for both new
  services so they start with the stack.
- `conf/systemd/rivendell.service.d/rivolution.conf` and
  `conf/systemd/liquidsoap.service`: both add
  `Environment=XDG_RUNTIME_DIR=/run/pipewire-system` so caed's JACK calls
  and Liquidsoap's `input.jack()` both find the system PipeWire socket via
  `pipewire-jack`.
- Dashboard `service_status.go`: added PipeWire and WirePlumber to the
  managed unit list so their status is visible on the System page.

### Phase 1 runtime prerequisites (manual steps)

These steps are performed once on the live system before the Phase 1
configuration is deployed:

1. Rebuild and install rdservice with the root check removed:

```
cd ~/dev/rivolution && make -j$(nproc) rdservice
```

```
sudo cp rdservice/rdservice /usr/sbin/rdservice
```

2. Install the JACK-to-PipeWire compatibility bridge:

```
sudo apt install pipewire-jack
```

3. Configure the JACK client library system-wide so caed and Liquidsoap
   use PipeWire's JACK implementation rather than a native JACK server:

```
sudo cp /usr/share/doc/pipewire/examples/ld.so.conf.d/pipewire-jack-x86_64-linux-gnu.conf /etc/ld.so.conf.d/
```

```
sudo ldconfig
```

   On arm64 the filename may differ; use `ls /usr/share/doc/pipewire/examples/ld.so.conf.d/`
   to confirm the exact name.

4. In RDAdmin, configure the sound card(s) that Rivendell uses for
   playout to use the **JACK** driver. caed will connect those cards to
   the system PipeWire graph via the JACK bridge.

5. Deploy all conf/ files per the deployment section and verify:
   - `pipewire-system.service` and `wireplumber-system.service` show
     `active (running)`
   - `rivendell.service` shows `active (running)` with caed, ripcd,
     rdcatchd listed as child processes running as `rd` (not root)
   - `liquidsoap.service` shows `active (running)` with no JACK error
     in `/home/rd/Log/liquidsoap.log`
   - `pw-dump` (as `rd` with `XDG_RUNTIME_DIR=/run/pipewire-system`)
     shows caed and Liquidsoap as nodes in the graph

### Phase 1 known gap

caed's JACK driver (`cae/driver_jack.cpp`) connects to the PipeWire-JACK
bridge today, but the full Phase 2 goal — caed becoming a native PipeWire
client (`pw_stream`) so ALSA and HPI cards also route through PipeWire —
is not yet done. In the Phase 1 state, cards configured for ALSA driver
in RDAdmin still use ALSA directly and are not visible in the PipeWire
graph. Only JACK-driver cards participate in the unified PipeWire graph
at this stage.

## Implementation deviations

- **2026-07-01:** Ubuntu 26.04's `pipewire` package ships no system-scope
  unit files (only user-scope). Custom `pipewire-system.service` and
  `wireplumber-system.service` unit files written for
  `conf/systemd/`. Spec referenced `pipewire-system.service` as
  "available alongside the default per-user unit" — that was incorrect;
  the custom units correct the gap.
