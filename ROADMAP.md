# Roadmap

Planned features and direction for this fork, written as
a blueprint rather than a backlog of bugs (that's [`BACKLOG.md`](https://github.com/anjeleno/rivolution/blob/main/BACKLOG.md)). An
entry here is a destination, not a commitment to a timeline. Once an
entry is actually picked up, it gets a real spec in
[`docs/specs/`](https://github.com/anjeleno/rivolution/tree/main/docs/specs) and a
branch; this file should then link to that spec rather than duplicate
its detail.

## Evaluate replacing the DocBook-XSL + FOP documentation toolchain

The current docs pipeline (`docs/rivwebcapi`, `docs/manpages`,
`docs/opsguide`, `docs/dtds`, `docs/apis`) transforms DocBook XML
through custom XSLT stylesheets, then renders PDF output via Apache
FOP, a Java/JVM-based renderer. Two separate rough edges surfaced
while debugging an unrelated build crash in this pipeline (see
[`BACKLOG.md`](https://github.com/anjeleno/rivolution/blob/main/BACKLOG.md)
for the crash itself): FOP doesn't implement `table-layout="auto"`,
and the toolchain's only failure mode for a renderer crash is a raw
JVM `hs_err_pid*.log` dump, with no graceful fallback. Worth a
deliberate evaluation of whether to keep this pipeline as-is or replace
it, rather than letting incremental crash workarounds accumulate.

Two candidate directions, neither evaluated in depth yet:

- **Swap only the PDF renderer, keep DocBook XML as the source
  format:** `dblatex` (DocBook → LaTeX → PDF) as a replacement for the
  FOP half of the pipeline. Smaller-scope change — the existing DocBook
  XML sources stay untouched — but this fork already has a customized
  FO stylesheet (`helpers/docbook/fo/docbook.xsl`) controlling FOP's
  output; `dblatex` doesn't consume that file at all, so switching
  means either accepting `dblatex`'s own default styling or re-porting
  those customizations into `dblatex`'s separate (LaTeX-based)
  customization mechanism — unscoped until someone actually compares
  output quality side by side. No JVM involved, so this entire class of
  crash disappears outright.
- **Move off DocBook entirely:** rewrite the source documents in a
  more actively maintained format — AsciiDoc (`asciidoctor-pdf`) or
  Markdown (`pandoc`) are the two obvious candidates. Far larger
  scope: every one of the ~50+ XML sources across all five doc
  directories would need rewriting, not just the rendering step, and
  every directory's `Makefile.am` build rules would need rewriting to
  match. Gets the most modern, best-maintained tooling of the two
  options, at the highest cost.

Not picked up yet — deliberately deferred rather than decided under
time pressure while a build was actively blocked on the unrelated JIT
crash above. Whichever direction (if any) gets picked up should get a
real spec in
[`docs/specs/`](https://github.com/anjeleno/rivolution/tree/main/docs/specs)
once decided.

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

Companion to the bug tracked in [`BACKLOG.md`](https://github.com/anjeleno/rivolution/blob/main/BACKLOG.md) ("RDAirplay plays silence
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

## Configurable segue ducking in RDAirplay

Found while investigating the segue back-timing/no-fade truncation fix
(2026-07-07, see `CHANGELOG.md` and `docs/specs/0002-segue-backtiming.md`).
While tracing how the outgoing and incoming elements' volumes interact
during a segue, it became clear there's a real gap: automated,
fully-scheduled segues never duck at all today, and the only ducking
mechanism that exists is manual and out of scope for this feature.

**Current state, confirmed against source:**

- `DUCK_UP_GAIN`/`DUCK_DOWN_GAIN` exist only as columns on `LOG_LINES`
  — a value that belongs to one specific scheduled log line, not to a
  cart or cut. There is no such column anywhere on `CUTS`. Default is
  `0` for every line (`lib/rdlog_line.cpp:142`).
- The **only** thing that ever writes a nonzero value into either
  column is the voice tracker, by a DJ manually dragging duck handles
  in the waveform view (`lib/rdtrackerwidget.cpp`). Nothing else in the
  codebase ever sets them.
- `RDPlayDeck` reads both values at cue time (`lib/rdplay_deck.cpp:239-240`)
  and does act on them for real — B's duck-up only engages if
  `duckUpGain()!=0` (`lib/rdplay_deck.cpp:441`), A's duck-down only
  engages if `duckDownGain()<0` (`lib/rdplay_deck.cpp:558`). So the code
  path is live and runs on every segue; it's the *value* that's inert —
  since ordinary automated log lines are never touched by the voice
  tracker, this is always "duck by zero," meaning no audible ducking
  ever happens on a normal, fully-automated segue today.
- The duck curve durations are hardcoded, not configurable or stored
  anywhere: `RDPLAYDECK_DUCKDOWN_LENGTH 750` / `RDPLAYDECK_DUCKUP_LENGTH
  1500` (milliseconds) in `lib/rdplay_deck.h:35-36`.
- Gain values throughout the codebase are stored as hundredths of a dB
  (confirmed via `RD_FADE_DEPTH -3000` = -30.00 dB), so 0.5 dB steps are
  trivial precision-wise — no schema constraint stands in the way of
  that granularity.

**Requested feature:** a user-editable ducking amount (in 0.5 dB steps)
that applies automatically during any fully-automated segue transition
in RDAirplay — completely independent of carts, cuts, and the voice
tracker, which must remain entirely untouched by this work (no shared
columns, no shared code path). Likely surfaced as a per-host RDAirplay
setting in RDAdmin (Manage Hosts → [host] → RDAirplay), not the Go
dashboard, since this is a live-playout-engine behavior rather than
library or system configuration — final placement still to be decided.

**Genuinely open questions, deliberately not decided yet:**

- Does the duck apply only to the outgoing element (A dips, B plays at
  full volume — the leading theory), or to both elements simultaneously?
- How does this interact with "No fade on segue out" (fixed 2026-07-07,
  see `docs/specs/0002-segue-backtiming.md`)? That flag now means A
  plays its tail out completely undisturbed to its own natural end —
  does a duck amount still apply on top of that (A's tail plays out in
  full but at a reduced level), or does "no fade" mean hands-off
  entirely, with no ducking either? These two features clearly
  interact and need to be designed together, not bolted on separately.
- How does this interact with segue back-timing's delay math? Back-timing
  already computes when to fire B relative to A's tail and B's intro
  length — ducking changes what's audible during that overlap window but
  presumably shouldn't change the timing math itself; needs to be
  confirmed rather than assumed.
- Whether the curve duration should scale with duck depth (a -6 dB duck
  and a -2 dB duck both need to sound smooth, not just use the same
  fixed 750/1500ms window regardless of depth) — deliberately deferred
  until this can actually be heard on real air before deciding whether
  the curve math needs to change at all.
- Exact storage location for the new default value (a new config table/
  column, separate entirely from `LOG_LINES.DUCK_UP_GAIN`/`DOWN_GAIN`).

**Needs a full spec before implementation starts** — this has enough
interacting variables (back-timing, the no-fade behavior, one-element
vs. two-element ducking, curve shape) that it warrants a dedicated
planning pass rather than incremental design in the middle of another
fix.

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
  [`docs/specs/0005-go-api-foundation.md`](https://github.com/anjeleno/rivolution/blob/main/docs/specs/0005-go-api-foundation.md) for the full bucket-based
  risk classification, Phase 1 scope, and the authentication design
  (JWT for browser sessions, layered over `rdxport.cgi`'s existing
  IP-bound ticket system).
- **Qt6 migration for the native desktop applications.** Real,
  medium-sized, mechanically tractable scope (a handful of
  `QWebView`/`QRegExp`/`QString::KeepEmptyParts` migrations plus a
  `configure.ac` module-detection change) — see
  [`docs/specs/0006-qt6-migration.md`](https://github.com/anjeleno/rivolution/blob/main/docs/specs/0006-qt6-migration.md). No technical dependency on the Go
  API work; the two proceed in parallel.
- **Native PipeWire support in `caed`, for AES67 and real cross-driver
  routing.** `caed` already runs ALSA/JACK/AudioScience HPI
  concurrently, but every route is driver-private today — there is no
  way to patch a port on one driver to a port on another. PipeWire's
  unified graph model, plus its native AES67 support (PTP sync, SAP
  discovery, available from PipeWire 1.1 — confirmed packaged in Ubuntu
  26.04) is the target architecture for closing that gap, not just
  adding AES67 as an isolated fourth driver. See
  [`docs/specs/0007-pipewire-audio-engine.md`](https://github.com/anjeleno/rivolution/blob/main/docs/specs/0007-pipewire-audio-engine.md) for the full design,
  including the requirement that the new routing UI (likely the Go
  dashboard) fully replace `qjackctl`, and the core/CPU-affinity-tuning
  feature scoped alongside it.
- **Broadcast tool suite integration** (Icecast, Liquidsoap, Stereo
  Tool, VLC, persistent patch connections) **as Go-managed
  configuration**, so an operator never hand-edits any of these tools'
  native config files directly. See
  [`docs/specs/0008-broadcast-tool-suite-integration.md`](https://github.com/anjeleno/rivolution/blob/main/docs/specs/0008-broadcast-tool-suite-integration.md).

**Repo structure, sequencing decided:** finish verifying and merging
the currently in-flight branches into `v4` first; only then copy that
resulting `v4` (carrying every real fix already made) into a new repo
for the v6 modernization work itself. This repo continues to exist and
be maintained independently for production use — the new repo is
additive, not a replacement.

**Product naming — decided: Rivolution.** Trademark clearance is done.
Public identity has been live since 2026-06-22 (the GitHub repository
itself, [`anjeleno/rivolution`](https://github.com/anjeleno/rivolution))
and 2026-06-23 (the [rivolution.dev](https://rivolution.dev/) landing
page). As planned, this is brand/marketing-layer only — internal code
(the `RD`-prefixed class names, the `rd` user/group convention,
`rd.conf`, `/var/snd`, binary names) was never in scope for renaming
and remains unchanged.

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

## Wiki markdown copied into the main repo for portability

The GitHub wiki (`anjeleno/rivolution.wiki`) is, architecturally,
always its own separate git repository — that's true for every GitHub
repo with the wiki feature enabled, not a choice made for this project
specifically. Right now its two pages (`Home.md`,
`Build-From-Source.md`) only exist there, so cloning `rivolution` alone
doesn't get you the wiki's source content, and a fork of `rivolution`
doesn't carry the wiki along with it.

**Decided approach: a manual, documented copy step, not CI.** An
earlier version of this entry planned a GitHub Actions workflow to
auto-sync the wiki on every push to `main`, but that needs a real
GitHub wiki write workaround — the default `GITHUB_TOKEN` issued to
Actions runs doesn't have write access to a repo's wiki, only to the
repo itself, so the workflow would have needed a separate Personal
Access Token stored as a repo secret just to push there. Decided that
overhead isn't worth it for two pages: instead, periodically copy
`rivolution.wiki`'s current markdown into `rivolution/docs/wiki/` by
hand (`cp` + commit), purely so the content travels with a clone or
fork of `rivolution`. The wiki repo itself stays the live, editable
source — `docs/wiki/` is a point-in-time mirror, not a synced copy, and
won't necessarily reflect the latest wiki edit until the next manual
pass.
