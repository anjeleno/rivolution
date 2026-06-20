# 0003 — MP3 waveform/peak energy

**Date:** 2026-06-20

## Goal

MP3 (MPEG Layer III) cuts show no waveform in RDLibrary today —
`RDWaveFile::LoadEnergy()` has no decode path for Layer III, so
`hasEnergy()` returns `false` and nothing is ever drawn. This makes
placing segue/talk markers on MP3 content a guess-by-ear exercise.
Goal: real, persisted peak data for MP3 cuts, computed once and
displayed exactly like PCM/Vorbis/Layer II already are — no special
case visible to RDLibrary, RDWavePainter, or any other display code.

## Background — verified against source, not assumed

### Current energy computation and its real persistence mechanism

`LoadEnergy()` (`lib/rdwavefile.cpp:4439-4553`) computes peak magnitude
per 1152-sample block per channel for PCM16, PCM24, and Vorbis; for
MPEG it only reads *pre-existing, third-party-embedded* ancillary
energy data on Layer II files (a separate, opportunistic-read-only
mechanism, not something Rivendell generates). Layer III hits the
default case and returns `false`.

The actual mechanism Rivendell uses to **persist its own computed**
energy data is the **LEVL chunk** — written by `closeWave()`
(`rdwavefile.cpp:644-679`) whenever Rivendell itself finalizes a wave
file, read back by `GetLevl()` on a later open. The write is gated:

```cpp
if(levl_chunk&&((format_tag==WAVE_FORMAT_PCM)||
                ((format_tag==WAVE_FORMAT_MPEG)&&(head_layer==2)))) {
```

This is persisted **inside the file's own RIFF/WAV structure**, never
the SQL database. There is no existing database-backed energy storage
to extend.

### MP3 storage today: never WAV-wrapped

Every MP3 file Rivendell produces today is a bare MPEG elementary
stream, with no RIFF/WAV container at all — regardless of how it got
there:

- **Passthrough import** (`web/rdxport/import.cpp:256`): `QFile::copy()`
  — a raw byte copy of the uploaded file, no container.
- **LAME-encoded output** (`lib/rdaudioconvert.cpp:1492-1555`,
  `Stage3Lame()` or equivalent): raw POSIX `open()`/`write()` to a bare
  file descriptor, no `RDWaveFile::createWave()`/`closeWave()`
  involvement at all.

This is *why* the LEVL mechanism can't apply to MP3 as things stand —
there's no RIFF container to write a chunk into, even in principle.

### WAV-wrapped MP3 already has first-class support — this isn't new

Upstream commit `bbeff9f3` (2014-08-27, Fred Gleason): *"Added support
for RIFF WAV files with WAVE_FORMAT_MPEGLAYER3 format."* Confirmed
still live: `lib/rdwavefile.cpp:2640-2647` — when a WAV file's `fmt `
chunk declares `WAVE_FORMAT_MPEGLAYER3` (`0x0055`,
`rdwavefile.h:535`), `RDWaveFile` calls `GetMpegHeader()` to sniff the
real frame header directly (so `head_layer` correctly becomes `3` from
the actual bitstream, not the container), then **normalizes
`format_tag` to `WAVE_FORMAT_MPEG` internally**. From that point on, a
WAV-wrapped MP3 is handled identically to a WAV-wrapped Layer II file
everywhere else in the class.

This means the LEVL-write gate above needs exactly one change
(`head_layer==2` → also accept `3`) to make the existing, proven
persistence mechanism work for MP3 — provided MP3 is actually stored
WAV-wrapped rather than bare.

### Confirmed: no shared-instance concurrency risk

Every caller does `new RDWaveFile(...)` independently (`grep` across
`cae/driver_alsa.cpp:776`, `cae/driver_jack.cpp:730`,
`web/rdxport/import.cpp:213`, `export.cpp:151`, etc.) — playback,
import, and display never share one instance. Multiple independent
file descriptors reading the same file concurrently is safe at the OS
level. No locking/thread-safety work needed for this feature.

### Display/GUI layer needs zero changes

`lib/rdwavepainter.cpp`, `lib/rdwavefactory.cpp`, `lib/rdmarkerview.cpp`,
`rdlibrary/record_cut.cpp` all go through `GetEnergy()`/`hasEnergy()`/
`energy()` — already fully abstracted from format. Once `LoadEnergy()`
and the LEVL gate handle Layer III, every consumer picks it up with no
code changes.

## Implementation plan

### 1. Storage format change: WAV-wrap MP3 instead of bare elementary stream

**Passthrough import** (`web/rdxport/import.cpp`): replace the
`QFile::copy()` passthrough path with a write through
`RDWaveFile::createWave()` using `WAVE_FORMAT_MPEGLAYER3`, copying the
existing MPEG frame bytes verbatim into the `data` chunk — **the audio
bitstream itself is still never decoded or re-encoded**, only the
container changes. `createWave()` already has a `WAVE_FORMAT_MPEG`
case (`rdwavefile.cpp:526`) to extend/confirm covers this.

**LAME-encoded output** (`lib/rdaudioconvert.cpp`): same treatment —
route the encoder's output through `RDWaveFile` instead of a raw file
descriptor, so it also ends up WAV-wrapped and LEVL-eligible.

### 2. Energy computation: new case in `LoadEnergy()`

Add a branch for `format_tag==WAVE_FORMAT_MPEG && head_layer==3` in
`lib/rdwavefile.cpp` (~line 4453), decoding via libmad and computing
the same peak-per-1152-sample-block value the PCM/Vorbis branches
already use, for consistent waveform appearance and auto-trim
behavior across formats. Mirror the existing libmad decode pattern in
`RDAudioConvert::Stage1Mpeg()` (`rdaudioconvert.cpp:548-698` —
`mad_stream_buffer()` → `mad_frame_decode()` → `mad_synth_frame()` →
`mad_f_todouble()`) rather than inventing a new one; libmad is already
dynamically loaded (`dlopen("libmad.so.0",...)`,
`rdaudioconvert.cpp:81`) and its function pointers already wrapped
(`rdaudioconvert.h:127-136`).

### 3. Persistence: extend the LEVL-write gate

`rdwavefile.cpp:644-645` — change

```cpp
((format_tag==WAVE_FORMAT_MPEG)&&(head_layer==2))
```

to accept `head_layer==3` as well. Computed energy then persists in
the file's own LEVL chunk exactly as it already does for PCM and
Layer II — read back on next open via `GetLevl()`, no re-decode needed
after the first computation.

### 4. Export-side follow-on (required, not optional) — and why playback is unaffected

Two genuinely different consumers of the stored file, affected
differently:

- **Playback** (`cae/driver_alsa.cpp`'s MPEG path, via `RDWaveFile`):
  no change needed. `RDWaveFile` already parses the WAV wrapper and
  locates `data_start` before handing frame bytes to the decoder —
  exactly what the existing `WAVE_FORMAT_MPEGLAYER3` support already
  does. Playback never sees or cares about the wrapper.
- **`web/rdxport/export.cpp`'s passthrough branch** does not go
  through `RDWaveFile` at all — it's a container-unaware byte-range
  copy (`open()`/`read()`/`write(1,...)` of the stored file from byte
  0 to EOF, labeled `Content-type: audio/x-mpeg`). Once storage is
  WAV-wrapped, this one function would ship the entire wrapped file —
  RIFF header, `fmt ` chunk, and a LEVL chunk that could be hundreds of
  KB on a long file — to whatever external client requested "an MP3
  export," mislabeled as a bare MPEG stream. This needs to change to
  strip the wrapper and stream only the inner MPEG frame data, so a
  passthrough export still hands back a real, standalone, playable
  `.mp3` exactly as it does today. Narrowly scoped to this one
  function; nothing about the passthrough principle (no decode/
  re-encode) or playback is at risk anywhere.

### 5. Tests

`tests/audio_peaks_test.cpp` currently has no MP3 case (no existing
automated test covers `LoadEnergy()` directly for any format). Add one
alongside the implementation.

## Confirmed out of scope

- PCM16/PCM24/Vorbis/Layer II's existing energy computation and LEVL
  behavior — untouched.
- No SQL database schema changes — this stays entirely file-based,
  consistent with how every other format already persists energy data.
- No RDLibrary/GUI changes — the display layer is already abstracted
  through `GetEnergy()` and needs nothing format-specific.
- The MEXT/ancillary-energy-read mechanism for third-party-encoded
  Layer II files — separate, pre-existing, untouched by this work.

## Confirmed decision: no migration path

This is a pre-production dev environment on a private repo, not yet
used anywhere in production — there are no existing MP3 cuts that need
preserving or migrating. Decided explicitly (2026-06-20): no one-time
migration of already-imported MP3 cuts. Any MP3 imported before this
change simply gets a real waveform the next time it's re-imported,
same as the existing behavior for any other format gap.

## Open items for implementation time

- Confirm `createWave()`'s existing `WAVE_FORMAT_MPEG` case
  (`rdwavefile.cpp:526`) handles the `MPEGLAYER3`-specific `fmt ` chunk
  layout correctly, or whether it needs its own case alongside the
  existing one — re-verify against the 2014 commit's actual chunk
  layout at implementation time.
- Re-confirm `RDAudioConvert::Stage3Lame()`'s exact function name/line
  range immediately before implementing (referenced here from search
  results, not re-read in full).
