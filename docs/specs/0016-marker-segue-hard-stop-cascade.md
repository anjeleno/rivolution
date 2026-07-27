# 0016 — Marker-cascade hard-stop can kill a still-legitimately-playing element

**Date:** 2026-07-26
**Status:** Implemented on branch `fix-marker-segue-hard-stop-cascade`.
Builds clean; not yet run, tested against the verification plan below,
or merged.

## Goal

Audio currently playing (a song fading out, or an imaging element
deliberately left running via "no fade on segue out") can be cut off
mid-play the instant a `Marker`-type log line configured "Timed, Make
Next" is reached via a Segue transition from the preceding element —
even though neither the Marker's own Hard-time setting nor the outgoing
element's own fade/segue configuration are ever consulted in that
specific path. The fix makes the auto-advance mechanism that causes this
wait for anything still legitimately running to actually finish before
force-starting the next real content.

## Background — verified against source, not assumed

### Two independently-clocked triggers can reach the same log line

1. **Position-based (segue chain):** `RDPlayDeck::pointTimerData()`
   (`lib/rdplay_deck.cpp:658`)'s `Segue` case fires purely off that
   deck's own audio position — armed by `StartTimers()` when the deck
   started playing, counting down to that cut's own `SEGUE_START_POINT`.
   Zero reference to any other line's wall-clock schedule. Fires
   `segueStart()` → `RDLogPlay::segueStartData()`
   (`lib/rdlogplay.cpp:1558`) → `StartEvent()` for whatever's next.
2. **Wall-clock (Hard-time scheduling):** `play_trans_timer`, armed by
   `SetTransTimer()` to fire at the next Hard-timed line's logged start
   time. Its handler, `RDLogPlay::transTimerData()`
   (`lib/rdlogplay.cpp:1438`), branches on `graceTime()` (verified
   against the UI mapping in `lib/rdlogeventdialog.cpp:205-213` and both
   `edit_event.cpp` copies): `0` = "Start Immediately" (falls through to
   an immediate, ungated `StartEvent(...,RDLogLine::Play,...)`, called
   **directly**, never through `FinishEvent()`); `-1` = "Make Next"
   (calls `makeNext()`, which only repoints `play_next_line` and forces
   nothing itself); a positive value = "Wait up to" *N* ms.

### Why a `Marker`-type line specifically triggers this

`StartEvent()`'s `Marker`/`Track`/`MusicLink`/`TrafficLink` case
(`lib/rdlogplay.cpp:2220-2247`) marks the line `Finished` and calls
`FinishEvent()` **synchronously, in the same call** — these types carry
no audio, so there's nothing to wait for. `FinishEvent()`
(`lib/rdlogplay.cpp:2493-2511`) immediately calls `GetNextPlayable()`
and force-starts whatever comes after with an unconditional `Play`-type
transition (`lib/rdlogplay.cpp:2009-2030`), which — unlike the `Segue`
branch — has no `segueGain()` gate at all.

A `Marker` configured "Timed, Make Next" only ever gets its Hard-time
wall-clock protection when reached by falling off the natural end of the
log. When reached via a Segue transition instead, that wall-clock gate
is never consulted — whatever moment the preceding line's own segue
point fires, the Marker fires right then, instantly self-finishes, and
hard-cuts whatever's running next. A real `Cart`-type line never
triggers this — `StartEvent()`'s `Cart` case just starts playing and
returns, no synchronous `FinishEvent()` cascade regardless of trigger.

`FinishEvent()` has four call sites total (`lib/rdlogplay.cpp:1728,
2204, 2245`, and via `Finished()` at `:3043`) — the Marker-cascade path
is only one of them; the other three share the identical risk.
`Finished()`'s own call site already has a narrower, unrelated guard
(skip `FinishEvent()` if the *next specific line* is already
Playing/Finishing, added 2026-07-07) that solves a different problem
and is left untouched by this spec.

`FinishEvent()`'s own auto-advance is gated to `play_op_mode==Auto`
only (`lib/rdlogplay.cpp:2502`), and `StartEvent`'s hard-stop block is
itself gated to `play_op_mode!=Manual` (`:2007`) — so the fix below only
ever matters in Auto mode, where an overlapping running line is
essentially always a genuine segue/transition artifact, not an unrelated
manual action. `TRANSPORT_QUANTITY` is `12`
(`lib/rdlogplay.h:47`) — cheap to scan.

### Historical origin (`v4.1.3..v4.4.1` bisection against upstream)

Two commits, five days apart, both explicitly about segue clipping:

- **`254b9bdc` (2024-08-25):** added a "remove degenerate segue
  transitions" rule at the top of `StartEvent()` — any `Segue`-type
  transition with `trans_length<=0` is coerced into `Play`. Same commit
  removed `FinishEvent()`'s prior behavior of preserving the
  just-finished line's own transition type (Play or Segue) when
  auto-advancing; after this commit it's hardcoded `Play`
  unconditionally — the direct origin of the mechanism this spec fixes.
- **`17b6048a` (2024-08-30):** added the actual `stopPlay()` call to
  `RDPlayDeck::pointTimerData()`'s segue-end case (previously a no-op
  debug statement) and the "Setup 'Play Style' Segue" fallback still
  present in current `StartTimers()`.

A later fix (2026-07-07, see `CHANGELOG.md`) gated both of those exact
call sites on `segueGain()`, but never touched `254b9bdc`'s coercion or
`FinishEvent()`'s hardcoded `Play`.

**A second, separate mechanism reaching the same class of symptom
through a completely different call path was found during this same
bisection and is deliberately out of scope here** — see
`BACKLOG.md`'s "'Degenerate segue transitions' coercion..." entry.
Fixing this spec does nothing for that one, and vice versa; they share
a root cause category but not a code path.

## Implementation plan

All changes confined to `lib/rdlogplay.cpp`, specifically
`RDLogPlay::FinishEvent()` (`~line 2493`).

1. Immediately before the existing
   `StartEvent(play_next_line,RDLogLine::Play,0,RDLogLine::StartPlay)`
   call, add a check: if `runningEvents()` shows anything still
   running, skip that `StartEvent()`/`SetTransTimer()` call for this
   invocation instead of forcing it.
   ```cpp
   if(logline->transType()!=RDLogLine::Stop) {
     int lines[TRANSPORT_QUANTITY];
     if(runningEvents(lines)==0) {
       StartEvent(play_next_line,RDLogLine::Play,0,RDLogLine::StartPlay);
       SetTransTimer(QTime(),prev_next_line==play_trans_line);
     }
     // else: something is still legitimately playing (segue tail, in-
     // progress fade). Defer -- its own natural completion will call
     // Finished() -> FinishEvent() again, and this check will pass then.
   }
   ```
2. `UpdateStartTimes()`/`emit stopped(line)` at the end of
   `FinishEvent()` stay unconditional — they concern the line that
   triggered this call, not whether the advance to the next real line
   succeeded.
3. No new timer or state needed. When the still-running line finishes
   naturally, its own `Finished(id)` fires, which — per its existing,
   untouched logic — calls `FinishEvent()` again once its own next-line
   check passes. `FinishEvent()` re-runs `GetNextPlayable()` from the
   same `play_next_line` it left off at (never advanced past the target
   while deferring), and this time `runningEvents()` is empty, so the
   advance proceeds with nothing left to kill.

**Scope: protect any currently-running line, fading out or not — not
just `segueGain()==0` ("no fade") elements.** "No fade on segue out" is
operationally used only on imaging (sweepers/promos/IDs), never songs —
but the actually-reported symptom (e.g.
[ElvishArtisan/rivendell#1022](https://github.com/ElvishArtisan/rivendell/issues/1022),
"songs are cut short") is about songs, which always use a normal
fade-out. A fix scoped to `segueGain()==0` alone would not address the
reported symptom.

## Confirmed out of scope

- `254b9bdc`'s degenerate-segue-transition coercion — see `BACKLOG.md`,
  needs its own investigation into `RDPlayDeck::stop()`'s zero-length
  semantics before any fix is designed there.
- "Start Immediately" (`graceTime()==0`) behavior — confirmed
  structurally unaffected; that path calls `StartEvent()` directly from
  `transTimerData()`, never through `FinishEvent()`.
- `Finished()`'s existing 2026-07-07 guard, the `Segue`-branch's
  `segueGain()` gate, and the segue back-timing feature (spec 0002) —
  all untouched; this is a pure addition inside `FinishEvent()`, nothing
  removed or restructured.
- A separate, lower-priority report (imaging surviving when the songs
  around it get dropped by a Make-Next transition, producing two
  sweepers back to back) — plausibly related to `makeNext()`
  (`lib/rdlogplay.cpp:514`) not transitioning every intervening line's
  status consistently when it overwrites `play_next_line`, but not
  traced line-by-line; needs its own forensic pass before any fix.

## Verification plan

1. Build clean — no new includes or members needed.
2. A clock with a song (normal segue, ordinary fade, no special
   markers) segueing into a `Marker` (Timed, Make Next) followed by
   another song: confirm the first song's fade now completes fully
   instead of being cut short the instant the marker fires.
3. An imaging element ("no fade on segue out" checked) segueing into a
   `Marker` under the same setup: confirm its tail plays out fully.
4. A separate Hard-time event set to "Start Immediately": confirm it
   still cuts off whatever's playing at the exact scheduled clock time,
   completely unchanged.
5. Ordinary back-to-back Auto playback with nothing overlapping at any
   transition: confirm zero behavior change.

## Implementation notes

- `FinishEvent()`'s current form matched this spec's Background section
  exactly at implementation time -- no drift to reconcile.
- Implemented with `runningEvents(NULL)` (the `lines` array is optional
  per its own signature, `lib/rdlogplay.h:105`, and this call site never
  needs the populated array) rather than declaring an unused
  `int lines[TRANSPORT_QUANTITY]`, matching the existing precedent at
  `lib/rdlogplay.cpp:337`.
- Still open: the verification plan below (items 2-5) needs a real build
  and real on-air/test-log listening -- not yet run.
