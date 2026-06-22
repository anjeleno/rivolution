# Changelog

Notable changes to the Rivendell v6 fork. Newest entries first.

Pre-fork history (through 2026-06-15) is preserved unchanged in
`ChangeLog.upstream-v4`, which is no longer appended to.

## 2026-06-21

- Fixed: MP3 passthrough (import) ignored a Dropbox's configured
  normalization/autotrim level whenever the source was already MP3 and
  the target format was also MP3 — the only acknowledgment was a syslog
  warning, never actually applied. Normalization/autotrim now requires
  falling through to the full decode/process/re-encode path, since
  neither is possible on a byte-for-byte passthrough copy. See
  `docs/specs/0003-mp3-waveform-energy.md`.
- Fixed two more bugs in the new MP3 waveform/peak energy feature, found
  during pre-build review: peaks computed during MP3 import/encoding
  could be undercounted (a signed-value comparison ignored negative-going
  excursions), and a same-format passthrough import could persist a
  permanently-empty peak chunk, leaving that cut's waveform blank
  forever with no recovery. See `docs/specs/0003-mp3-waveform-energy.md`.
- Fixed generated helper scripts (`helpers/install_python.sh`,
  `helpers/rdi18n_helper.sh`, `xdg/install_usermode.sh`, `build_debs.sh`)
  losing their executable bit whenever `make` triggers automake's
  per-file regeneration via `config.status`, instead of only a full
  `./configure` run. The `chmod` is now part of each file's own
  `AC_CONFIG_FILES` recipe in `configure.ac`, so it reruns on every
  regeneration path.

## 2026-06-20

- Added real MP3 (MPEG Layer III) waveform/peak energy display: actual
  decoded peak data via `libmad`, persisted to the file's own `LEVL`
  chunk so repeat views don't re-decode from scratch. Previously MP3
  cuts had no real waveform in "Edit Markers" at all. See
  `docs/specs/0003-mp3-waveform-energy.md`.

## 2026-06-18

- Fixed: MP3 passthrough (import and export) could produce a file
  whose real sample rate doesn't match the system's output rate,
  which `caed`'s MPEG playback path doesn't resample — audible as
  pitch/speed-shifted ("helium") playback. Passthrough now requires
  the source's real sample rate to match the system rate; otherwise it
  falls through to the existing, correct conversion path. See
  `docs/specs/0001-mp3-import-format.md`.

## 2026-06-17

- Added segue back-timing: when the outgoing element in a segue has
  "No fade on segue out" checked, the next element's start is now
  delayed (when needed) so its intro lands exactly when the outgoing
  element's tail finishes, instead of firing instantly at the segue
  marker regardless of how much lead-in the next element has. No
  effect when "No fade on segue out" is unchecked. See
  `docs/specs/0002-segue-backtiming.md`.
- Added selectable MP3 (MPEG Layer III) as an import coding format,
  alongside the existing PCM16/PCM24/MPEG Layer II options: a new
  `--audio-format=<0|1|2|3>` flag on `rdimport`, a matching override on
  the web import service (`rdxport.cgi`), a per-Dropbox "Target Audio
  Format" setting in RDAdmin, and an MP3 entry in the host-level default
  format dropdown. See `docs/specs/0001-mp3-import-format.md`.
- Added a true passthrough import mode: whenever the source file is
  genuinely MP3 and the target format is also MP3, the server always
  copies the file directly instead of decoding and re-encoding it
  through LAME — unconditional, no flag or setting needed, since
  there's never a reason to re-encode an MP3 to MP3.
- Fixed: `utils/rdimport/rdimport.cpp`'s local format switch was missing
  a PCM24 case (present in the web import path and the RDAdmin UI but
  not the CLI tool) — added for consistency.
- Schema: added `DROPBOXES.CODING_FORMAT` (database schema version
  377 → 378).
- Fixed: the new "Target Audio Format" label on the Dropbox editor was
  clipped on its left edge (right-aligned text in a box too narrow for
  it) — widened and shifted the dropdown over to make room.
- Fixed: passthrough import failures (e.g. a write/access error) showed
  the nonsensical "Audio Converter Error: OK" instead of a real message,
  because the new error-exit calls left out the audio converter error
  code. Now reports a correct error.
- Added the same MP3-to-MP3 passthrough optimization to audio export
  (RDLibrary's per-cut "Import/Export" dialog, and anywhere else that
  uses the `rdxport.cgi` export service): exporting an already-MP3 cut
  back to MP3 now copies the file directly instead of re-encoding it,
  as long as the export is a plain, full-length, unmodified copy (no
  trimming, forced-length speed adjustment, normalization, or embedded
  metadata requested — any of those still go through the normal export
  path exactly as before).
