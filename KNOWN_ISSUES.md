# Known Issues

Practical limitations you'll hit running this fork today, what causes
them, and what to do about it. For the technical "why hasn't this been
fixed yet" detail, see [`BACKLOG.md`](https://github.com/anjeleno/rivolution/blob/main/BACKLOG.md).

## AudioScience hardware (HPI/HPK) unsupported on Ubuntu 26.04

**Symptom:** stations with AudioScience professional audio adapters
can't build or run with HPI support on Ubuntu 26.04 — `hpklinux-dev`
(and the underlying kernel driver) isn't available on this
release yet. This is a real blocker, not a minor gap, for anyone relying
on that hardware specifically.

**Cause:** no prebuilt `hpklinux`/`hpklinux-dev` `.deb` has been
published for 26.04 ("resolute") yet by any repo known to this project
— builds are only known to be published for 22.04 ("jammy") and 24.04
("noble"). AudioScience's own underlying driver/SDK is still actively
maintained directly by AudioScience — this is a packaging-availability
gap for this one Ubuntu release, not the hardware or driver being
abandoned.

**Workaround:** build the package yourself, directly from the upstream
packaging scripts — maintained by Rivendell's own author at
[`ElvishArtisan/debian-hpklinux`](https://github.com/ElvishArtisan/debian-hpklinux)
(branch `hpi4.20.46`, the newest available as of this writing):

```bash
git clone -b hpi4.20.46 https://github.com/ElvishArtisan/debian-hpklinux.git
cd debian-hpklinux
./prepare_build.sh
cd hpklinux_4.20.46
dpkg-buildpackage -us -uc
```

This produces real, native `hpklinux`/`hpklinux-dev` `.deb` packages
built directly on whichever Ubuntu release you run it on — confirmed
by reading `debian/control.src` and `debian/postinst.src` directly,
there's nothing 24.04-specific baked into a prebuilt kernel module: the
package's own `postinst` runs `dkms build`/`dkms install` against the
*installing machine's actual running kernel*, not a kernel pinned to
whatever box originally built the `.deb`. So there's no real need to
go hunting for a Noble-built package and hope it's binary-compatible
with 26.04 — building straight on the 26.04 box, the same way it
already works on 22.04/24.04, should work in principle.

**Genuinely unverified beyond that** — this is based on reading the
packaging scripts, not on actually running them on real hardware. The
package's own declared dependencies (`build-essential`, `dkms`,
`linux-headers`, `devscripts`, `debhelper`, `ubuntu-dev-tools`) are all
ordinary packages with no known 26.04 availability concern, but
whether AudioScience's bundled driver source actually compiles cleanly
via DKMS against a 26.04 kernel has not been confirmed by anyone — that
needs a real AudioScience card and a real 26.04 box, exactly the kind
of thing this project needs testers for. Stations without AudioScience
hardware aren't affected at all either way — `./configure
--disable-hpi` builds the rest of Rivendell normally.

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
keep refreshed automatically. See [`BACKLOG.md`](https://github.com/anjeleno/rivolution/blob/main/BACKLOG.md) for the install-prefix
question this is one concrete symptom of.

**Workaround:** run `sudo ldconfig` once after every `make install`.
Not yet automated as part of the install target itself.

## A stale `/usr/local`-prefix install can silently shadow a working `/usr` one

**Symptom:** after a clean `sudo make install` to the correct prefix
(`/usr`, via `configure_build.sh`), things still behave like an older
build — `which rdimport` (or any other Rivendell binary) resolves to
`/usr/local/bin/rdimport` instead of `/usr/bin/rdimport`; freshly
swapped launcher icons don't show up; a real code fix doesn't seem to
take effect at all even though the build succeeded.

**Cause:** if `./configure` was ever run directly at some earlier
point — bypassing `configure_build.sh`'s `--prefix=/usr` — that build
installed a complete, parallel copy of every Rivendell binary, library,
and icon at the autotools default prefix, `/usr/local` (see
[`BACKLOG.md`](https://github.com/anjeleno/rivolution/blob/main/BACKLOG.md)). Every later `make install` done correctly via
`configure_build.sh` only adds/updates files at `/usr`; it never
touches or removes that old `/usr/local` tree, since `make install`/
`uninstall-local` only manage whichever prefix the build is currently
configured for. Both `$PATH` (`/usr/local/bin` before `/usr/bin` by
default on Debian/Ubuntu) and the XDG icon theme search order
(`/usr/local/share/icons` checked before `/usr/share/icons`) prefer the
stale copies, so the old build silently keeps winning.

**Workaround:** check directly — `which rdimport` should print
`/usr/bin/rdimport`; if it prints `/usr/local/bin/rdimport` instead, a
shadow install exists. Confirm with `stat -c "%y %n" /usr/bin/rdimport
/usr/local/bin/rdimport` (the `/usr/local` copy will have a visibly
older mtime). There's no `make uninstall` that can clean this up after
the fact, since the build is now configured for a different prefix
than the stale install used — remove it by hand. A full stale install
spans: `/usr/local/{bin,sbin}/{rd*,rivendell_filter}`,
`/usr/local/lib/{librd*,librdalsa*}`, `/usr/local/libexec/` (entirely
Rivendell's, safe to remove as a whole directory),
`/usr/local/share/{pixmaps/rivendell,rivendell}/` (also entirely
Rivendell's), `/usr/local/share/icons/hicolor/*/apps/{rivendell,rdadmin,
rdairplay,rdcartslots,rdcastmanager,rdcatch,rdlibrary,rdlogedit,
rdlogmanager,rdpanel}.png` (only the specific Rivendell-named files —
this directory is shared with other locally-installed software),
`/usr/local/share/X11/fvwm2/pixmaps/{rivendell,mini.rivendell}.xpm`,
and `/usr/local/etc/rd-bin.conf`. Run `sudo ldconfig` afterward to drop
the removed `librd*.so` entries from the linker cache. Not yet
automated — see [`BACKLOG.md`](https://github.com/anjeleno/rivolution/blob/main/BACKLOG.md).

## Missing `mp3gain` makes MP3 imports silently much slower, not broken

**Symptom:** importing and normalizing an MP3 still works, but takes
noticeably longer than expected, with no error — easy to mistake for
an unrelated performance problem rather than a missing package.

**Cause:** `mp3gain` is a runtime-only dependency, never linked
against at build time — it's invoked at import time to apply MP3
loudness normalization directly to the compressed bitstream. Without
it, normalization still happens, but falls back to a full decode and
re-encode instead of the fast bitstream-level patch.

**Workaround:** install `mp3gain` (see the build dependency list in
[Build From Source](https://github.com/anjeleno/rivolution/wiki/Build-From-Source)).

## Submitted mixes must be encoded at the system's sample rate

**Symptom:** an imported MP3 plays back pitch-shifted ("helium"
voice) or sped up.

**Cause:** MP3 passthrough only activates when the source file's real
sample rate matches the system's configured output rate (48kHz by
default). If it doesn't match, the import correctly falls back to a
full re-encode instead of passthrough — but if a mismatched-rate MP3
ever ends up in the library some other way, playback of that file will
be pitch-shifted, because `caed`'s MPEG playback path doesn't resample
mismatched-rate audio (see [`BACKLOG.md`](https://github.com/anjeleno/rivolution/blob/main/BACKLOG.md)).

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
[`BACKLOG.md`](https://github.com/anjeleno/rivolution/blob/main/BACKLOG.md)) — flagged high priority, not yet fixed. Recurred again
2026-06-21 with a second promo; that occurrence also raised a separate
concern that sequential rotation itself isn't cycling members in order
correctly — see [`BACKLOG.md`](https://github.com/anjeleno/rivolution/blob/main/BACKLOG.md) for both.

**Workaround:** before a cart's kill date arrives, manually remove or
replace it in its rotation category rather than relying on the
scheduler to exclude it automatically. If a log was already generated
with the exceptions present, replacing the cart and regenerating the
log clears that specific report — but the underlying rotation will keep
selecting any other not-yet-removed expired cart the same way.

## RDAirplay plays silence for a cart with a missing audio file, instead of skipping it

**Symptom:** a cart whose database record exists but whose actual
audio file is missing from `/var/snd` (never imported successfully,
deleted out-of-band, etc.) plays dead air for its full listed duration
during a live log, instead of being skipped. Reported as a regression:
previous behavior was to skip such a cart automatically and move on to
the next log element immediately, with no dead air.

**Cause:** RDAirplay's playout logic treats "a cut record exists" as
"this cart has audio," with no check that the backing file is actually
present on disk. Not yet investigated in detail — see [`BACKLOG.md`](https://github.com/anjeleno/rivolution/blob/main/BACKLOG.md).

**Workaround:** none yet. If you suspect a cart has a missing audio
file, check `/var/snd` against its cut record manually before it's
scheduled into a live log.

## Manually added carts may lose their audio even after clicking OK on Edit Cart

**Symptom:** a cart added via RDLibrary's "Add" button, with audio
successfully imported, can have both its database row and its audio
file in `/var/snd` disappear after the Edit Cart dialog closes —
including cases where OK was the button actually clicked, not Cancel,
Escape, or the window's X.

**Cause:** not confirmed. A related issue was found and fixed —
closing the dialog any way *other* than OK could delete an
already-imported cart's audio — but that fix doesn't account for loss
following a confirmed OK click, and no mechanism for that has been
identified yet. Suspected to be a Qt6-migration regression, not yet
bisected to confirm. See [`BACKLOG.md`](https://github.com/anjeleno/rivolution/blob/main/BACKLOG.md) for what's been ruled out
so far.

**Workaround:** none yet. After using "Add," confirm the new cart's
audio actually persisted (waveform visible in "Edit Markers," or the
cart plays) before relying on it.

## Clicking Cancel on Edit Cart after importing audio doesn't actually cancel

**Symptom:** if you import audio into a newly-added cart via "Add,"
then click Cancel instead of OK, the cart is kept anyway — its
database row and audio file both persist, the opposite of what Cancel
implies.

**Cause:** the fix for a related bug (already-imported audio being
destroyed by an *incidental* dialog close, like Escape or the window's
X) preserves any cart with real, already-imported audio regardless of
how the dialog closed — which also covers an explicit Cancel click,
not just the incidental closes it was meant to protect against. See
[`BACKLOG.md`](https://github.com/anjeleno/rivolution/blob/main/BACKLOG.md) for the maintainer-facing detail.

**Workaround:** if you click Cancel after importing audio and want the
cart actually discarded, delete it manually afterward through
RDLibrary (not raw SQL, to keep bookkeeping consistent).

## Edit Markers waveform goes blank/truncated on long cuts at high zoom — fixed 2026-07-03, pending real-world confirmation

**Symptom:** in "Edit Markers," zooming in on a long cut could show
blank space or a truncated waveform instead of the actual audio,
making it hard to place a segue marker precisely.

**Cause:** the waveform display rendered an entire cut into a single
image sized to fit the whole file at the current zoom level; long
cuts at high zoom needed an image wider than the display toolkit
supports, silently corrupting the image past that width. Long-standing
Rivendell v4 behavior, not introduced by this fork (see
[`BACKLOG.md`](https://github.com/anjeleno/rivolution/blob/main/BACKLOG.md)
for the technical detail).

**Fix:** the waveform is now rendered as a strip of bounded-width tiles
instead of one oversized image, and the artificial zoom-level cap this
issue's earlier workaround relied on has been removed — zoom is no
longer limited by cut length at all.

**Workaround (if still seen after updating):** zoom out two or three
steps from maximum when placing markers near the end of a file.

## Waveform zoom has a precision floor of about 26 milliseconds

**Symptom:** zooming in on a cut's waveform past a certain point in
"Edit Markers" doesn't reveal any finer detail — the same shape just
stretches across more screen pixels rather than showing more of the
actual waveform.

**Cause:** the waveform display draws from pre-computed peak data
sampled in fixed ~26-millisecond blocks, not the raw audio itself.
That's fine for placing most markers, but it's a real precision limit
for edits that need finer-than-26ms accuracy. Deferred — see
[`BACKLOG.md`](https://github.com/anjeleno/rivolution/blob/main/BACKLOG.md)
for the technical detail and what a real fix would require.

**Workaround:** none currently; place markers with the understanding
that sub-26ms accuracy isn't achievable in the waveform display yet.
