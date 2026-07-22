# Segue Back-Timing

How Rivolution decides *when* to start the next log element during a
segue, so a produced element's dry/walkable tail lands under the next
song's intro instead of colliding with its vocal — and what the
markers involved actually mean.

## The short version

When an imaging element is set to segue into a song, Rivolution used to always start the next element the instant it hit its own segue-start
marker, with zero regard for how much instrumental intro the next
element had. If the outgoing element still had audio left after that
point and the next element had a short or :00 intro, the result was a 
train wreck: the next element's vocal would start while the outgoing element's tail was still playing.

Segue back-timing fixes this by automatically calculating the difference and delaying the trigger, when needed, so the previous element walks the available intro and hits the post without stepping on the vocal. 

## To enable Segue Back-timing:

1. Open RDLibrary
2. Open an **Imaging cart:**
- Sweeper
- Promo
- Legal ID
- etc.
3. Select a cut/s inside the cart
4. Tap the "Edit Markers" button
5. **Confirm that "No fade on segue out" is checked. Check it [x] if it's unchecked.**

## "No fade on segue out"

When this option is checked on Imaging Elements:

- Segue back-timing is active for that transition.
- The outgoing element plays out completely to its own
  natural end — no fade, no early cutoff, regardless of where the
  segue-end marker sits. A reverb tail or trailing echo past segue-end
  plays out in full, on air, under whatever comes next.

NOTE: If that box is unchecked, none of this applies — Rivolution falls back to the classic behavior (fire at segue-start, fade the outgoing element out) and segue Back-timing is ignored. 

## The markers involved

Every cart has a **segue marker** — a start point and an end point.
Every song can additionally have an **intro marker** — also a start and
an end point, labeled "Talk Start"/"Talk End" in RDLibrary (a naming
holdover from Rivendell upstream; this document calls them **intro
marker 1** and **intro marker 2**, which is what they actually mean to
an operator).

- **Segue start** — where the *next* element should be triggered, if
  there's enough intro (or back-time) available.
- **Segue end** — the end of *usable audio* on the outgoing element —
  not necessarily the end of the file. For example: A sweeper might have
  several seconds of reverb tail or an echoing word past this point
  that's meant to be preserved on air, not trimmed.
- **Intro marker 1** — the first post where an intro *could* start being  talked over. In practice this is largely vestigial today; see the gotcha below.
- **Intro marker 2** — the real one. This is where the vocal actually
  starts. **This is the number segue back-timing uses as "how much
  intro does the next element have."**

> [!IMPORTANT]
> **Intro marker 1 must actually be set — don't leave it sitting at the
> very beginning of the file out of habit.** The back-timing
> calculation reads intro marker 1's position as the next element's
> intro length. If you only ever set intro marker 2 (the real
> vocal-start point) and leave marker 1 untouched at `:00`, the math
> will treat the song as having a cold, zero-length intro every time —
> because as far as the calculation is concerned, that's what a marker
> 1 sitting at `:00` *means*. Move marker 1 to reflect the song's actual
> intro length.
>
> RDLibrary's "Talk" column always displays intro marker 2's time (the
> final, vocal-start marker) regardless of where marker 1 sits — so
> don't use that column to judge whether marker 1 has been set
> correctly. Check the waveform editor directly.

## The three segue types

Example: a 12-second produced sweeper. First half (`:00`–`:06`)
is fully produced; second half (`:06`–`:12`) is dry and can walk up a
song's intro. Segue start marker is at `:06`, segue end marker is at `:12`. "No fade on segue out" is checked.

### Type 1 — the next song has a cold (`:00`) intro

Back-timing looks ahead at the next song and sees no real intro (no intro markers set assumes `:00` cold vocal intro). Since there's no room to walk up the intro without violently slamming into the vocal immediately, it holds off triggering the next song until segue end (`:12`) — the end of the sweeper's *usable* audio. Because "no fade" is checked, any reverb tail past `:12` keeps playing under the new song rather than getting cut off.

### Type 2 — the next song has a `:04` intro

Back-timing knows to wait until `:08` (segue-start `:06` + a `:02`
delay) to trigger the next song, so its `:04` of intro plays out and
lands the vocal right at `:12` — the sweeper's segue end — with no
collision.

### Type 3 — the next song has a `:20` intro

Plenty of intro to spare. Segue start fires the next song immediately
at `:06`, no back-time delay needed at all — the song's own long intro
easily covers whatever's left of the sweeper.

## Reference

- Full technical background and implementation:
  [`docs/specs/0002-segue-backtiming.md`](https://github.com/anjeleno/rivolution/blob/main/docs/specs/0002-segue-backtiming.md)
- Relevant fix history: [Changelog](https://github.com/anjeleno/rivolution/blob/main/CHANGELOG.md), 2026-07-07 entries
