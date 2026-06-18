# Changelog

Notable changes to the Rivendell v6 fork. Newest entries first.

Pre-fork history (through 2026-06-15) is preserved unchanged in
`ChangeLog.upstream-v4`, which is no longer appended to.

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
