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
