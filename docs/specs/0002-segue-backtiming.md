# 0002 — Segue back-timing

**Date:** 2026-06-17

## Goal

Today, when a playing element (A) reaches its segue marker, Rivendell
fires the next element (B) immediately — at that exact instant, with no
regard for how much intro (vocal-free lead-in) B has. If A still has
several seconds of audio left after its segue point and B has a short
or zero intro, the result is a collision: B's vocal starts while A's
tail is still playing.

The fix is to delay firing B, when needed, so B's vocal lands exactly
when A's tail finishes — the same calculation a human DJ does manually
when backtiming a segue:

```
delay = max(0, A's remaining tail after the segue point − B's intro length)
```

This only applies when A has "No fade on segue out" checked — see
**Gate condition** below. If A is meant to fade/duck as B comes in, the
existing fade behavior is what's wanted and back-timing must not
interfere with it.

## Background — verified against source, not assumed

### Trigger chain (today, no back-timing)

1. `RDPlayDeck::StartTimers()` (`lib/rdplay_deck.cpp:760-895`) arms a
   timer for A's segue-start point when A begins playing.
2. When that timer fires, `pointTimerData()`
   (`lib/rdplay_deck.cpp:658-701`) emits `segueStart(play_id)`.
3. `RDLogPlay::segueStartData()` (`lib/rdlogplay.cpp:1552-1577`) handles
   it — this is the actual trigger, and it fires B **synchronously,
   immediately**, with zero delay:
   ```cpp
   StartEvent(play_next_line,next_logline->transType(),
              logline->segueTail(next_logline->transType()),
              RDLogLine::StartSegue,-1,
              logline->segueTail(next_logline->transType()));
   SetTransTimer();
   ```
   This only runs when `play_op_mode==RDAirPlayConf::Auto`,
   `next_logline->transType()==RDLogLine::Segue`, and `logline` (A) is
   currently `Playing` — back-timing must preserve all three guards
   unchanged, it only changes *when* `StartEvent()`/`SetTransTimer()`
   run within that same branch.

### Data already available — no schema change needed

- **B's intro length** = `talkStartPoint()` on `RDCut`/`RDLogLine`
  (`lib/rdcut.h:128`, `lib/rdlog_line.h:129`) — literally "where vocals
  start," loaded from the cut into the logline at
  `lib/rdplay_deck.cpp:213-216`. Defaults to `0` for any cut that's
  never had talk markers set — meaning, for most untouched music-library
  carts, B's intro is `0` and the delay formula reduces to "wait the
  full tail." Not a bug, just the existing meaning of an unset marker;
  flagging it here because it determines how often back-timing actually
  changes on-air behavior once "no fade on segue out" is checked.
- **A's remaining tail** = `logline->segueTail(next_logline->transType())`
  (`lib/rdlog_line.cpp:1541`) — `segueEndPoint() - segueStartPoint()`,
  already computed at the exact trigger point above.

### Gate condition: "No fade on segue out"

This is not a separate field — it's `CUTS.SEGUE_GAIN == 0`, exposed via
`RDCut::segueGain()` / `RDLogLine::segueGain()`. Confirmed in
`lib/rdmarkerview.cpp:1264` (`d_no_segue_fade=q->value(11).toInt()==0`)
and in actual playout use at `lib/rdplay_deck.cpp:578-581`, where
`play_point_gain` (`= logline->segueGain()`) gates whether A gets a
`fadeOutputVolume()` call when it stops — when `0`, A is *not* faded,
it plays naturally to the stop point. That's the existing, real meaning
of "no fade on segue out," and it's a property of **A** (the element
segueing *out*), not B — confirming the gate is `logline->segueGain()==0`
on A, exactly as described when this was scoped.

**Flagged for future review:** `segueGain()` is overridable per
scheduled log event (`rdairplay/edit_event.cpp`), not just per-cut in
the library (`lib/rdmarkerplayer.cpp`). The existing fade code already
reads the per-logline (possibly-overridden) *effective* value, not the
raw cut default, and this spec gates back-timing on that same effective
value for consistency. Noting this explicitly so that if back-timing
ever produces a surprising result for a specific scheduled event, this
override path is the first place to check — a per-event override
changing audio-cue timing automatically, with no visible indicator in
the log other than the existing fade behavior, is a real
"action-at-a-distance" risk worth being aware of.

### Voice tracking — checked, not directly reusable

Voice tracking (`lib/rdtrackerwidget.cpp:880-894`) does **not** do
runtime back-timing. After a DJ finishes recording a voice track, it
statically *repositions* the next song's segue marker once:
```cpp
int segue_start = endPoint - startPoint - talkStartPoint();
setSegueStartPoint(segue_start, ...);
```
This computes the same kind of "land at the vocal start" math, but at
edit time, on a fixed marker — not dynamically at trigger time against
whatever A's actual remaining tail turns out to be. Confirms the
formula's direction is right; not reused as a mechanism. Back-timing is
implemented fresh in the playout trigger path instead.

## Implementation plan

All changes confined to `lib/rdlogplay.cpp`, specifically
`RDLogPlay::segueStartData()`. **No changes to `RDPlayDeck` or its timer
arming logic** — `StartTimers()` continues to fire the segue-start
signal exactly when it does today; only what happens in response to
that signal, inside `RDLogPlay`, changes.

1. In `segueStartData()`, after the existing guards (op mode, transition
   type, A's status) and before calling `StartEvent()`, compute:
   ```cpp
   int delay = 0;
   if(logline->segueGain()==0) {  // "No fade on segue out"
     delay = logline->segueTail(next_logline->transType()) -
             next_logline->talkStartPoint();
     if(delay<0) {
       delay = 0;
     }
   }
   ```
2. If `delay==0`, call `StartEvent()` + `SetTransTimer()` exactly as
   today — zero behavior change for every cart that doesn't have "no
   fade on segue out" set, and for any case where B's intro already
   covers A's tail.
3. If `delay>0`, defer the existing `StartEvent()`/`SetTransTimer()`
   call pair via a one-shot timer (`QTimer::singleShot` or an owned
   `RDLogPlay` member timer) instead of calling immediately. Confirmed
   `SetTransTimer()` (`lib/rdlogplay.cpp:2654`) is unrelated bookkeeping
   for *hard-clock-timed* events elsewhere in the log — it doesn't
   depend on whether B has started, so it's safe to defer alongside
   `StartEvent()` without a separate timing concern.
4. **Stale-state guard, required:** by the time the deferred callback
   fires, the world may have changed — operator switched out of Auto
   mode, hit Stop/Skip on A, or otherwise intervened during the delay
   window. The deferred callback must re-validate the same preconditions
   `segueStartData()` already checks today (op mode still `Auto`, A's
   `logline` still `Playing`, `id()!=-1`, B still resolvable via
   `nextEvent()`/`GetNextPlayable()`) before calling `StartEvent()` —
   exactly mirroring today's guards, just re-checked at fire time
   instead of assumed to still hold.

## Confirmed out of scope

- Only applies inside the existing `next_logline->transType()==
  RDLogLine::Segue` branch — hard `Play`/`Stop` transitions are
  untouched, as they are today.
- No database schema changes — `segueGain()`, `segueTail()`, and
  `talkStartPoint()` all already exist and are already loaded into
  `RDLogLine` during normal playout.
- No RDAdmin/UI changes — "No fade on segue out" is an existing control
  (`lib/rdmarkerplayer.cpp`); this spec only changes what playout *does*
  in response to that flag already being set, not how it's edited.
- Voice tracking's marker-repositioning behavior is untouched.

## Correction (2026-07-07): the "Gate condition" section above was wrong about what "no fade" actually did

The Background section's claim that `play_point_gain==0` meant A "is
*not* faded, it plays naturally to the stop point" was true as far as
it went, but "the stop point" was assumed to mean A's own natural end
— it doesn't. Two call sites unconditionally stop A right at its
segue-end marker regardless of `segueGain()`:
`RDPlayDeck::pointTimerData()`'s `Segue` case, and
`RDLogPlay::StartEvent()`'s `Segue` branch. Only the fade *curve* was
ever gated on the flag; the stop itself never was. So back-timing, as
originally implemented per this spec, correctly delayed firing B, but
A was still being truncated at its own segue-end the whole time,
independent of that delay — silently defeating the point whenever a
cut's segue-end sits meaningfully before its actual audio end (e.g. a
produced element with a trailing reverb tail). Both call sites now also
skip the stop when `segueGain()==0`, letting A run out via its ordinary
natural-completion path instead. See `CHANGELOG.md` (2026-07-07) and
`ARCHITECTURE.md`'s "a flag gating one side-effect of a compound action
doesn't gate the others" for the full mechanism.

## Open items for implementation time

- Re-confirm `segueStartData()`'s exact guard conditions immediately
  before implementing, in case anything has changed since this spec was
  written.
- Decide the concrete deferred-call mechanism (a dedicated one-shot
  `QTimer` member on `RDLogPlay` vs. `QTimer::singleShot`) based on
  whatever's idiomatic for the rest of `RDLogPlay`'s existing timer
  members (`play_trans_timer`, etc.) — match existing style rather than
  introducing a new pattern.
