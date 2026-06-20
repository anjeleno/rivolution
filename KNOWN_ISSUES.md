# Known Issues

Practical limitations you'll hit running this fork today, what causes
them, and what to do about it. For the technical "why hasn't this been
fixed yet" detail, see `BACKLOG.md`.

## Submitted mixes must be encoded at the system's sample rate

**Symptom:** an imported MP3 plays back pitch-shifted ("helium"
voice) or sped up.

**Cause:** MP3 passthrough only activates when the source file's real
sample rate matches the system's configured output rate (48kHz by
default). If it doesn't match, the import correctly falls back to a
full re-encode instead of passthrough — but if a mismatched-rate MP3
ever ends up in the library some other way, playback of that file will
be pitch-shifted, because `caed`'s MPEG playback path doesn't resample
mismatched-rate audio (see `BACKLOG.md`).

**Workaround:** make sure anyone submitting a mix or file for import
encodes it at the system's sample rate (48kHz unless you've configured
otherwise). This isn't enforced automatically yet.

## Very long imports/exports can silently waste server resources

**Symptom:** an import or export of a large/long file shows a client-
side error after about 20 minutes, but the station otherwise seems
fine.

**Cause:** the underlying HTTP transfer times out client-side after 20
minutes, but the server isn't told to stop and keeps converting the
file in the background regardless, fully orphaned. This is pre-existing
upstream Rivendell behavior, not something this fork introduced.

**Workaround:** keep individual imports/exports well under 20 minutes
where practical. If you suspect a stuck conversion, check `ps aux` on
the server for a long-running `rdxport.cgi` process consuming CPU and
kill it manually; any partial cart/cut it left behind should be deleted
through RDLibrary (not raw SQL) to keep bookkeeping consistent.

## Log Exception Report shows "is not playable" for kill-dated carts

**Symptom:** generating a log for a date after a cart's kill date has
passed produces validation exceptions like
`cart 064721 [...] is not playable`, repeated for every slot the
scheduler tried to place it in — rather than the scheduler simply
skipping that cart and rotating in another active one from the same
category.

**Cause:** the log scheduler isn't excluding kill-dated carts from
rotation before generating the log. Suspected regression (see
`BACKLOG.md`) — flagged high priority, not yet fixed.

**Workaround:** before a cart's kill date arrives, manually remove or
replace it in its rotation category rather than relying on the
scheduler to exclude it automatically. If a log was already generated
with the exceptions present, replacing the cart and regenerating the
log clears that specific report — but the underlying rotation will keep
selecting any other not-yet-removed expired cart the same way.

## No waveform/peak display for MP3 cuts

**Symptom:** carts containing MP3-format audio show no waveform in
RDLibrary, making it hard to place segue/talk markers precisely.

**Cause:** `RDWaveFile::LoadEnergy()` has no decode path for MPEG Layer
III — pre-existing upstream limitation, not a regression in this fork.
Real MP3 peak computation is planned but not yet built.

**Workaround:** none currently — place markers by ear/timestamp until
this is built.
