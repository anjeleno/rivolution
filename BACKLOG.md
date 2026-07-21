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

## No automated detection of a stale `/usr/local`-prefix install shadowing `/usr`

A raw `./configure` (bypassing `configure_build.sh`, which correctly
hardcodes `--prefix=/usr`) installs a complete parallel copy of every
binary/library/icon at the autotools default prefix, `/usr/local`.
`$PATH` and the XDG icon search order both prefer `/usr/local` over
`/usr`, so a stale copy left behind this way silently keeps winning —
confirmed live 2026-06-24: it shadowed a correct `/usr` install for the
better part of a day, and two unrelated-looking symptoms (RDLibrary's
group list, `rdimport`'s dropbox-watch) got mis-diagnosed before the
stale install was found. See
[`KNOWN_ISSUES.md`](https://github.com/anjeleno/rivolution/blob/main/KNOWN_ISSUES.md)
for the full manual detection/cleanup steps.

**Not yet built:** a `make install` hook (or separate check) that flags
a populated `/usr/local` Rivendell tree when installing to a different
prefix, so this is caught immediately instead of costing real debugging
time. Using `configure_build.sh` instead of raw `./configure` avoids
causing this in the first place, but doesn't detect an existing one
left over from before.

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

## Waveform zoom has a ~26ms precision floor

The Edit Markers waveform's own crash/truncation bug at high zoom on
long cuts was root-caused and fixed 2026-07-03 (tiled rendering instead
of one oversized `QPixmap` — see `CHANGELOG.md`). A separate, smaller
limitation remains, deliberately deferred: the peak data
`RDWaveFactory`/`RDWavePainter` render from is pre-computed in
1152-sample blocks (~26ms at 44.1kHz) — `RDWaveFile::GetEnergy()`, the
same mechanism used for the MP3 passthrough autotrim support added
2026-07-03. Zooming in past that resolution doesn't reveal finer real
detail, it just stretches the same block value across more pixels.

**Not yet built:** genuinely sample-accurate marker placement at
extreme zoom would need falling back to reading real per-sample PCM
data directly from the file (rather than the coarse peak-block cache)
once the visible zoom level exceeds that resolution — new code, not a
port of anything that exists elsewhere in this codebase today.

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

## Audio processing chain routing (caed -> Stereo Tool -> broadcast streams) is hardcoded

The signal chain needs caed's JACK output routed into Stereo Tool, and
Stereo Tool's output routed into the broadcast streams. Stereo Tool
reaches JACK through ALSA's `type jack` PCM plugin, exactly as its own
config UI's "jack (ALSA)" device label says (confirmed via live
behavioral testing, not just reading `ldd`/`strings` output — see
ARCHITECTURE.md's "device label" recurring-mistake writeup for why
static linkage evidence alone couldn't settle this and briefly pointed
the wrong way). The actual target is configured in `~/.asoundrc`
(`conf/alsa/rd.asoundrc`), not `~/.stereo_tool.rc`'s own "Jack ID"
fields.

**Original mitigation:** `conf/alsa/rd.asoundrc` mapped a hardcoded
`pcm.jack` definition to `rivendell_0:playout_0L/R` (input) and
`liquidsoap:in_0/1` (output) — the `liquidsoap` target stopped existing
once Liquidsoap was replaced by per-stream ffmpeg processes, which is
what "Stereo Tool keeps leaking JACK clients" turned out to be. See
Update 2026-07-10 below for the fix that's actually shipped.

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

**Update 2026-07-10 (shipped, see `docs/specs/0015-ffmpeg-broadcast-output.md`
for the full design):** the `liquidsoap:in_0/1` target described above
stopped existing once Liquidsoap was replaced by per-stream ffmpeg
processes; every failed auto-connect attempt tore Stereo Tool's client
down and retried with a fresh index, forever. An earlier fix attempt
(same day) built a permanent PipeWire virtual bus
(`libpipewire-module-loopback`) as Stereo Tool's target, on the theory
that it needed something permanent and stream-count-independent.
Confirmed live this was itself unreliable — Stereo Tool's ALSA-JACK
bridge connects stably to a genuine native JACK client but not to a
loopback-module node — and was removed. Fixed generically without any
intermediary process at all:

- Stereo Tool's fixed `~/.asoundrc` target is whichever configured
  broadcast stream sorts first by mount — fully dynamic
  (`primaryStreamMount`), re-evaluated on every deploy, never a
  hardcoded mount name. `syncStereoToolTarget`
  (`rivapi/store/ffmpeg_generator.go`) patches `~/.asoundrc` and
  restarts `stereo-tool.service` only when that target actually
  changes.
- The fan-out from Stereo Tool's live output to every *other*
  configured stream — and to the anchor stream too, which must stay in
  the saved set rather than being skipped as redundant, since
  `ReconcileLinks`' removal half tears down anything absent from it —
  uses the same operator-set `BroadcastConfig.ProgramSource` field
  (now with a `"stereo_tool"` sentinel value alongside plain JACK
  client names), auto-connected once per stream on first deploy
  (`syncStreamPatchLinks`), never touched again after that, so a manual
  `/patchbay` override always sticks.
- `ReconcileLinks` itself needed a real fix, not just Stereo Tool's own
  side: a saved link naming a since-restarted Stereo Tool's stale PID
  could never actually reconnect, since only the "is this already
  satisfied" comparison was PID-agnostic, not the actual `Link()` call.
  `resolvePortName` (`rivapi/store/patchbay.go`) resolves a stale saved
  name to whatever's currently live with the same normalized identity
  first.
- Verified end-to-end: a full stack restart (including a genuine VM
  crash and recovery) reconnects every link automatically with zero
  manual reconnection, on a fresh PID every time.

Known gap, not yet generalized: a station with its own saved Stereo
Tool presets (`~/.stereo_tool.presets/*.sts`) can have a loaded preset's
own copy of the device section override the base `.rc`/`.asoundrc` fix
— the immediate case was resolved by hand through Stereo Tool's own web
UI.

Phase 2 (`caed`/Stereo Tool/streams as fully native PipeWire clients,
no per-app auto-connect guessing or ALSA bridging anywhere) remains the
real final fix and is still unscoped.

**Update 2026-07-20 — real ordering trap found and confirmed on a
fresh droplet install.** `syncStereoToolTarget` only runs as part of
`/broadcast`'s Save & Deploy, using whatever `ProgramSource` is already
saved to disk at that moment. On a genuinely fresh install, the natural
order is: configure streams and click Save & Deploy on `/broadcast`
first (before `ProgramSource` has ever been set on `/patchbay`), then
set `ProgramSource` afterward. Per the 07-17 design decision, saving
`ProgramSource` on `/patchbay` deliberately does not trigger a
redeploy — so the very first deploy runs with an empty `ProgramSource`,
`syncStereoToolTarget` no-ops (no error, no write, no restart), and
`~/.asoundrc` is left exactly as shipped: hardcoded to the long-dead
`liquidsoap:in_0/1` target inherited from the pre-ffmpeg-swap era (see
`conf/alsa/rd.asoundrc`). Restarting the stack doesn't fix it either —
nothing but a real `/broadcast` Save & Deploy call reaches
`syncStereoToolTarget`. Confirmed live: re-running Save & Deploy on
`/broadcast` after `ProgramSource` was set patched `~/.asoundrc` to the
real `ffmpeg-192:input_1/2` target and restarted `stereo-tool.service`
correctly — `/patchbay` showed all four expected links (both
`rivendell_0` → Stereo Tool input ports, both Stereo Tool output ports
→ `ffmpeg-192`) saved immediately after.

**Not yet fixed:** the trap itself — whichever of `/broadcast` Save &
Deploy or `/patchbay`'s Program Source save runs first, the other one's
effect on Stereo Tool's routing is silently deferred with no warning —
needs either a UI nudge or real install documentation calling it out
explicitly. A wiki write-up for `.deb` package installs (distinct from
the existing from-source build walkthrough) is planned to cover this
with an unmissable callout, alongside the audio-driver provisioning gap
above.

## Nothing in any GUI can actually set `AUDIO_CARDS.DRIVER` — RDAlsaConfig only manages ALSA device selection, not the driver column

Traced 2026-07-21 while scoping the fresh-install JACK-default fix
below: this entry originally described RDAlsaConfig's "deselect every
listed device, click Save" gesture as an *unlabeled* way to select
JACK. That undersold the actual gap. Direct code tracing (not
assumption) found:

- `RDAlsaConfig` (`utils/rdalsaconfig/rdalsaconfig.cpp`) only ever
  writes `/etc/asound.conf` (an ALSA hardware enable/disable list,
  `RDAlsaModel::saveConfig()`) — no code path in this tool touches the
  database at all.
- `RDAdmin`'s "Edit Audio Ports" dialog displays Card Driver as a
  **read-only** field (`rdadmin/edit_audios.cpp:65`,
  `card_driver_edit->setReadOnly(true)`) — it shows "JACK Audio
  Connection Kit"/"ALSA"/"AudioScience HPI"/"[none]" but has no control
  to change it.
- `RDStation::setDriver()` (`lib/rdstation.cpp`, the only method that
  writes this column) has **zero callers anywhere in the codebase** —
  confirmed via a full-tree grep, including the other Rivendell/
  Rivolution checkouts on this box (`rivendell`,
  `rivendell-v6-deprecated`, `rivendell-work`).

So every previously-working box got `AUDIO_CARDS.DRIVER` set via a raw
SQL edit, not "RDAdmin's Edit Audio Ports dialog" as this entry
previously claimed — that claim was wrong and is corrected here.
Live-observed correlation on the 2026-07-20 droplet (an empty
`asound.conf` alongside a JACK-driven Card 0) is consistent with either
a real sync mechanism this trace missed, or simply both having been set
correctly by hand at the same time with nothing keeping them in sync
since — not yet settled either way; a live before/after test (toggle
RDAlsaConfig's device selection, check the DB) would confirm it, not
yet run.

**Now load-bearing, not just a UX nitpick**, given the fresh-install
default below: once `postinst` defaults Card 0 to JACK unconditionally,
a station that genuinely wants ALSA-driven physical hardware has **no
working path at all** to switch to it short of a manual SQL edit.
**Needed:** make `RDAlsaConfig` write `AUDIO_CARDS.DRIVER` for real when
an operator selects a specific device (and back to JACK when none are
selected) — turning today's implicit, database-blind gesture into the
actual control surface it's already assumed to be. Also flagged by
Brandon as something to revisit properly once PipeWire's own native
`caed` driver work (see `ROADMAP.md`'s "Full modernization" section)
gets a real planning pass — this may get superseded rather than fixed
in place, depending what that redesign decides about audio-device
selection generally.

**Design refined 2026-07-21, not yet built:** Brandon's concrete shape
for the fix above --
- Label the explicit driver choice **"PipeWire/JACK"**, not bare
  "JACK" — both in the new RDAlsaConfig control and in RDAdmin's Edit
  Audio Ports read-only display (currently "JACK Audio Connection
  Kit," `rdadmin/edit_audios.cpp:334`). Deliberately named to signal
  the real mechanism (`caed`'s JACK driver, backed by `pipewire-jack`)
  and that it's a bridge, not the end state — this label should change
  again once `caed` gains a genuinely native PipeWire driver.
- Selecting "PipeWire/JACK" in RDAlsaConfig must **automatically
  deselect every currently-selected ALSA device** before `Save` runs —
  not require the operator to separately uncheck them, which is the
  exact awkwardness this whole entry is about. The two states
  (PipeWire/JACK selected vs. one-or-more real ALSA devices selected)
  should be mutually exclusive at the control level, not just by
  convention.
- Implementation hook: `RDAlsaConfig::saveData()`/`SaveConfig()`
  (`utils/rdalsaconfig/rdalsaconfig.cpp`) already runs once, right
  place to also call `RDStation::setDriver()` — writing `Jack` when
  "PipeWire/JACK" is selected, `Alsa` when a real device is selected
  instead. No new plumbing needed on the database side; this genuinely
  is just wiring up an existing, currently-dead method.

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
general-purpose. But the role also did several things with no dashboard
equivalent at the time it was removed:

- ~~Nightly cron jobs: database backup (`daily_db_backup.sh`) and log
  generation (`autologgen.sh`)~~ — **done.** `/tasks` (2026-07-04) added
  exactly this as real systemd service+timer pairs, not a
  reimplementation of cron; its log-generation task type had a real bug
  (calling `rdlogmanager -t` instead of `-g`) found and fixed 2026-07-09
  on the first real-box run. See `CHANGELOG.md`.
- `auto-merge.sh`, `reconcile-traffic.sh`, `stl.sh` — automation
  scripts still with no traced dashboard replacement.
- Desktop shortcuts for RDAdmin/RDAirplay/RDLibrary/RDLogEdit/
  RDLogManager, Stereo Tool, and STL — still not built.

**Deferred, not forgotten** — the plan is a custom implementation of
this functionality in the Go dashboard (`rivapi`) rather than
reintroducing the old fixed-bundle Ansible role. The removed role's
content is still in `rivolution-unified-installer`'s git history if
needed as a reference for what the old scripts actually did.

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

## No Debian-built `.deb` release target

`build-deb.yml`'s x64 leg only ever runs on Ubuntu runners (26.04
primary, 24.04 `-noble`); the arm64 leg is built by hand on `rivdev`
(also Ubuntu). No Debian Trixie `.deb` has ever been published, despite
spec 0002 in `rivolution-unified-installer` having specifically
designed ARM64 *and* Debian source-build support. Confirmed a real,
practical consequence 2026-07-20: `rivolution-unified-installer`'s
rewrite around installing the released `.deb` by default
(`docs/specs/0004-deb-based-provisioning.md` in that repo) means its
`rivolution_install_method: deb` path can't target Debian at all until
this closes — `rivolution_install_method: source` (builds a local
`.deb` from a checkout) is the only way onto Debian with that playbook
today.

**Needed, not yet built:** a Debian leg in `build-deb.yml`, or a
documented decision that Debian support is source-build-only going
forward and spec 0002's original intent has narrowed. Whichever way
this goes, update `rivolution-unified-installer`'s spec 0004 to match
once it's decided.

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

## The MySQL database itself is still named "Rivendell", not "Rivolution"

The rebrand only ever touched the branding/package layer -- binaries,
service names, dashboard UI. The actual schema name is hardcoded as
`Rivendell` in roughly a dozen places in the inherited
`scripts/rd_create_db` and shipped as the default in
`conf/rd.conf-sample`'s `[mySQL] Database=` line, so every install's
live database is genuinely named `Rivendell`, not just displayed that
way somewhere.

Noticed 2026-07-09 when a database backup task named its output file
from `/etc/rd.conf`'s `Database=` value, producing a
`Rivendell_<date>.sql.gz` file on a Rivolution box. Deliberately left
alone rather than renamed: touching the live schema name is a bigger,
riskier change than anything else in the rebrand (every `USE
Rivendell;` in that script, plus a migration path for databases that
already exist on real installs) and wasn't worth doing under time
pressure just to fix a cosmetic backup filename. `/tasks` database
backup tasks can set their own file name prefix instead (see
`CHANGELOG.md`, 2026-07-09) as a workaround that doesn't touch the
schema.

## `rdxport.cgi`'s CREATETICKET accepts any password for a user with no password set

Confirmed by direct testing 2026-07-09: `RDXPORT_COMMAND_CREATETICKET`
returns a valid ticket for the `admin` Rivendell account regardless of
what password is submitted -- an obviously wrong password gets a
`200`/valid `<ticketInfo>` response, same as the real one would. This
is a real authentication bypass in Rivendell's own C++ web API, not
anything specific to this fork's own code, and it's still live: any
client that calls `rdxport.cgi` directly (or through code that still
forwards a user-submitted password to it) is affected.

`rivapi`'s dashboard and JSON API logins, and `/mode`'s
re-authentication gate, no longer depend on this for access control
(see `CHANGELOG.md`, 2026-07-09, and `auth.CheckDashboardPassword`'s
own comment in `rivapi/auth/auth.go`) -- they check a fixed dashboard
credential (`/etc/rd.conf`'s `[dashboard] JwtSecret`) directly instead,
and only call `CreateTicket` afterward, with that same value as the
ticket password, purely to obtain a real ticket for features that need
one. That closes the actual access-control hole those specific
callers had, but it works around the underlying bug rather than fixing
it -- `rdxport.cgi` itself still accepts any password for this
account, and anything else that talks to it directly (RDAirplay, other
`web/rdxport/` clients, a future integration) is still exposed.

Root cause not fully traced. The likely mechanism, not yet confirmed
against the actual stored value: `lib/rduser.cpp`'s
`RDUser::authenticated()` builds its password-match SQL clause as
`` `PASSWORD`='<base64 of the submitted password>' ``, matching how
`RDUser::setPassword()` stores passwords (plain base64, not a real
hash) -- but `scripts/rd_create_db` seeds the default `admin` row via
a raw SQL `INSERT` using MySQL's own `PASSWORD()` function on an empty
string (`RDA_PASSWORD=` is blank by default), a completely different
encoding than `RDUser`'s own convention. If those two paths produce
values that don't reliably fail to match through Qt/MySQL's string
comparison the way an actually-wrong password should, that would
explain the symptom -- but this needs an actual `SELECT PASSWORD,
HEX(PASSWORD) FROM USERS WHERE LOGIN_NAME='admin'` against a real
install to confirm before treating it as the fix, not just the theory.

## Dashboard nav says "Streaming", everything underneath still says "Broadcast"

The `/broadcast` route, its handlers (`handlers_broadcast.go`,
`BroadcastSave`), the persisted config type (`BroadcastConfig`,
`LoadBroadcastConfig`/`SaveBroadcastConfig`), the on-disk file it's
stored in (`/home/rd/etc/rivolution/broadcast.json`), the
`RIVAPI_BROADCAST_CONFIG` env var, and the export/import bundle's
`"broadcast"` JSON key all still say "broadcast" -- only the two
visible nav labels (main nav and the home page's quick-link tile) were
changed to "Streaming", 2026-07-09.

Deliberately left everything else alone: renaming the Go identifiers,
handler file names, and routes is a mechanical, low-risk refactor
whenever someone wants to spend the time on it, but the on-disk config
file name, env var, and export-bundle JSON key are a different kind of
change -- every existing install already has a `broadcast.json` on
disk and possibly saved export bundles keyed `"broadcast"`, so renaming
those needs an actual migration path (or a back-compat alias), not
just a search-and-replace, or existing installs silently lose their
saved config on upgrade. Same reasoning already applied to the
`liquidsoap.*`/`liq_*` field names kept in `BroadcastConfig` itself
after the ffmpeg swap (see `CHANGELOG.md`, 2026-07-09).

If this mismatch (nav says one thing, everything else says another)
ends up being more confusing than the original wording, worth doing
the full internal rename in one pass -- it's roughly 110 occurrences
across 18 files by a straight grep, but all mechanical, no persisted
data involved outside the three places named above.

## `postinst`'s fresh-vs-upgrade guard trusts a single signal (`OLD_VERSION`) that can be fooled by an intervening purge

`debian/postinst`'s destructive block (`DROP DATABASE`, regenerate the
MySQL password, regenerate the dashboard JWT secret, `rddbmgr --create
--generate-audio`) is gated entirely on `test -z "$OLD_VERSION"`
(`$2`, dpkg's own upgrade-vs-fresh-install argument). This is exactly
what fired incorrectly on `acid-rack`, 2026-07-10: `rivolution` had
been `apt purge`d over an hour before the actual reinstall, which
resets dpkg's own tracked "previously configured version" state, so
the guard saw an empty `$2` and correctly-by-its-own-logic (but
disastrously in practice) treated a real upgrade as a genuinely fresh
install. The guard itself was never buggy; the problem is that it only
has one signal, and that signal is decoupled from whether real data
actually exists.

**Proposed fix, not yet built:** before the destructive block runs
(i.e. before `mysql_pass`/`jwt_secret` get regenerated and written
into `/etc/rivendell.d/rd-default.conf`, `postinst` lines ~146-150),
query MySQL directly for whether a database matching the *current*
`/etc/rd.conf`'s `[mySQL] Database=` value already exists **and
actually contains tables** (an empty just-`CREATE DATABASE`'d shell
from a partial/failed prior attempt shouldn't count) --
`information_schema.tables` row count, not just schema-name
existence. This needs no new credentials: the existing `mysql -h
"$mysql_host"` calls in this same block already connect with no
explicit user/password, relying on MariaDB's `unix_socket` auth for
`root@localhost`, which works passwordless for anything running as
root -- the same mechanism a pre-check could reuse.

Then require **both** `OLD_VERSION` empty *and* no real populated
database found before treating an install as genuinely fresh. If the
two signals disagree -- `OLD_VERSION` empty but a real, populated
database exists (exactly the 07-10 scenario) -- abort the install with
a loud, explicit error instead of guessing which signal to trust,
forcing a human to actually look at it before anything destructive
runs. `/etc/rd.conf`'s own existence (already checked earlier in this
same script, line 120) is a third, already-computed, free signal that
could fold into the same combined check.

**Confirmed requirement, not just a caveat to think about later:** a
genuinely intentional clean wipe (fresh testing, deliberately starting
over) would now be blocked by this same check, and Brandon confirmed
2026-07-17 that the escape hatch itself is a definite part of this fix,
not an optional nice-to-have -- an env var or a flag file checked
before the guard, so the safety net can be deliberately overridden on
purpose but never silently bypassed by accident (e.g. by an
intervening purge, the actual 07-10 failure mode). Both halves --
the double-signal check and the explicit override -- ship together or
not at all; a version of this fix without the escape hatch just trades
one footgun for another.

Not fixed yet: Brandon flagged this 2026-07-17 as worth strengthening,
explicitly deferred to a future session rather than done live against
`postinst` the same night as three other production changes.

## Tailscale dashboard "Network" page — spec 0014's other half, still fully unbuilt

`docs/specs/0014-tailscale-integration.md` designs Tailscale as three
pieces: an Ansible role (install + enable, opt-in), a dashboard
"Network" page (auth-key activation, MagicDNS/status display, TLS cert
provisioning via `tailscale cert`), and TLS-serving support in `rivapi`
itself. Confirmed 2026-07-20: only the first piece exists
(`rivolution-unified-installer`'s `roles/tailscale`, shipped the same
day). The dashboard half has no route, no handler file, and no
`conf/sudoers.d/rivapi` entries for `tailscale up`/`cert`/`status` —
`base.html`'s nav only lists System/Streaming/Patchbay/Mode/Tasks/
Backup, nothing Tailscale-related.

**Needed, not yet built:** the actual dashboard page — auth-key paste +
activate, MagicDNS/status display, TLS cert provisioning — plus the
matching `rivapi` sudoers grant and `RIVAPI_TLS_CERT`/`RIVAPI_TLS_KEY`/
`RIVAPI_TLS_CERT_DIR` env var support spec 0014 already calls for.
Flagged as a priority for the `rc1-3` candidate.

## Ubuntu Applications Menu still says "Rivendell", not "Rivolution"

The 2026-07-17 desktop-menu investigation fixed a real, narrow bug
(RDMonitor's shortcut landing in a generic "Other" category instead of
with the rest of the Rivendell/Rivolution tools — see `CHANGELOG.md`)
but didn't touch this: the actual top-level Applications Menu
**category/submenu itself** (Applications -> Rivendell -> ...) is still
literally labeled "Rivendell," not "Rivolution" — every rebranded icon
and tool still files under the old name at the menu-navigation level.
Flagged by Brandon 2026-07-20 as a priority for the `rc1-3` candidate.

**Needed, not yet built:** find and update wherever the submenu's
display name is actually declared (likely `xdg/rivendell-rivendell.menu`
and/or a `.directory` file defining the category's `Name=`) and rename
it to Rivolution, consistent with everything else the 2026-06-24 icon/
branding pass already covers. Not yet investigated in detail -- start
from the same `xdg/*.menu`/`.desktop` files the RDMonitor fix already
traced.
