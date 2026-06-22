# Roadmap

Planned features and direction for this fork, written as
a blueprint rather than a backlog of bugs (that's `BACKLOG.md`). An
entry here is a destination, not a commitment to a timeline. Once an
entry is actually picked up, it gets a real spec in `docs/specs/` and a
branch; this file should then link to that spec rather than duplicate
its detail.

## Nested cart rotation (carts containing carts, not just cuts)

Today a single cart can hold multiple cuts, and the log scheduler
rotates *between carts* in a category but treats all cuts inside one
cart as equivalent alternates of the same element, not as independently
trackable rotation members. Two concrete problems this causes:

1. **Same-song collision risk:** if multiple remixes/versions of one
   song are split across separate carts in a category (rather than cuts
   in one cart), nothing stops the scheduler from picking two or three
   of them back-to-back before the category cycles through its other
   members — there's no memory that "these are variants of the same
   thing, don't stack them."
2. **Now/Next granularity:** Now/Next metadata is sourced from the
   cart, not the individual cut. A cart holding three remixes as
   separate cuts sends the same Now/Next data regardless of which
   remix cut is actually playing — listeners/streaming metadata never
   see which specific version is on air. Applies equally to promos and
   imaging, not just music.

**Requested feature:** support a cart that itself contains multiple
*carts* (not cuts) as rotation members, the same way a cart today
contains multiple cuts — modeled on how Music Master/Selector handled
this (a "packet" of carts inside another cart, treated identically to
multiple cuts in a cart from the scheduler's point of view). This would
let a single category-rotation slot rotate among real, independent
carts internally, with Now/Next correctly reflecting whichever cart is
actually playing — built into the native Rivendell scheduler, not an
external workaround.

Not yet scoped: data model (new table/relationship vs. reusing the
existing cut mechanism), how rotation type (sequential/percentage/
weight) applies at the nested level, RDAirplay/Now-Next propagation
changes required. Needs its own spec before implementation starts.

## Missing-audio library audit tool

Companion to the bug tracked in `BACKLOG.md` ("RDAirplay plays silence
for a cart with a missing audio file, instead of skipping it") — that
entry is the playout-time bug fix; this is the separate, proactive
feature that would help catch the underlying data problem before it
ever reaches air.

**Requested feature:** an admin/maintenance tool that scans the
database for cuts in specific categories, confirms each cart (and every
cut inside a multi-cut cart) actually has a real, present audio file on
disk (not just a cut record), and generates a report of anything
missing — so absent files can be identified and copied back into
`/var/snd` deliberately, rather than discovered live on air.

Not yet scoped: where this lives (RDAdmin tool, standalone CLI script,
RDLibrary feature), exact report format, whether it should also offer
to act (e.g. flag/disable carts with missing audio) or stay read-only.

## Full modernization (the "v6" effort)

Longer-term direction, with several major decisions now locked in and
real specs written for them. Each spec still needs implementation —
none of this is built yet — but the architectural shape is no longer
speculative for the items below.

**Decided, with a real spec:**
- **Go REST API + web dashboard, covering `RDAdmin`/`RDLogManager`/
  `RDCatch`-shaped administration functionality.** `RDAirplay`/
  `RDLibrary`/`RDLogEdit`/`RDPanel` stay native, untouched. The Go layer
  is primarily a proxy/translator in front of the existing `rdxport.cgi`
  HTTP/XML API (46 commands, already documented in
  `docs/apis/web_api.xml`) — not a new transport, and never touches RML
  (RML stays exclusively internal to the native apps). See
  `docs/specs/0005-go-api-foundation.md` for the full bucket-based
  risk classification, Phase 1 scope, and the authentication design
  (JWT for browser sessions, layered over `rdxport.cgi`'s existing
  IP-bound ticket system).
- **Qt6 migration for the native desktop applications.** Real,
  medium-sized, mechanically tractable scope (a handful of
  `QWebView`/`QRegExp`/`QString::KeepEmptyParts` migrations plus a
  `configure.ac` module-detection change) — see
  `docs/specs/0006-qt6-migration.md`. No technical dependency on the Go
  API work; the two proceed in parallel.
- **Native PipeWire support in `caed`, for AES67 and real cross-driver
  routing.** `caed` already runs ALSA/JACK/AudioScience HPI
  concurrently, but every route is driver-private today — there is no
  way to patch a port on one driver to a port on another. PipeWire's
  unified graph model, plus its native AES67 support (PTP sync, SAP
  discovery, available from PipeWire 1.1 — confirmed packaged in Ubuntu
  26.04) is the target architecture for closing that gap, not just
  adding AES67 as an isolated fourth driver. See
  `docs/specs/0007-pipewire-audio-engine.md` for the full design,
  including the requirement that the new routing UI (likely the Go
  dashboard) fully replace `qjackctl`, and the core/CPU-affinity-tuning
  feature scoped alongside it.
- **Broadcast tool suite integration** (Icecast, Liquidsoap, Stereo
  Tool, VLC, persistent patch connections) **as Go-managed
  configuration**, so an operator never hand-edits any of these tools'
  native config files directly. See
  `docs/specs/0008-broadcast-tool-suite-integration.md`.

**Repo structure, sequencing decided:** finish verifying and merging
the currently in-flight branches into `v4` first; only then copy that
resulting `v4` (carrying every real fix already made) into a new repo
for the v6 modernization work itself. This repo continues to exist and
be maintained independently for production use — the new repo is
additive, not a replacement.

**Product naming — not decided.** A renamed public identity (distinct
from repo/internal naming) is under consideration. Any internal-code
renaming (the `RD`-prefixed class names, the `rd` user/group
convention, `rd.conf`, `/var/snd`, binary names) is explicitly out of
scope regardless of what name is eventually chosen — only the brand/
marketing layer (product name, package name, documentation) would
change. The name itself needs real trademark clearance before
committing, not a roadmap decision.

**Still genuinely open:**
- Container-forward design is an agreed principle for new components
  (the Go API, broadcast-tool config management) but applied
  per-component, not as a blanket rule — it explicitly doesn't apply to
  the live audio engine itself.
- Exact core/CPU-affinity auto-detection heuristics and manual-override
  UI design (spec 0007 names the feature; the detailed design is not
  yet written).
- Which `rdxport.cgi`-proxied endpoints (if any) eventually move to
  native Go — deliberately case-by-case, not pre-decided (spec 0005).
- Exact per-tool config schema/validation design for spec 0008.
