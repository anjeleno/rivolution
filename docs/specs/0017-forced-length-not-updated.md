# 0017 — `RDCart::updateLength()` never writes `FORCED_LENGTH`, so automated updates show zero length in RDLibrary

**Date:** 2026-07-26
**Status:** Implemented on branch `fix-forced-length-not-updated`. Builds
clean; not yet run, tested against the verification plan below, or
merged.

## Goal

A cart's `FORCED_LENGTH` column — what RDLibrary's browse grid actually
displays as "Length" — is only ever written by RDLibrary's own Edit Cart
dialog. Every automated path that recomputes a cart's length (`rdimport`,
the web import handler, `rddbmgr --check`, marker-set tools, schema
migrations) leaves it untouched, so a freshly-imported cart shows zero
length until someone manually opens it in RDLibrary and presses OK. The
fix restores the write that used to happen unconditionally as part of
the same recompute.

## Background — verified against source, not assumed

`RDCart::updateLength(bool enforce_length, unsigned length)`
(`lib/rdcart.cpp:1071-1294`) takes `enforce_length`/`length` as
parameters but never references either one anywhere in its current body
— confirmed by grepping the full function text. Every write branch
(active cuts, evergreen cuts, "nothing playable") updates
`AVERAGE_LENGTH`/`AVERAGE_SEGUE_LENGTH`/`AVERAGE_HOOK_LENGTH` but never
`FORCED_LENGTH`.

**Root cause, confirmed via `git log -S"setForcedLength" -- lib/rdcart.cpp`:**
the same commit that broke `AVERAGE_SEGUE_LENGTH`'s semantics (see
`CHANGELOG.md`'s 2026-07-26 entry on that fix) also silently dropped
this. Diffing that commit directly shows the pre-refactor function had,
in each branch:

```cpp
if(weight_total>0) {
  setAverageLength(total/weight_total);
  setAverageSegueLength(segue_total/weight_total);
  setAverageHookLength(hook_total/weight_total);
  if(!enforce_length) {
    setForcedLength(total/weight_total);
  }
}
else {
  setAverageLength(0);
  setAverageSegueLength(0);
  setAverageHookLength(0);
  if(!enforce_length) {
    setForcedLength(0);
  }
}
```

The evergreen-cuts restructuring rewrote this into the current
active-cuts/evergreen/nothing-playable branch shape but dropped every
`setForcedLength()` call — collateral damage from the same refactor,
unrelated to its actual (legitimate) evergreen-cuts goal.

**Confirmed this reaches the real display in this fork today.**
`lib/rdlibrarymodel.cpp` (RDLibrary's live browse-grid model, not the
print-report code) selects `CART.FORCED_LENGTH` (~line 898) and shows it
directly as the "Length" column (~line 778) — unchanged from upstream.
Every automated caller of the zero-arg `RDCart::updateLength()` —
`utils/rdimport/rdimport.cpp` (both call sites), `web/rdxport/import.cpp`
(the server-side handler every import client funnels through, see
`ARCHITECTURE.md`'s "MP3 import pipeline" section), `utils/rddbmgr/
check.cpp`, `utils/rdmarkerset/rdmarkerset.cpp`, `lib/rdtrackerwidget.cpp`,
and the schema-migration call sites in `utils/rddbmgr/updateschema.cpp`
— never touches `FORCED_LENGTH` at all. The only code that does is
`rdlibrary/edit_cart.cpp`'s `EditCart::okData()` (~line 794-807) and
`rdlibrary/audio_cart.cpp`'s save path, each of which calls
`setForcedLength()` itself, separately, immediately after calling
`updateLength()` — which is the entire reason manually opening a cart in
RDLibrary and pressing OK "fixes" the displayed length: it's that extra,
RDLibrary-only bookkeeping doing it, not `updateLength()`.

## Implementation plan

All changes confined to `RDCart::updateLength(bool,unsigned)`
(`lib/rdcart.cpp`). Restore a `!enforce_length`-guarded `setForcedLength()`
call in each of the three existing write branches, using the value
already being written to `AVERAGE_LENGTH` in that same branch:

1. **Active-cuts branch** (`active_len>0`): after the existing
   `AVERAGE_LENGTH`/`AVERAGE_SEGUE_LENGTH`/`AVERAGE_HOOK_LENGTH` update,
   add `if(!enforce_length) { setForcedLength(active_len/active_cuts); }`.
2. **Evergreen branch** (`evergreen_found`): same pattern, using
   `evergreen_len/evergreen_cuts`.
3. **"Nothing playable" branch**: `if(!enforce_length) { setForcedLength(0); }`.

`enforce_length` is the function's own first parameter — when true (a
cart with "Enforce Length" checked in RDLibrary), automated recompute
must not overwrite an operator's deliberately pinned value, matching
the existing guard `EditCart::okData()` uses for its own direct
`setForcedLength()` call.

## Confirmed out of scope

- The Macro-cart and "ambiguous type" early-return branches at the top
  of `updateLength()` — neither has ever set `FORCED_LENGTH` (macros
  don't carry cut audio, and the ambiguous-type case is a logged
  warning path), and upstream's pre-refactor code didn't either.
- `AVERAGE_SEGUE_LENGTH`/`AVERAGE_HOOK_LENGTH` and the accumulator
  overwrite-vs-accumulate bug — both already fixed on branch
  `fix-average-segue-length-semantic` (spec-less, see `CHANGELOG.md`),
  unrelated to this field.

## Verification plan

1. Build clean.
2. Import a new cart via `rdimport` (or the web import path): confirm
   RDLibrary's browse grid shows its real length immediately, without
   needing to open the cart first.
3. A cart with "Enforce Length" checked and a manually-set forced
   length: re-import/re-verify its cuts and confirm the forced length is
   *not* silently overwritten.
4. A cart whose last cut is removed (falls into "nothing playable"):
   confirm its displayed length goes to zero rather than showing a
   stale value.
