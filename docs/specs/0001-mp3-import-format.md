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
- The **host/cut format mini-enum** — a separate, persisted enum used
  everywhere a host's or cut's format is actually stored. Confirmed from
  the real, shipped UI code in `rdadmin/edit_rdlibrary.cpp:385-521`
  (dropdown-index-to-stored-value translation) and matched by
  `web/rdxport/import.cpp:181-193`:
  `0 = WAV (Pcm16)`, `1 = MPEG Layer II in a WAV wrapper (MpegL2Wav)`,
  `2 = PCM24`.

**Correction (caught mid-implementation):** an earlier pass at this spec
missed the existing `case 2` in `web/rdxport/import.cpp` (PCM24) and
proposed assigning MP3 to mini-enum value `2`. That value is already
taken in real, shipped code — reusing it would make a stored
`CODING_FORMAT=2` mean PCM24 to one consumer and MP3 to another. **MP3
gets value `3`**, the first genuinely free slot. `utils/rdimport/rdimport.cpp`'s
own format switch was, separately, missing a `case 2` for PCM24 entirely
(a pre-existing gap vs. the web path and the RDAdmin UI) — fixed
alongside this change since it directly affects which values the CLI
needs to handle correctly.

The mini-enum is the canonical on-disk value — `RDCart::addCut()`
(`lib/rdcart.cpp:1319-1340`) writes it directly into `CUTS.CODING_FORMAT`
with no translation, and every consumer of `RDCut::codingFormat()`
(`lib/rdcut.cpp:537`) re-decodes it with its own small switch, e.g.
`rdlibrary/record_cut.cpp:84-96`.

## Feature A — selectable format

| Layer | File | Change |
|---|---|---|
| CLI flag | `utils/rdimport/rdimport.cpp` ~638-670 | Add `--audio-format=<n>` parsed identically to the adjacent `--segue-level=`/`--autotrim-level=` blocks. Accepts `0`, `1`, `2`, or `3`. |
| Format switch (web) | `web/rdxport/import.cpp` ~181-193 | Add `case 3` alongside the existing `0`/`1`/`2`. Source value is `conf->defaultFormat()` by default, overridable by an optional new POST field (`FORMAT`) read via `xport_post->getValue(...)`. **This is the layer that actually decides the encoded format and bitrate for every import** — see note below. |
| HTTP client | `lib/rdaudioimport.h`/`.cpp` | New `setFormat(unsigned)` method + `conv_format`/`conv_format_set` members; `runImport()` adds a `FORMAT` `curl_formadd(...)` only when explicitly set, so every other caller of this shared class (RDLibrary's import dialog, CD/disk rippers) gets zero behavior change. |
| CLI → client wiring | `rdimport.cpp` ~1450-1453 | `conv->setFormat(import_format)` right alongside the existing `conv->setCartNumber()`/`setSourceFile()` calls. |

**Correction (caught mid-implementation, second pass):** the local
`switch(import_format)` block in `rdimport.cpp` that sets
`RDSettings::Format` on a local `RDSettings` object — including the
pre-existing `case 0`/`case 1` — turns out to be **inert**. Traced
`RDAudioImport::runImport()` (`lib/rdaudioimport.cpp:107-162`) and
confirmed by exhaustive grep: it only ever reads `channels()`,
`normalizationLevel()`, and `autotrimLevel()` off that settings object,
never `format()` or `bitRate()`. `rdimport` never converts audio locally
— it always uploads the raw source file to `rdxport.cgi` over HTTP via
libcurl, and the server (`import.cpp`) decides the actual destination
format and bitrate independently. This means an earlier version of this
spec's "fix" (adding `settings->setBitRate(import_bitrate)` to that local
switch, believing it fixed a missing-bitrate bug for LAME) was based on
an incomplete trace and has been reverted — there was no bug; bitrate is
correctly computed server-side from `conf->defaultBitrate()` regardless
of what the client does locally. The local switch itself was left as-is
(now with `case 2`/`case 3` added for completeness/consistency with the
already-dead `case 0`/`case 1`, in case anything ever starts reading
`conv_settings->format()`), but the **real** mechanism that makes format
selection take effect is the new `setFormat()`/`FORMAT` POST field path
above.
| Host default UI | `rdadmin/edit_rdlibrary.cpp:385-521` | Add a fourth "MPEG Layer III (MP3)" entry to `lib_format_box`, following the exact dropdown-index-to-stored-value translation pattern already used for PCM16/PCM24/MPEG L2, including enabling the bitrate box for it (mirrors the existing MPEG L2 bitrate-enable logic at line 524). |
| Schema (new column) | `updateschema.cpp` (new `cur_schema<378` block), `revertschema.cpp` (downgrade), `lib/dbversion.h` (`RD_VERSION_DATABASE` 377→378) | Add `CODING_FORMAT int default -1` to `DROPBOXES` via `alter table`, following the exact precedent of the `DROP_BOX_SCAN_COUNT`/`DROP_BOX_SCAN_INTERVAL` version-377 migration immediately preceding it. `-1` = "use host default". **`create.cpp` is deliberately NOT touched** — it has an explicit maintainer comment ("DO NOT alter the schema in this method!") confirming fresh installs go through `Create()` then `Modify()`→`UpdateSchema()`, so the new column is picked up automatically via the same migration path used for upgrades. |
| Dropbox spawn | `rdservice/startup.cpp:227-289` (`MainObject::StartDropboxes()`) | Add `CODING_FORMAT` to the `SELECT`; if value is `0`, `1`, `2`, or `3`, append `--audio-format=N` to the `rdimport` args, mirroring the existing conditional pattern used for `TO_CART`/`SEGUE_LEVEL`. |
| Dropbox UI | `rdadmin/edit_dropbox.cpp` (+ its `.ui`) | Add a "Target Audio Format" dropdown (Use Host Default / PCM16 / MPEG Layer II / PCM24 / MPEG Layer III) saved to the new `CODING_FORMAT` column. Includes PCM24 for symmetry with the host-level dropdown — no extra cost once the control exists. |
| Cut format decode | `rdlibrary/record_cut.cpp:84-96` | **Resolved, no change.** `RDCae::AudioCoding` (`lib/rdcae.h:62`) does define `MpegL3=3`, but `cae/caed.cpp` never references `MpegL3`/`AudioCoding` anywhere — confirming the daemon's actual live-recording encode support would require tracing its full RML/IPC protocol, a different subsystem from file import and out of scope here. Left the existing `default: Pcm16` fallback untouched; it already handles the unrecognized value safely if a cut imported as MP3 is later re-recorded over. Note: this switch also lacks a PCM24 case today (pre-existing gap, unrelated to this patch, not touched). |

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

**Correction (caught while implementing):** this spec originally placed
the bypass logic inside `rdimport.cpp`'s per-file import loop. That's
wrong for the same reason the local format switch was inert (see Feature
A correction above) — `rdimport` never converts audio itself; it always
uploads the raw source file to `rdxport.cgi` and the **server**
(`import.cpp`) performs the actual conversion. So passthrough has to be
implemented server-side.

Implemented as:

- `rdimport.cpp`: new `--passthrough` flag → `import_passthrough` bool →
  `conv->setPassthrough(import_passthrough)` (mirrors the `setFormat()`
  wiring).
- `lib/rdaudioimport.h`/`.cpp`: new `setPassthrough(bool)` +
  `conv_passthrough` member; `runImport()` sends a `PASSTHROUGH` POST
  field only when set, so every other caller is unaffected.
- `web/rdxport/import.cpp`: reads the optional `PASSTHROUGH` field.
  After opening the uploaded file with `RDWaveFile` (already done today
  for metadata), captures `wave->getHeadLayer()==3` — the actual decoded
  MPEG audio layer, not just a container/extension guess — as
  `source_is_mp3` *before* deleting that probe object. Passthrough is
  honored only when `passthrough_requested && source_is_mp3 &&
  (effective_format==3)`; otherwise it's silently ignored and the normal
  `RDAudioConvert` path runs exactly as before. When honored: skip
  `RDAudioConvert` entirely, `QFile::copy()` the uploaded file straight
  to `RDCut::pathName(cartnum,cutnum)`, re-open *that* file with
  `RDWaveFile` to get `msecs` (mirroring the existing post-convert
  duration read), and use the already-probed `wavedata` for cart/cut
  metadata instead of `conv->sourceWaveData()` (there is no `conv` object
  in this branch).
- **Conflict rule (decided):** if passthrough is honored and
  `autotrim_level!=0` was requested for the same file, log a warning via
  `rda->syslog()` that the level is being ignored (audio-level changes
  require decoding, which passthrough explicitly skips) and continue the
  import via passthrough rather than failing it. `normalization_level`
  is handled the same way implicitly — it's simply never applied in the
  passthrough branch.

## Open items for implementation time

- Re-grep for `codingFormat()`/`CODING_FORMAT` consumers immediately
  before implementing, in case anything has changed since this spec was
  written.
- Confirm whether `RDCae` has any MP3 recording target before deciding
  whether `record_cut.cpp` gets a real `case 2` or keeps the safe
  `default:` fallback.
