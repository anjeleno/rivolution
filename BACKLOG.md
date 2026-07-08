# Backlog

Known technical debt and deferred fixes — things we've found, scoped,
and deliberately decided not to fix yet, with the reasoning for why.
This is **not** a feature roadmap or pipeline of planned work; see
[`docs/specs/`](https://github.com/anjeleno/rivolution/tree/main/docs/specs) for that. Entries here get promoted to a real spec and
branch once they're picked up.

## conf/ deployment files need installer and deb packaging integration

The `conf/` directory in this repo is the source of truth for all
system-level configuration files introduced by Rivolution: systemd unit
files and drop-ins, the sudoers rule, and udev rules. None of them are
auto-deployed. On a fresh installation every file must be placed manually
(see [spec 0010 Deployment section](https://github.com/anjeleno/rivolution/blob/main/docs/specs/0010-systemd-stack-orchestration.md#deployment)).
This needs to be automated in two places:

**1. `anjeleno/rivolution-unified-installer`**

A new Ansible role (tentatively `rivapi`) must:
- Build the `rivapi` Go binary (`go build` in `rivapi/`)
- Install the binary to the system path expected by `rivapi.service`
- Copy `conf/sudoers.d/rivapi` → `/etc/sudoers.d/rivapi`, set `chmod 440`
- Create drop-in directories and copy `conf/systemd/*.d/*.conf` files
- Copy `conf/systemd/rivolution-stack.target` and
  `conf/systemd/stereo-tool.service` to `/etc/systemd/system/`
- Copy `conf/udev/99-ptp.rules` to `/etc/udev/rules.d/`
- Run `systemctl daemon-reload`, `udevadm control --reload-rules &&
  udevadm trigger`, `systemctl enable rivolution-stack.target`
- Install and enable `rivapi.service`

**2. Debian package**

The long-term path is a `rivolution` (or `rivolution-rivapi`) deb package
whose `postinst` script handles all the above. Required package components:
- `rivapi` binary in `/usr/bin/rivapi` or `/usr/local/bin/rivapi`
- `rivapi.service` in `/lib/systemd/system/`
- `rivolution-stack.target` and `stereo-tool.service` in
  `/lib/systemd/system/`
- Drop-ins in `/lib/systemd/system/<unit>.d/`
- Sudoers rule in `/etc/sudoers.d/rivapi` (set `chmod 440` in `postinst`)
- udev rule in `/lib/udev/rules.d/99-ptp.rules`
- `postinst`: `daemon-reload`, `udevadm trigger`, `systemctl enable`

Neither path has been started. Until one of them is implemented, all
deployment steps are manual.

**Update 2026-07-01:** `conf/systemd/rivapi.service` now exists and is
deployed on the dev box (survives reboot, no more manual
`go build && ./rivapi` every time — see `scripts/rivapi-rebuild.sh` for
the one-command rebuild-and-restart workflow after a source change).
This closes the specific "rivapi has no systemd unit" gap that used to
be its own entry here, but not the automation this entry describes —
a fresh install still needs every `conf/` file placed by hand.

---

## Intermittent FOP/JIT crash during DocBook PDF builds — worked around, not eliminated

**Symptom:** building any DocBook PDF target (`docs/rivwebcapi`,
`docs/manpages`, `docs/opsguide`, `docs/dtds`, `docs/apis`)
intermittently fails with `fop` exiting via SIGABRT and a
`hs_err_pid*.log` JVM crash dump — roughly one run in eight observed
during testing. The failure isn't tied to which document is being
rendered: repeated runs against the identical input produced both
clean output and crashes, and different crashes landed in different,
unrelated core JDK methods (`java.lang.AbstractStringBuilder.<init>`,
`com.sun.org.apache.xerces.internal.util.XMLChar.isContent`,
`java.lang.Byte.toUnsignedInt`) — never inside FOP's own rendering
logic. Each JVM crash report identifies the fault as a SIGSEGV inside
C1-tier JIT-compiled code.

**Cause:** appears to be a JIT-compiler bug in this OpenJDK build
(`25.0.3+9`, Ubuntu 26.04.2), not anything in this fork's DocBook
sources or stylesheets. Heap usage at crash time was a few MB out of a
3.8GB max in every captured crash, ruling out memory pressure as the
cause. A `clibsummary.xml`-specific JVM heap theory raised in an
earlier session's handoff notes was checked directly against the crash
logs and the actual output files on disk and doesn't hold up:
`clibsummary.pdf` had already built successfully by the time of the
first crash, and both crash logs' command lines point to a different,
much smaller file. The crash isn't input-specific.

**Workaround applied:** each affected `Makefile.am`'s `fop` invocation
now sets `JAVA_TOOL_OPTIONS="-XX:-TieredCompilation"`, which forces the
JVM straight to the C2 compiler and skips the C1 tier where every
observed crash originated. Across more than a dozen repeated test runs
each, the same input that crashed intermittently under default JIT
settings produced zero crashes with this flag set, at comparable
speed to the default.

**Not fixed, just mitigated:** this doesn't address the underlying JIT
bug, and the test sample isn't large enough to call the crash provably
eliminated — only much less likely. Revisit and drop the flag once the
distro's OpenJDK package moves past `25.0.3+9` and the bug can be
confirmed fixed upstream.

**Confirmed against the real build, not just the single-file test:**
ran the full `docs/rivwebcapi` build end to end with the fix in place
— 45 PDFs, 45 HTML files, and 44 man pages all built clean, zero
`hs_err_pid*.log` crash dumps, zero zero-byte/truncated PDFs. This is
the same ~50-file directory that originally surfaced the crash.

**Related, larger question, deliberately deferred:** whether to
replace the FOP+DocBook-XSL rendering pipeline entirely — see
[`ROADMAP.md`](https://github.com/anjeleno/rivolution/blob/main/ROADMAP.md).

## `autoreconf` failing repo-wide on missing top-level `ChangeLog` file — resolved

**Symptom:** running `autoreconf -fi` from a clean checkout failed with
`Makefile.am: error: required file './ChangeLog' not found`, before
automake got far enough to process any individual directory's rules.
Confirmed this was **pre-existing and unrelated** to any specific
directory's build rules: reproduced identically against an unmodified
checkout (verified via `git stash` before re-running), so it wasn't
caused by, or specific to, the DocBook/FOP work above.

**Cause:** `configure.ac`'s `AM_INIT_AUTOMAKE([1.9 tar-pax])` doesn't
pass the `foreign` option, so automake defaults to GNU-package
strictness, which requires a literal `ChangeLog` file to exist at the
top level (along with `NEWS`, `README`, `AUTHORS`, `COPYING`, all of
which this repo already has). It's purely a filename-existence check —
automake doesn't read or validate the file's contents, it just checks
the name is present. This fork's actual changelog convention is
`CHANGELOG.md` (see this repo's own internal conventions for why); the
literal `ChangeLog` filename automake checks for didn't exist — only
the frozen, never-appended-to `ChangeLog.upstream-v4` did, which
doesn't match the name automake looks for.

**Fix:** `ln -s CHANGELOG.md ChangeLog` at the repo root. Since
automake's check is existence-only and follows symlinks, this
satisfies it with zero duplicate content and zero drift risk — it's
the same file under a second name, not a second changelog to keep in
sync. Deliberately chosen over the two heavier alternatives considered
first: adding `foreign` to `AM_INIT_AUTOMAKE` would have also disabled
every *other* GNU-convention check it currently performs (not just
this one), and a real standalone `ChangeLog` file would have meant two
changelogs that could silently drift apart. Verified directly:
`autoreconf -fi` now completes cleanly end-to-end, and all five
DocBook directories' `Makefile.in` files regenerated correctly with
the JIT workaround from the entry above intact. Tracked in git as a
normal symlink (`git add ChangeLog`).

## Install prefix: resolved — use `configure_build.sh`, not raw `./configure`

Today's dev install went to `--prefix=/usr/local`, but that wasn't
because anything actually defaults there: `configure_build.sh`
(upstream's own distro-detection wrapper) already hardcodes
`--prefix=/usr` for every distro case (debian/rhel/ubuntu), matching
every example in `INSTALL`'s distro-specific notes. `/usr/local` only
happened because today's build invoked raw `./configure` directly
(for speed/control while debugging Qt6 issues), bypassing that wrapper
entirely.

`/usr/local` was originally adopted on the old shared 24.04 box
specifically to let `v4` (`/usr`) and `v6` coexist without `make
install` overwriting `v4`'s binaries. That reason no longer applies on
the dedicated `v6` box, and `/usr/local` has a real cost: it needs an
explicit `sudo ldconfig` after every install that `/usr/lib` typically
doesn't (see [`KNOWN_ISSUES.md`](https://github.com/anjeleno/rivolution/blob/main/KNOWN_ISSUES.md)), and it's non-standard relative to
every script/config elsewhere in this toolchain (Apache config,
systemd units, [`rivolution-unified-installer`'s provisioning scripts](https://github.com/anjeleno/rivolution-unified-installer/tree/main/roles/provision)) that
assumes the conventional `/usr`-rooted FHS layout. If this fork ever
ships a real `.deb`, `/usr` is mandatory — Debian packaging conventions
reserve `/usr/local` for software installed outside the package
manager.

**No code change needed** — just use `./configure_build.sh` for future
builds instead of raw `./configure`, and clean up the stale
`/usr/local`-installed files once a `/usr`-prefixed build replaces
them.

**Follow-up, 2026-06-24:** that cleanup didn't happen automatically and
the stale `/usr/local` tree sat there silently shadowing the real
`/usr` install for the better part of a day — `$PATH` and the XDG icon
theme search order both prefer `/usr/local` over `/usr`, so binaries
and icons kept resolving to the old build with zero indication
anything was wrong, until two seemingly-unrelated symptoms (RDLibrary's
group list, `rdimport`'s dropbox-watch) both got mis-diagnosed for a
while before the stale install was found and manually removed. See
[`KNOWN_ISSUES.md`](https://github.com/anjeleno/rivolution/blob/main/KNOWN_ISSUES.md) for the operator-facing symptom/check/cleanup detail.
Still no automated detection — a `make install` (or a separate check)
that flags a populated `/usr/local` Rivendell tree when installing to a
different prefix would have caught this immediately instead of costing
real debugging time.

## `make install` doesn't refresh the linker cache

Installing a new `librd-*.so` doesn't make it loadable until `sudo
ldconfig` runs, independent of install prefix — see [`KNOWN_ISSUES.md`](https://github.com/anjeleno/rivolution/blob/main/KNOWN_ISSUES.md)
for the symptom/workaround. `/usr/lib` only "stays fresh
automatically" in the common case because `apt`/`dpkg` runs `ldconfig`
as a post-install trigger; that's a property of installing through a
package manager, not of the directory itself. This fork installs via
raw `sudo make install` (a plain file copy via libtool), which never
goes through `dpkg`, so the cache needs a manual refresh either way.
Not fixed in the install target itself yet — worth an actual
`install-exec-hook` fix.

## RD_CURL_TIMEOUT orphans server-side conversions on large/slow imports

`RD_CURL_TIMEOUT` (`lib/rd.h:514`, 1200s) is shared by every HTTP-based
audio transfer (`rdimport`, Dropbox, the RDLibrary Import/Export
dialog). After 20 minutes the *client* gives up and reports an error,
but the *server* (`rdxport.cgi`) isn't told to stop — it keeps
converting to completion regardless, fully orphaned, burning CPU for
however long the conversion actually takes. Pre-existing upstream
behavior, not introduced by this fork.

**Scoped a fix (2026-06-18), then deferred it** — turned out
materially bigger than expected:
- `lib/rdaudioconvert.cpp`'s decode/encode work is spread across ~15
  separate format-specific loops, not one — a naive "check for
  disconnect" hook would touch the core conversion engine shared by
  every caller (imports, exports, voice tracker, CD ripper).
- `rdxport.cgi`'s stdout is the HTTP response body itself, parsed by
  the client as one complete XML document at the end — any heartbeat
  write mid-conversion to detect a dead connection would corrupt that
  XML and break every successful import too.
- The clean design: fork the actual conversion into a child process
  (writing only to the destination file), while the parent
  periodically `poll()`s its own stdout for `POLLHUP`/`POLLERR` (a
  read-only check, no protocol corruption) and kills the child early
  if the client's gone. Contained to `import.cpp`/`export.cpp`, but
  means adding `fork()`/process-management code to a **setuid-root CGI
  binary** (`rdxport.cgi` installs `chown root; chmod 4755`) —
  meaningfully more delicate than a quick patch.

**Current mitigation:** none server-side. Operationally, keep imports
well under the 20-minute mark. See [`KNOWN_ISSUES.md`](https://github.com/anjeleno/rivolution/blob/main/KNOWN_ISSUES.md) for the
user-facing version of this.

**Needs to be fixed before any public install** of this fork — an
unfamiliar submitter or station can't be relied on to always stay
under the timeout. Not urgent for current single-station use.

## caed's MPEG playback path doesn't resample mismatched-rate audio

Full trace and planned fix shape: see "Known issue, deferred" in
[`docs/specs/0001-mp3-import-format.md`](https://github.com/anjeleno/rivolution/blob/main/docs/specs/0001-mp3-import-format.md). Short version: `caed`
(`cae/driver_alsa.cpp`'s `WAVE_FORMAT_MPEG` case) never applies the
sample-rate-correction `ratio` that the PCM/Vorbis cases already do, so
any MP3 file whose real sample rate doesn't match the system's output
rate plays back pitch-shifted. The MP3-import passthrough feature gates
around this (only passes through when rates match — see spec 0001),
but the underlying engine bug is real and would affect any MP3 file at
a mismatched rate from any source, not just passthrough.

**Current mitigation:** the passthrough gate (fixed 2026-06-18) plus an
operational policy — all submitted mixes must be encoded at the
system's configured sample rate (48kHz on this fork's default install).

**Deliberately deferred** to its own future branch — `cae/driver_alsa.cpp`
is the live audio output path, the highest-risk code in this project
(same reasoning that put segue back-timing on its own branch rather
than bundling it into the import-feature work).

**Needs to be fixed before any public install** — can't assume every
future submitter/station will always encode at the system rate.

## Log generation doesn't exclude kill-dated carts from rotation — likely a regression

**High priority** — flagged 2026-06-19, recurred again 2026-06-21. Not
deferred indefinitely like the other entries here.

A promo cart (cart 064721, "Live at Swan Dive LV") had a kill date of
8PM the next day. A script that auto-generates the music log three
days out kept scheduling that same cart into logs for days *after* its
kill date had already passed, instead of excluding it from rotation
and picking another active cart in the same category. The result
wasn't a quiet skip — the scheduler placed the expired cart into the
log anyway, and the Log Exception Report then flagged every one of
those slots as "is not playable" (20 exceptions in one observed case,
for log `2026_06_23`).

Manually swapping the expired promo for an active one in the rotation
fixed the report for logs generated *after* the swap — but the
underlying bug isn't really about any one already-generated log: as
long as an expired cart remains a member of a category's rotation, the
scheduler will keep selecting it for any newly-generated log, kill date
or not. Suspected regression — reported as not happening in earlier
Rivendell versions (possibly introduced in 4.4.1 specifically — not yet
bisected to confirm or to a specific upstream change).

**Recurred 2026-06-21** with a second, independent promo (kill date 8PM
6/20), same symptom, same workaround. That second occurrence also
surfaced something new, not present in the original report: **the
rotation itself looks wrong too**, separately from the kill-date
exclusion issue — not yet characterized with a concrete example of what
it actually did instead of the expected sequential order. Worth keeping
in mind during investigation: this category's rotation type is
sequential (not percentage- or weight-based) — when those alternate
rotation types aren't explicitly set, rotation should just cycle
through members in order (A B C A B C A B C...), so any fix needs to
confirm it's actually checking the configured rotation type, not
assuming sequential.

**Current mitigation:** manually remove/replace kill-dated carts from
their rotation category before they expire, rather than relying on the
scheduler to skip them automatically. See [`KNOWN_ISSUES.md`](https://github.com/anjeleno/rivolution/blob/main/KNOWN_ISSUES.md) for the
user-facing version.

**Deferred for now** at the reporter's request — not investigated yet,
but tracked here as the next thing to pick up given the priority, and
now reinforced by a second real-world occurrence.

## RDAirplay plays silence for a cart with a missing audio file, instead of skipping it — likely a regression

**High priority** — flagged 2026-06-21.

Some carts in the library have a cut record in the database with no
actual backing audio file (file never made it to `/var/snd`, got
deleted out-of-band, etc.). Rivendell currently treats "a cut record
exists" as "this cart has audio," with no check that the file is
actually present. When such a cart is scheduled and reached during
playout, `RDAirplay` plays silence for the cut's full listed duration
instead of moving on. Reported as a regression: previous behavior was
to skip a cart with missing audio automatically and start the next log
element immediately, with no dead air.

Not yet investigated — no file/line citations yet for where
`RDAirplay`'s playout logic decides "does this cart have audio" (likely
checking cut-record existence rather than file existence).

**Current mitigation:** none. See [`KNOWN_ISSUES.md`](https://github.com/anjeleno/rivolution/blob/main/KNOWN_ISSUES.md) for the user-facing
version.

**Deferred for now** — not investigated yet, but tracked here given the
priority (dead air on a live broadcast). See [`ROADMAP.md`](https://github.com/anjeleno/rivolution/blob/main/ROADMAP.md) for the
related feature request (a library-wide missing-audio audit tool) that
came up alongside this report — distinct from this bug fix itself.

## Edit Markers waveform goes blank/truncated at high zoom on long cuts — fixed

**Flagged 2026-06-22, root-caused and fixed 2026-07-03.** Long-standing,
pre-existing Rivendell v4 behavior — not introduced by this fork. Same
underlying bug as upstream `ElvishArtisan/rivendell` issue #835
("RdLibrary: Waveform display broken when zooming"), open there since
2022 with only a stopgap (capping the maximum zoom level rather than
fixing the rendering).

**Root cause:** `RDMarkerView::WriteWave()` rendered a cut's entire
waveform into a single `QPixmap`, sized to `cut_length / zoom_level`.
Long cuts at high zoom need a pixmap wider than Qt/X11's roughly
32767px maximum single-bitmap dimension, which silently truncates or
blanks the pixmap's contents past that width rather than erroring —
explaining both symptoms (blank near the end of a long file, or
truncated mid-file) as the same limit hit from different starting
positions. A partial safety net already existed
(`RDMarkerView::d_min_shrink_factor`, capping how far a user could zoom
in) but wasn't actually enforced on either real interaction path
(mouse-wheel zoom, the "Full In" toolbar button), so in practice it
provided no protection at all.

**Fix:** `RDWaveFactory::generate()` (`lib/rdwavefactory.cpp`) now
accepts an optional column range and can render just a slice of the
waveform; `WriteWave()` (`lib/rdmarkerview.cpp`) renders the full width
as a strip of bounded-width tiles (each well under the ~32767px limit)
laid edge-to-edge in the same `QGraphicsScene`, instead of one pixmap.
No single `QPixmap` this widget creates can hit the limit regardless of
cut length or zoom, so the now-superfluous `d_min_shrink_factor` cap
was removed entirely — zoom is limited only by the underlying peak
data's own resolution (see the precision entry below), not an
artificial ceiling.

**Known remaining limitation, deferred:** the peak data
`RDWaveFactory`/`RDWavePainter` render from is pre-computed in
1152-sample blocks (~26ms at 44.1kHz) — `RDWaveFile::GetEnergy()`, the
same mechanism used for the MP3 passthrough autotrim support added
2026-07-03. Zooming in past that resolution doesn't reveal finer real
detail, it just stretches the same block value across more pixels.
Genuinely sample-accurate marker placement at extreme zoom would need
a further change: falling back to reading real per-sample PCM data
directly from the file (rather than the coarse peak-block cache) once
the visible zoom level exceeds that resolution. Not implemented here —
this fix resolves the crash/truncation bug and the artificial
file-length-dependent zoom ceiling, not the separate ~26ms precision
floor.

## RDLibrary's manual "Add" cart flow may still lose audio after a confirmed OK click — suspected Qt6-migration regression

**High priority** — flagged 2026-06-27. Suspected regression: the
reporter states this didn't happen before the Qt6 migration — not yet
bisected to confirm or to a specific change.

A related cart-audio-loss bug was already found and fixed (see
[`CHANGELOG.md`](https://github.com/anjeleno/rivolution/blob/main/CHANGELOG.md#2026-06-26), 2026-06-26): `rdlibrary.cpp`'s `addData()` deleted a
newly-added cart's audio whenever `EditCart::exec()` returned anything
other than true — every dialog-close path except OK. That fix removed
the rollback's dependence on which button closed the dialog.

The two original field reports that prompted that investigation both
involved the OK button specifically being clicked — confirmed directly
by the reporter, not assumed. `rdlibrary/edit_cart.cpp`'s `EditCart`
dialog has exactly two exit paths: `okData()`'s `done(true)` (OK only,
and only when no validation warning blocks it first) and
`cancelData()`'s `done(false)` (every other close, including via
`closeEvent()`). `addData()` only deletes a cart when `exec()` returns
false. That code does not explain audio loss following a confirmed OK
click. Whatever caused the two original incidents isn't yet
identified, and may be a second, separate bug from the one already
fixed.

**Not yet investigated** — no file/line citation yet for any mechanism
that could make `EditCart::exec()` return non-true despite the OK
button being clicked, or for any other path that could delete a cart's
audio independent of `EditCart::exec()`'s return value. Worth checking
first whether any Qt6 signal/connection rename of the kind already
found elsewhere in this migration ([`docs/specs/0006-qt6-migration.md`](https://github.com/anjeleno/rivolution/blob/main/docs/specs/0006-qt6-migration.md))
touches `ok_button`'s `clicked()` connection or `okData()`'s own
control flow — the OK path looks unaffected on a first read, but that
first read is exactly what missed the other three renames already
found and fixed in this codebase.

**Current mitigation:** none. See [`KNOWN_ISSUES.md`](https://github.com/anjeleno/rivolution/blob/main/KNOWN_ISSUES.md) for the
user-facing version.

**Deferred for now** at the reporter's request — tracked here given the
priority (live audio loss on a broadcast system), to be investigated in
a future session.

## Add-cart rollback fix doesn't distinguish an explicit Cancel from an incidental dialog close — too broad

**Flagged 2026-06-27**, found during a documentation-accuracy review of
the 2026-06-26 cart-deletion-on-close fix.

The governing principle should be: audio already written to
`/var/snd` with a matching database row is persisted, full stop, and
no dialog-close path should be able to destroy it — except an explicit
Cancel, which is the user actively saying "discard this," and should
still roll back regardless of whether audio was already imported.

The fix as shipped doesn't draw that line. `rdlibrary.cpp`'s
`addData()` now keeps any cart with real audio (`CUTS.LENGTH > 0`)
"regardless of how the dialog was closed" — which also covers an
explicit Cancel click, not just incidental closes like Escape or the
window's X. `rdlibrary/edit_cart.cpp`'s `EditCart` dialog can't
currently tell Cancel apart from those other closes: both reach
`cancelData()` → `done(false)` via the identical path (`closeEvent()`
calls `cancelData()` directly). Distinguishing them would need
`cancelData()`/`closeEvent()` to record which one actually fired, or a
dedicated signal for the Cancel button specifically.

**Current mitigation:** none — clicking Cancel after a successful
import currently still keeps the cart, the opposite of what Cancel
implies. See [`KNOWN_ISSUES.md`](https://github.com/anjeleno/rivolution/blob/main/KNOWN_ISSUES.md) for the user-facing version.

**Deferred for now** — found during documentation review, not yet
scoped as a real fix. Worth picking up alongside the item above (the
confirmed-OK-click cart loss), since both touch the same rollback
decision in `addData()`.

## xrdp/dbus session-bus bug after a host reboot or xrdp restart

"Could not acquire name on session bus" black-screen error, hit
repeatedly on both this dev VM and the production on-air machine.
Likely a stale per-user `dbus-daemon`/`systemd --user` instance left
behind by an orphaned xrdp session, holding a bus name a fresh login
can't reacquire. Not a Rivendell bug — an Ubuntu/xrdp interaction.

**Current mitigation:** full host reboot (the only proven fix so far).
Disruptive on a live on-air machine.

**Deferred** — needs investigation into a lighter-weight fix (e.g.
`loginctl terminate-session`, killing the stale per-user bus) suitable
for a machine that can't just be rebooted on demand. Low priority since
the reboot workaround, while disruptive, reliably works.

## Broadcast dashboard's HE-AAC stream option can't actually encode SBR

Ubuntu's packaged `libfdk-aac2` (2.0.2-3~ubuntu5) has SBR (spectral band
replication) *encoding* disabled — the AAC-LC encoder and SBR decoder are
patent-unencumbered and shipped, but the SBR encoder specifically carries
a separate patent restriction some distros build out. `fdkaac --profile 5`
(HE-AAC) and `--profile 29` (HE-AAC v2) both fail with "unsupported
profile" on this build; only AAC-LC (`--profile 2`), AAC-LD (`23`), and
AAC-ELD (`39`) work.

**Current mitigation:** the dashboard's he-aac-v1/he-aac-v2 codec options
both generate a plain AAC-LC stream (`liqEncoder` in
`rivapi/store/liquidsoap_generator.go`) instead of true HE-AAC. Decent
quality, but loses the low-bitrate efficiency SBR is chosen for.

**Deferred** — real fix needs either a non-distro `fdk-aac`/`fdkaac`
build compiled with SBR encoding enabled, or switching the
low-bitrate-efficient stream option to a different codec (e.g. Opus,
which isn't patent-restricted and is already linked into Liquidsoap).

## Audio processing chain routing (caed -> Stereo Tool -> Liquidsoap) is hardcoded

The signal chain needs caed's JACK output routed into Stereo Tool, and
Stereo Tool's output routed into Liquidsoap. Stereo Tool only reaches
JACK via an ALSA-JACK bridge plugin (its "jack (ALSA)" I/O option) —
not as a native JACK client — and that plugin's default port mapping
(`/usr/share/alsa/alsa.conf.d/50-jack.conf`) is hardcoded to
`system:capture_1/2`/`system:playback_1/2`, real jackd+ALSA hardware
port names that don't exist under system-scope PipeWire.

**Current mitigation:** `conf/alsa/rd.asoundrc` overrides the `pcm.jack`
definition with a hardcoded mapping to `rivendell_0:playout_0L/R` (input)
and `liquidsoap:in_0/1` (output). Works, but is fixed at exactly one
routing — no way to reassign which caed stream feeds Stereo Tool, or add
additional processing/monitoring taps, without hand-editing this file.

**Update 2026-07-01:** an MVP patchbay page now exists (`/patchbay`,
`rivapi/store/patchbay.go`, `handlers_patchbay.go`), and has been used
to replace `conf/alsa/rd.asoundrc`'s hardcoded routing for real —
connections are made and saved from the dashboard, no hand-editing or
SSH required. Two iterations so far:

1. First pass: an output x input matrix table, no persistence.
2. Same day, after real use: the matrix didn't fit on screen at a
   readable zoom level once there were more than a handful of ports —
   replaced with a connections list (Output → Input rows, each with a
   Remove button) plus an "Add connection" form (two dropdowns). Scales
   with the number of *connections* (usually small) instead of
   outputs x inputs (grows fast). Also added real persistence: a
   "Save current patch" button snapshots the live graph to
   `/home/rd/etc/rivolution/patchbay.json`, and a background poll in
   `rivapi` (every 5s) re-applies anything missing — **verified working
   live**, survived a real Liquidsoap restart with no manual
   reconnection needed.

**This is still deliberately Phase 1, not the final design** — see
`docs/specs/0007-pipewire-audio-engine.md`'s Implementation deviations
for the full writeup, but in short: WirePlumber's own declarative
link-policy mechanism was investigated and empirically confirmed **not
to apply** to JACK-bridged ports (the current architecture) — not
merely deferred for lack of time. The `rivapi`-side poll-and-reapply
reconciler is what actually works today; it's expected to be retired
in favor of real WirePlumber policy once Phase 2 lands (`caed`/
Liquidsoap/Stereo Tool as native PipeWire clients, no JACK bridge).
Also still open: no live/auto-refresh (reload the page to see changes
made outside the dashboard), and no client-grouped visual graph (still
plain text rows, not virtual patch cables) — a nicer visual pass is
wanted eventually but not scoped yet.

## Groups and Carts nav links hidden — not yet meaningful in the new dashboard

`/groups` and `/carts` (from the original `rivapi` Phase 1 work) are
still fully implemented and reachable by direct URL, but as of
2026-07-01 don't make sense as top-level navigation yet — they're
early scaffolding from before this fork's actual build priority order
(RDAdmin parity is explicitly long-tail, after service control,
broadcast tools, Tailscale, and PipeWire — see `ROADMAP.md`) and
don't yet connect to anything an operator would actually use them for
day to day.

**Current mitigation:** hidden from `base.html`'s nav bar and
`home.html`'s button grid (Go template comments, not deleted code —
trivial to re-enable). Routes/handlers untouched.

**Revisit** once RDAdmin porting actually reaches group/cart
management — at that point either restore them as-is or redesign
them alongside whatever RDAdmin-parity work makes them coherent.

## Dashboard needs to replace what the removed Ansible `broadcast_advanced` role did

`anjeleno/rivolution-unified-installer`'s `broadcast_advanced` role
(and its `advanced-config/` bundle) was removed 2026-07-01 — its
Icecast/Liquidsoap/VLC config generation and Stereo Tool download are
now properly handled by `rivapi` (specs 0007/0008), and its seed
database was hardcoded to a single host name (`onair`), not
general-purpose. But the role also did several things with **no
dashboard equivalent yet**, silently lost when it was removed:

- Nightly cron jobs: database backup (`daily_db_backup.sh`) and log
  generation (`autologgen.sh`)
- `auto-merge.sh`, `reconcile-traffic.sh`, `stl.sh` — automation
  scripts with no traced dashboard replacement
- Desktop shortcuts for RDAdmin/RDAirplay/RDLibrary/RDLogEdit/
  RDLogManager, Stereo Tool, and STL

**Deferred, not forgotten** — the plan is a custom implementation of
this functionality in the Go dashboard (`rivapi`) rather than
reintroducing the old fixed-bundle Ansible role. Needs its own scoping
pass: at minimum, scheduled DB backup and log generation are real
operational gaps until this lands. The removed role's content is still
in `rivolution-unified-installer`'s git history if needed as a
reference for what the old scripts actually did.

## Patchbay can't persist connections to clients with PID-embedded JACK names (Stereo Tool's case fixed; general case still open)

Some ALSA-JACK-bridged software (confirmed with Stereo Tool) names its
own JACK client after its own process ID (`stereo_tool.C.<pid>.<n>`),
which changes every restart. The dashboard's patchbay Save/Reconcile
feature used to persist by exact name match, so it could never restore
a connection to a client like this -- not a bug in the reconciler, a
structural limit of name-matching against an identifier that's
guaranteed to change. Stereo Tool's own auto-*connection* is separately
worked around via `~/.asoundrc`'s `pcm.jack` block, which hardcodes the
stable *target* port names instead of trying to match the unstable
*source* name -- but the bidirectional reconciler was tearing that
auto-connection back down every ~30 seconds anyway, since it could
never match the stale PID saved in `patchbay.json`.

**Fixed for Stereo Tool specifically:** `store/patchbay.go`'s
`normalizedLinkKey`/`normalizePortName` collapse Stereo Tool's PID
segment (`stereo_tool\.[CP]\.\d+\.` -> pattern-matched, not
exact-matched) before comparing saved links against live ones, so a
saved link is recognized as already satisfied -- and a live link
recognized as already saved -- regardless of which PID Stereo Tool has
this run. Deliberately scoped narrowly to Stereo Tool's known pattern
rather than a general mechanism, since it was the only confirmed real
case at the time.

**Still open, deliberately not built:** the *general* version of this
for whenever a second, different piece of software hits the same
quirk -- Stereo Tool's fix above is hardcoded to its specific naming
pattern, not reusable as-is for anything else. A real general fix would
need:

- Detect it: a saved connection that never reconnects while a
  similarly-named live connection keeps appearing (same prefix/suffix,
  different embedded number) is a real, checkable signal.
- For ALSA-JACK-bridged clients specifically: a button that
  generates/updates an `~/.asoundrc` `pcm.jack` block pinning the
  stable target ports for the detected pattern, same shape as Stereo
  Tool's existing workaround.
- For native JACK/PipeWire clients with unstable names (no ALSA layer
  to hook into at all): the only real generalization is pattern-based
  matching inside patchbay's own persistence for an arbitrary detected
  pattern -- storing a glob/regex instead of an exact name -- which is
  a materially bigger change to `store/patchbay.go`'s data model than
  the Stereo-Tool-specific fix above, not a quick add.

Related, smaller gap also not yet built: `/patchbay` currently shows
nothing at all when a saved connection can't be re-established -- it
just silently vanishes from the page. A "Saved but not currently
connected" section would at least make an orphaned save visible
instead of silent, independent of whether the general fix above ever
gets built.

## x64 CI builds can silently ship a higher CPU ISA floor than intended

`build-deb.yml`'s x64 leg runs on a standard GitHub Actions Ubuntu
26.04 runner. Ubuntu 26.04's package archive is CPU-ISA-tiered (a plain
`amd64` component and a separate `amd64v3` component), and `apt`
automatically prefers whichever tier matches the installing machine's
own detected CPU capability. A modern cloud CI runner supports v3
easily, so its entire build toolchain -- including the CRT startup
objects the linker merges into every compiled binary -- gets installed
from the `v3` tier, regardless of any `-march`/`-mtune` flag this
project's own build passes (see `ARCHITECTURE.md`'s "x86-64 ISA
baseline" section for the full mechanism). Building the same source on
genuinely v2-class hardware instead produces binaries requiring only
the universal `x86-64-baseline` floor, confirmed via `readelf -n`.

Deliberately not yet fixed at the CI level: whether pinning `apt` to
the plain `amd64` component inside `build-deb.yml`, before it installs
build dependencies, would make the runner's own toolchain match a
v2-class target regardless of the runner's real hardware. Untested --
picking this up means verifying it actually holds with a real build,
not just that the pin was applied. Until then, x64 packages that need
to support pre-Haswell-class hardware are built directly on real
v2-class hardware instead of through this CI workflow, and the
resulting asset is manually substituted for whatever `build-deb.yml`
auto-attaches to the release. This is a manual step with nothing
enforcing it, and is worth automating once a decision is made either
way.

## Per-function AVX2/BMI2 multi-versioning for audio processing code

amd64 builds now compile with a global `-march=x86-64-v2` cap (see
`ARCHITECTURE.md`'s "x86-64 ISA baseline" section for the full story) --
a real crash on genuine pre-Haswell hardware forced this, and nothing in
the affected binaries is known hot, vectorizable code, so the cap costs
nothing measurable today. Not fixed, and deliberately not attempted
speculatively: whether any part of the audio processing chain (`caed`,
`rdaudioconvert`'s resampling) has an actual hot loop that would benefit
from AVX2/BMI2/FMA specifically. GCC's function multi-versioning
(`target_clones`) can restore v3-class speed for one specific,
individually annotated function while leaving the rest of the program at
the safe v2 baseline -- worth picking up only if real profiling finds an
actual bottleneck there, not before.

## `postinst`'s fresh-install check can misfire after an earlier failed install attempt, destructively wiping an existing database

`debian/postinst` decides whether to run its destructive first-time
database setup (`drop database if exists` / `create database` /
`rddbmgr --create --generate-audio`) versus the safe upgrade path
(`rddbmgr --modify`) based solely on whether dpkg's `$2` argument to
`postinst configure` is empty. Per dpkg's own mechanics, that argument
reflects the last version for which `configure` *completed
successfully* -- not simply whether some version was previously
unpacked. If every earlier install attempt failed partway through
`postinst` (for example, the exec-format/ISA-mismatch failure described
in `ARCHITECTURE.md`'s x86-64 ISA baseline section), dpkg has never
recorded a successful configure for the package at all. The next
attempt that finally succeeds is then indistinguishable, from this
script's point of view, from a genuinely fresh install -- so it takes
the destructive branch and wipes whatever database already exists,
even on what looks from the outside like a routine upgrade.

Not yet fixed. The fresh-install check should be based on whether the
target database/schema actually exists (or is genuinely empty) rather
than trusting dpkg's bookkeeping alone, since that bookkeeping can be
reset by an unrelated failure in an earlier install attempt.

## Segue markers are frozen into the log at generation time, unlike talk markers

`RDLogModel::LoadLines()` reads `SEGUE_START_POINT`/`SEGUE_END_POINT`
only from `LOG_LINES` -- a snapshot copied in from `CUTS` whenever that
log line was originally generated or imported. Editing a cart's segue
markers in the library afterward has no effect on any log that already
existed, including on a plain unload/reload in the on-air player;
picking up the edit requires deleting and regenerating that log. Talk
(intro) markers don't have this problem -- `RDPlayDeck` reloads them
live from `CUTS` on every cue, regardless of log age. Confirmed via a
real test (delete + regenerate a log, reload it, edit still not fully
reflected in some lines) while investigating the segue back-timing
truncation bug (see `CHANGELOG.md`, 2026-07-07); ruled out as the
dominant cause there, but the asymmetry itself is real and unfixed.
Deferred because it wasn't the actual bug in that investigation and a
fix means deciding whether segue points should also be reloaded live
(mirroring talk markers) or whether the frozen-snapshot behavior is
intentional (it would preserve a voice-tracker's per-line marker
override independent of later library edits) and the real gap is only
that ordinary library edits have no path to reach an already-built log
at all.
