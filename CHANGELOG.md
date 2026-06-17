# Changelog

Notable changes to the Rivendell v6 fork. Newest entries first.

Pre-fork history (through 2026-06-15) is preserved unchanged in
`ChangeLog.upstream-v4`, which is no longer appended to.

## 2026-06-17

- Added selectable MP3 (MPEG Layer III) as an import coding format,
  alongside the existing PCM16/PCM24/MPEG Layer II options: a new
  `--audio-format=<0|1|2|3>` flag on `rdimport`, a matching override on
  the web import service (`rdxport.cgi`), a per-Dropbox "Target Audio
  Format" setting in RDAdmin, and an MP3 entry in the host-level default
  format dropdown. See `docs/specs/0001-mp3-import-format.md`.
- Added a true passthrough import mode (`rdimport --passthrough`): when
  the source file is already MP3 and the target format is MP3, the
  server copies the file directly instead of decoding and re-encoding
  it through LAME.
- Fixed: `utils/rdimport/rdimport.cpp`'s local format switch was missing
  a PCM24 case (present in the web import path and the RDAdmin UI but
  not the CLI tool) — added for consistency.
- Schema: added `DROPBOXES.CODING_FORMAT` (database schema version
  377 → 378).
