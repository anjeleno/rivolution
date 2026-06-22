# 0007 — PipeWire audio engine: native AES67 and unified cross-driver routing

**Date:** 2026-06-21

## Goal

Add native PipeWire support to `caed`, specifically to deliver: (1)
native AES67 audio-over-IP support, and (2) real-time, any-to-any audio
routing across every backend `caed` supports (ALSA, JACK, AudioScience
HPI, and the new PipeWire/AES67 path), with no exclusivity between
them. This touches `caed`, the highest-risk code in this project
(per `BACKLOG.md`), and is scoped accordingly.

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

`pipewire-jack`, PipeWire's JACK-API compatibility layer, would let an
existing JACK client work against a PipeWire server with no source
changes. This spec does not rely on that shim for the new AES67
driver — native support is wanted specifically — but it remains a
relevant fact for any future evaluation of running existing JACK-based
tooling (e.g. `qjackctl`-adjacent workflows) against a PipeWire server
during a transition period.

## Design

### Scope of the new driver

A new `cae/driver_pipewire.cpp`, built against PipeWire's native client
API (not the `pipewire-jack` compatibility shim), providing AES67/RTP
support specifically — following the same `MakeDriver`/`GetDriver`
registration pattern the three existing drivers already use.

### Cross-driver routing requirement

The existing `driver_alsa.cpp`/`driver_jack.cpp`/`driver_hpi.cpp` files
are not removed — there is no reason to delete working hardware-
specific code. However, this spec's actual requirement is that ports
on **all four** backends (ALSA, JACK, HPI, PipeWire/AES67) be patchable
to ports on any of the others, in real time, with no exclusivity. This
is the core deliverable, not an optional extension of "add a fourth
driver."

The exact mechanism for achieving this is itself open implementation
design, to be resolved during implementation, not decided by this
spec:

- **Option A:** PipeWire absorbs the underlying hardware directly
  (via its own ALSA backend), so the existing ALSA- and HPI-backed
  cards become native PipeWire graph nodes, and `caed`'s existing
  drivers are effectively superseded by PipeWire-side equivalents for
  routing purposes.
- **Option B:** the existing drivers remain the owners of their
  hardware as today, and a bridging layer exposes each driver's ports
  as PipeWire graph nodes, leaving the existing drivers' internal
  implementation untouched.

Whichever option is chosen must satisfy the same end requirement: one
unified, real-time-patchable graph spanning all four backends.

### Routing UI

The routing-matrix interface — expected to be the Go web dashboard
(`0005-go-api-foundation.md`) — is intended to fully replace `qjackctl`
as the patchbay UI, not coexist alongside it. One interface for the
entire any-to-any patchbay across all four backends.

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
- Reference/possibly modified depending on which routing option (A/B
  above) is chosen during implementation: `cae/cae.cpp`,
  `cae/driver_alsa.cpp`, `cae/driver_jack.cpp`, `cae/driver_hpi.cpp`,
  `lib/rdstation.{cpp,h}`, `lib/rdmatrix.cpp`.
- `configure.ac`: add PipeWire client library detection.

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

## Implementation deviations

None yet — implementation has not started.
