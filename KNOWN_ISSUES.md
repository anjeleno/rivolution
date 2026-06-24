# Known Issues

Practical limitations you'll hit running this fork today, what causes
them, and what to do about it. For the technical "why hasn't this been
fixed yet" detail, see `BACKLOG.md`.

## AudioScience hardware (HPI/HPK) unsupported on Ubuntu 26.04

**Symptom:** stations with AudioScience professional audio adapters
can't build or run with HPI support on Ubuntu 26.04 — `hpklinux-dev`
(and the underlying kernel driver) isn't installable at all on this
release. This is a real blocker, not a minor gap, for anyone relying
on that hardware specifically.

**Cause:** the `hpklinux`/`hpklinux-dev` package comes from a repo
that currently only publishes builds for 22.04 ("jammy") and 24.04
("noble") — no 26.04 ("resolute") build exists there yet. AudioScience's
own underlying driver/SDK is still actively maintained directly by
AudioScience — this is specifically a packaging gap for this one
Ubuntu release, not the hardware or driver being abandoned.

**Workaround:** build AudioScience's driver directly from their own
published source tarball (audioscience.com) against the 26.04 kernel
headers until `hpklinux-dev` is available for this release. Stations
without AudioScience hardware aren't affected at all — `./configure
--disable-hpi` builds the rest of Rivendell normally. No verified,
tested step-by-step procedure exists yet for the from-source driver
build on 26.04 specifically; treat this as unverified until someone's
actually done it and reported back.

## `docs/` build fails: "cannot parse ../../helpers/docbook/template/titlepage.xsl"

**Symptom:** `make` fails partway through `docs/stylesheets` with:
```
xsltproc -o book-fo-titlepages.xsl ../../helpers/docbook/template/titlepage.xsl book-fo-titlepages-spec.xml
warning: failed to load external entity "../../helpers/docbook/template/titlepage.xsl"
cannot parse ../../helpers/docbook/template/titlepage.xsl
```
on a build that otherwise compiled cleanly.

**Cause:** `configure.ac` creates a `helpers/docbook` symlink to the
system's DocBook stylesheet tree at `./configure` time, but only if
`$DOCBOOK_STYLESHEETS` is set in the environment first — `INSTALL.md`
documents this as required for every Ubuntu section, but
`configure_build.sh` never exported it, so the symlink silently never
got created. `docs/apis`, `docs/manpages`, `docs/dtds`, and
`docs/rivwebcapi` all depend on the same variable and would hit
equivalent failures if their targets are built. Easy to miss on a
build machine that already has a copy of this same stylesheet file
sitting locally outside the symlink's expected path — the build works
by coincidence there, which is exactly what made this gap hard to spot
until a genuinely clean checkout hit it directly.

**Workaround:** export the variable before configuring, or just create
the symlink directly if you're resuming a build that already ran
`./configure` without it:
```bash
ln -s /usr/share/xml/docbook/stylesheet/docbook-xsl-ns helpers/docbook
```
Fixed going forward: `configure_build.sh` now exports
`DOCBOOK_STYLESHEETS` for Debian/Ubuntu automatically. Not yet fixed
for RHEL — see `BACKLOG.md`.

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
