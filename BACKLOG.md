# Backlog

Known technical debt and deferred fixes — things we've found, scoped,
and deliberately decided not to fix yet, with the reasoning for why.
This is **not** a feature roadmap or pipeline of planned work; see
`docs/specs/` for that. Entries here get promoted to a real spec and
branch once they're picked up.

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
well under the 20-minute mark. See `KNOWN_ISSUES.md` for the
user-facing version of this.

**Needs to be fixed before any public install** of this fork — an
unfamiliar submitter or station can't be relied on to always stay
under the timeout. Not urgent for current single-station use.

## caed's MPEG playback path doesn't resample mismatched-rate audio

Full trace and planned fix shape: see "Known issue, deferred" in
`docs/specs/0001-mp3-import-format.md`. Short version: `caed`
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
scheduler to skip them automatically. See `KNOWN_ISSUES.md` for the
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

**Current mitigation:** none. See `KNOWN_ISSUES.md` for the user-facing
version.

**Deferred for now** — not investigated yet, but tracked here given the
priority (dead air on a live broadcast). See `ROADMAP.md` for the
related feature request (a library-wide missing-audio audit tool) that
came up alongside this report — distinct from this bug fix itself.

## Edit Markers waveform goes blank at maximum zoom near end-of-file

**Flagged 2026-06-22.** Long-standing, pre-existing Rivendell v4
behavior — not introduced by this fork.

In "Edit Markers" (used to view a cut's waveform and place segue
markers), zooming all the way in while positioned near the end of the
file doesn't peg the waveform at the highest zoom level the way it
does everywhere else in the file — instead the display goes blank,
showing empty space rather than the actual waveform at that zoom
level. Working around it means staying two or three zoom steps back
from maximum, which costs real precision when placing a segue marker
right at a file's tail.

Not yet investigated — no file/line citations yet for the zoom/
rendering logic responsible (likely in the waveform widget's
end-of-file boundary handling, where the visible window's pixel-to-
sample mapping runs past the actual sample count at high zoom).

**Current mitigation:** manually zoom out two or three steps from
maximum when placing markers near the end of a file.

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
