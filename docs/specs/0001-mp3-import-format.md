# 0001 — Selectable MP3 import format + passthrough

**Date:** 2026-06-17

## Goal

Allow MP3 source audio to be imported without being forced through the
host's default WAV profile, in three places: the `rdimport` CLI, Dropbox
configuration, and (separately) a true byte-for-byte passthrough mode that
skips re-encoding entirely.

This spec covers two related but independently mergeable changes:

- **Feature A** — add MPEG Layer III as a selectable coding format
  alongside the two that already exist (WAV, MPEG Layer II/WAV), wired
  through the CLI, the web import service, and Dropboxes.
- **Feature B** — true passthrough: when the source file is already MP3
  and the target format is MP3, copy the file's bytes directly instead of
  decoding and re-encoding through LAME.

## Background: the existing format enum

There are two unrelated enums in this codebase and it's important they
stay unrelated:

- `RDSettings::Format` (`lib/rdsettings.h:31-32`) — the codec engine's
  internal enum: `Pcm16=0, MpegL1=1, MpegL2=2, MpegL3=3, Flac=4,
  OggVorbis=5, MpegL2Wav=6, Pcm24=7`. Used transiently inside
  `RDAudioConvert`/`RDSettings` during a conversion.
- The **host/cut format mini-enum** — a separate, persisted, two-value
  enum used everywhere a host's or cut's format is actually stored:
  `0 = WAV (Pcm16)`, `1 = MPEG Layer II in a WAV wrapper (MpegL2Wav)`.

The mini-enum is the canonical on-disk value — `RDCart::addCut()`
(`lib/rdcart.cpp:1319-1340`) writes it directly into `CUTS.CODING_FORMAT`
with no translation, and every consumer of `RDCut::codingFormat()`
(`lib/rdcut.cpp:537`) re-decodes it with its own small switch, e.g.
`rdlibrary/record_cut.cpp:84-96`.

**Decision:** MP3 gets mini-enum value **2** (the next sequential value),
not `3`. Reusing `3` to match `RDSettings::MpegL3` would make `0` and `3`
look like they correspond directly to `RDSettings::Format` while `1`
silently doesn't (it maps to enum value `6`) — a readability trap, not a
functional one (the switch statements translate explicitly either way,
so there's no actual collision possible). Sequential `2` keeps the
mini-enum visibly its own thing.

## Feature A — selectable format

| Layer | File | Change |
|---|---|---|
| CLI flag | `utils/rdimport/rdimport.cpp` ~638-670 | Add `--audio-format=<n>` parsed identically to the adjacent `--segue-level=`/`--autotrim-level=` blocks. Accepts `0`, `1`, or `2`. |
| Format switch (CLI) | `rdimport.cpp` ~1445-1453 | Add `case 2: settings->setFormat(RDSettings::MpegL3); break;` |
| Format switch (web) | `web/rdxport/import.cpp` ~181-189 | Same new case. Source value is `conf->defaultFormat()` by default, overridable by an optional new POST field (`FORMAT`) read via `xport_post->getValue(...)`. |
| HTTP client | `lib/rdaudioimport.cpp` | Add one more `curl_formadd(...)` call for the new field when an explicit override was requested, mirroring the existing `NORMALIZATION_LEVEL`/`AUTOTRIM_LEVEL` additions. |
| Host default UI | `rdadmin/edit_rdlibrary.cpp` | Add an "MPEG Layer III (MP3)" entry to the existing format dropdown. |
| Schema (new column) | `utils/rddbmgr/create.cpp:1156` (fresh installs), `updateschema.cpp` (migration), `revertschema.cpp` (downgrade) | Add `CODING_FORMAT int(11) default -1` to `DROPBOXES`, following the exact precedent of `SEGUE_LEVEL`/`SEGUE_LENGTH` in those same three files. `-1` = "use host default", matching existing fall-through behavior for other per-dropbox settings. |
| Dropbox spawn | `rdservice/startup.cpp:227-289` (`MainObject::StartDropboxes()`) | Add `CODING_FORMAT` to the `SELECT`; if value is `0`, `1`, or `2`, append `--audio-format=N` to the `rdimport` args, mirroring the existing conditional pattern used for `TO_CART`/`SEGUE_LEVEL`. |
| Dropbox UI | `rdadmin/edit_dropbox.cpp` (+ its `.ui`) | Add a "Target Audio Format" dropdown saved to the new `CODING_FORMAT` column. |
| Cut format decode | `rdlibrary/record_cut.cpp:84-96` | Add `case 2: rec_format=RDCae::MpegL3; break;` *if* `RDCae` exposes an MP3 recording target — otherwise leave the existing `default: Pcm16` fallback in place. This path is live recording (mic/CD-rip), not file import; out of scope to extend live-record-to-MP3 capability as part of this patch if it doesn't already exist. Flagging only so a cut imported as MP3 and later re-recorded over doesn't silently misbehave — today's `default:` fallback already handles unknown values safely. |

**Confirmed out of scope / not touched:** `cae/caed` does not reference
`CODING_FORMAT` at all — on-air playback decodes cuts by their actual
file contents, not this field, so no playout-engine changes are needed.
`importers/rivendell_filter.cpp` just passes an existing `CODING_FORMAT`
value through unchanged (cart-to-cart copy tool). `importers/wings_filter.cpp`
is an unrelated legacy import filter for a different automation system;
not part of the rdimport/Dropbox/web-import path this feature touches.

**Backward compatibility:** `DROPBOXES.CODING_FORMAT` defaults to `-1` on
migration, meaning "don't emit `--audio-format`, behave exactly as
today." Existing installs see zero behavior change after upgrading.

## Feature B — true passthrough

Re-encoding an already-compressed MP3 through `RDAudioConvert`'s
decode→PCM→LAME-encode pipeline (`lib/rdaudioconvert.cpp:148-223`) is
lossy generational recompression, not a passthrough. There is currently
no shortcut in that pipeline for "source format already matches
destination, just copy."

- New `--passthrough` flag on `rdimport`.
- In the per-file import logic (`rdimport.cpp` ~1416-1480): if the source
  file is detected as MPEG (already known via the existing
  `wavefile->openWave()` / `RDWaveFile::Mpeg` type probe — no new
  detection code needed) **and** the effective target format is MP3
  (mini-enum value `2`) **and** `--passthrough` is set, bypass
  `RDAudioImport`/`RDAudioConvert` entirely: copy the source file's bytes
  directly to the destination path, and write duration/metadata from the
  same probe that already reads MP3 headers today (no new metadata code
  needed).
- **Conflict rule (decided):** if `--passthrough` is combined with a
  non-zero `--autotrim-level` or `--normalization-level` for the same
  file, log a warning that the level is being ignored (audio-level
  changes require decoding, which passthrough explicitly skips) and
  continue the import via passthrough rather than failing it.

## Open items for implementation time

- Re-grep for `codingFormat()`/`CODING_FORMAT` consumers immediately
  before implementing, in case anything has changed since this spec was
  written.
- Confirm whether `RDCae` has any MP3 recording target before deciding
  whether `record_cut.cpp` gets a real `case 2` or keeps the safe
  `default:` fallback.
