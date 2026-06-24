# Known Issues

Practical limitations you'll hit running this fork today, what causes
them, and what to do about it. For the technical "why hasn't this been
fixed yet" detail, see `BACKLOG.md`.

## Binaries fail with "cannot open shared object file" after a fresh install

**Symptom:** right after `sudo make install`, every Rivendell binary
(`rdadmin`, `rdairplay`, etc.) fails immediately:
```
error while loading shared libraries: librd-6.0.0int0.so: cannot open shared object file: No such file or directory
```
even though the file genuinely exists at `/usr/local/lib/librd-*.so`.

**Cause:** `/usr/local/lib` is in the linker's configured search path
(`/etc/ld.so.conf.d/libc.conf`), but `make install` doesn't refresh the
linker's *cache* (`ldconfig`) after installing a new shared library
there — unlike `/usr/lib`, which most package-manager-driven installs
keep refreshed automatically. See `BACKLOG.md` for the install-prefix
question this is one concrete symptom of.

**Workaround:** run `sudo ldconfig` once after every `make install`.
Not yet automated as part of the install target itself.

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
`BACKLOG.md`) — flagged high priority, not yet fixed. Recurred again
2026-06-21 with a second promo; that occurrence also raised a separate
concern that sequential rotation itself isn't cycling members in order
correctly — see `BACKLOG.md` for both.

**Workaround:** before a cart's kill date arrives, manually remove or
replace it in its rotation category rather than relying on the
scheduler to exclude it automatically. If a log was already generated
with the exceptions present, replacing the cart and regenerating the
log clears that specific report — but the underlying rotation will keep
selecting any other not-yet-removed expired cart the same way.

## Edit Markers waveform goes blank when zoomed in fully near end-of-file

**Symptom:** in "Edit Markers," zooming all the way in while positioned
near the end of a cut shows blank space instead of the waveform,
making it hard to place a segue marker precisely at a file's tail.

**Cause:** long-standing Rivendell v4 behavior, not introduced by this
fork — not yet investigated in detail (see `BACKLOG.md`).

**Workaround:** zoom out two or three steps from maximum when placing
markers near the end of a file.
