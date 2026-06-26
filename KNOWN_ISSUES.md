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
system's DocBook stylesheet tree at `./configure` time, if
`$DOCBOOK_STYLESHEETS` is set in the environment at that moment. A
previous fix made `configure_build.sh` export that variable
automatically for Debian/Ubuntu, which does make the symlink itself get
created correctly — but `docs/apis`, `docs/manpages`, `docs/dtds`, and
`docs/rivwebcapi`'s `Makefile.am` rules never actually referenced that
symlink; they referenced the raw `$(DOCBOOK_STYLESHEETS)` environment
variable directly, at *make* time, not configure time. `export` inside
a script only affects that script's own process and its children —
once `configure_build.sh` exits, the variable is gone from the
invoking shell, so any later, separate `make` invocation (even chained
in the same command line with `&&`) never sees it, and the path
resolves to a bare `/fo/docbook.xsl` with no prefix at all. Easy to
miss if a build machine already has a copy of this stylesheet sitting
somewhere the variable happens to still be set from an earlier,
separate `source`d session — the build works by coincidence there,
exactly the kind of masking this doc exists to flag.

**Fixed for real:** the four `Makefile.am` files now reference
`$(top_srcdir)/helpers/docbook` directly instead of the environment
variable, so the doc build depends only on the symlink created once at
configure time — a real file on disk, not shell state — and works
regardless of how `make` is invoked or in how many separate shell
sessions. Confirmed by rebuilding with `DOCBOOK_STYLESHEETS` explicitly
unset. Still not fixed for RHEL, since the symlink itself is never
created there in the first place — see `BACKLOG.md`.

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
`BACKLOG.md`). Every later `make install` done correctly via
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
automated — see `BACKLOG.md`.

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
