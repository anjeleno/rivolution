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

**Why `format=MpegL3` keeps meaning "real, standalone MP3," not
negotiable here:** `RD_Export`'s `format` parameter is part of
Rivendell's documented, externally-facing Web API
(`docs/rivwebcapi/rd_export.xml`) — not an internal-only call. Any
third-party integrator calling this API today with `format=MpegL3` is
relying on getting back literal MP3 bytes, because that's what the
documented format name means. Changing what that specific, named,
already-public format value returns would silently break that contract
for any unknown external caller. The wrapped-file-with-embedded-peaks
benefit is real, but it doesn't require bending this contract to get
it — see below.

### 5. Future extension (not in this pass): a distinct `MpegL3Wav` export option

A WAV-wrapped MP3 with its LEVL chunk intact is genuinely more useful
than a bare MP3 for system-to-system transfer — any Rivendell-aware
consumer that already has direct access to the audio store (a sync,
backup, or migration tool) gets this for free once storage is
WAV-wrapped, with no API involved. For HTTP `RD_Export` callers who
specifically want that richer file instead of a bare one, the right
shape is a **new, distinct, opt-in format value** — not a change to
what `MpegL3` already means.

The naming convention for exactly this already exists in this
codebase: `RDSettings::Format` (`lib/rdsettings.h:31-32`) already has
`MpegL2Wav=6` alongside `MpegL2=2`. A new `MpegL3Wav` value would
follow that precedent.

**Important distinction, checked against the existing code rather than
assumed:** `MpegL2Wav` is *not* a passthrough option today — confirmed
at `web/rdxport/export.cpp:200-213`, requesting it runs the full
`RDAudioConvert::convert()` transcode pipeline, a general "encode to
this target format from any source" path. Mirroring that fully for
`MpegL3Wav` is **not trivial**: Rivendell has no MP3 *encoder* path
that writes through `RDWaveFile` at all (LAME's output is raw
`open()`/`write()`, bypassing the WAV-wrapping machinery entirely —
see item 1 above — and even fixing that only covers encoding from PCM,
not a generic "wrap arbitrary source content as Layer III" target,
which doesn't exist).

What *would* be small, once the core mechanism in this spec is built
and verified: a narrower, passthrough-only version of `MpegL3Wav` —
when requested *and* the cut is already stored WAV-wrapped (true for
any MP3 imported after this spec lands), serve the raw stored bytes
exactly as today's `MpegL3` passthrough does, just without the unwrap
step. Same byte-copy loop already in this spec, gated on a different
format value. If the cut isn't already in that shape, there's no
sensible fallback — no encode target exists to produce one on demand.

Deliberately sequenced *after* the rest of this spec, not alongside
it — it depends entirely on the storage-wrapping and LEVL-extension
work actually landing and being verified correct first.

### 6. Tests

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

## Implementation deviations from this spec

Anything that ends up different from what's written above, with the
reasoning, goes here — so there's a real reference point if this needs
revisiting later, rather than relying on chat history.

- **On-disk format code: `WAVE_FORMAT_MPEG` (`0x0050`), not
  `WAVE_FORMAT_MPEGLAYER3` (`0x0055`).** `createWave()` only has a
  `WAVE_FORMAT_MPEG` case (`rdwavefile.cpp:530-532`) — there's no
  separate `MPEGLAYER3` case to use. The existing Layer II encoder path
  (`rdaudioconvert.cpp:1616-1635`, the `TwoLame` destination) already
  establishes the precedent for this codebase: write
  `format_tag=WAVE_FORMAT_MPEG` directly with `setHeadLayer()` set
  explicitly, rather than using the `MPEGLAYER3` code at all.
  `GetFmt()`'s read-back path (`rdwavefile.cpp:2630`) then reads
  `head_layer` straight from the embedded `fmt ` chunk bytes for
  `WAVE_FORMAT_MPEG` — no frame-sniffing needed, since this code wrote
  those bytes correctly itself. The `MPEGLAYER3` code (`0x0055`) stays
  relevant only for *reading* files some other, external encoder
  produced with that code — `GetFmt()` still frame-sniffs and
  normalizes those to `WAVE_FORMAT_MPEG` internally, per the original
  2014 support. Functionally equivalent either way for everything that
  matters here; this is a closed, verified decision now, not still
  open.
- **No `cart`/`bext`/`mext`/`rdxl` chunks embedded in the new
  WAV-wrapped file.** Checked against every existing import path before
  deciding, not assumed: `web/rdxport/import.cpp`'s normal
  (non-passthrough) `RDAudioConvert`-based path never calls
  `setDestinationWaveData()`, `setDestinationRdxl()`,
  `setCartChunk()`, or `setBextChunk()` either — for every import path
  that exists today, PCM or Vorbis included, cart/cut metadata lives
  only in the SQL database, never embedded in the file. Leaving them
  off here matches that existing precedent exactly rather than
  introducing new behavior. `mext` specifically isn't a real option at
  all for Layer III — it's the Layer-II-specific ancillary-energy
  byte-position convention from item 1 above, with no defined meaning
  outside Layer II. Decided explicitly (2026-06-20), after being raised
  as an open question rather than decided unilaterally.

  **Declined for now, left here for potential future expansion** —
  two narrower options exist if file-embedded metadata is ever wanted
  for MP3 imports specifically (this would still be new behavior versus
  every other import path, not something to slip in quietly):
  - *Add `cart` + `bext` only*: embeds Rivendell's own cart/cut fields
    plus the industry-standard Broadcast Wave Format chunk, so the
    metadata travels with the file if it's copied or exported
    elsewhere. Both already have working setters
    (`setCartChunk()`/`setBextChunk()`) and populate from `wavedata`,
    which `import.cpp` already parses from the source file — no new
    parsing needed, just two setter calls before `createWave()`.
  - *Add `cart` + `bext` + `rdxl`*: also includes a full XML snapshot
    of the cart's database record, mirroring what
    `export.cpp:132` already does on the export side
    (`cart->xml(true,...)` via `setDestinationRdxl()`). The more
    complete option, and the largest deviation from current import
    behavior of the three.

- **New public method: `RDWaveFile::updateEnergy(const int16_t *pcm)`.**
  Item 1's LAME-encoder change (`RDAudioConvert::Stage3Layer3()`)
  needed a way to measure energy from the *source* PCM as LAME
  consumes it, rather than by decoding the encoded MP3 back afterward
  (chosen explicitly over the redecode approach: more efficient, no
  redundant decode pass, and measures the true source signal rather
  than the lossy-recompressed one). `RDWaveFile` had no existing way
  for an external caller to feed it pre-computed samples — every other
  energy path either reads its own file (`LoadEnergy()`) or tracks
  bytes as they're written internally (`writeWave()`'s Layer II
  branch). This is a genuinely new piece of public API surface on
  `RDWaveFile`, raised and confirmed explicitly (2026-06-20) rather than
  added silently. Mirrors `LoadEnergy()`'s existing PCM16
  peak-comparison convention exactly (one call per full 1152-frame
  block; a trailing partial block is simply never measured, matching
  every other format), just fed from outside instead of read from the
  file. `Stage3Layer3()` also now mirrors `Stage3Layer2Wav()`'s
  existing `cart`/`bext`/`rdxl` embedding pattern (confirmed
  2026-06-20) and, since it's WAV-wrapped now rather than a bare
  stream, no longer calls `ApplyId3Tag()` — confirmed against
  `Stage3Layer2Wav()`, which never called it either, since
  `TagLib::MPEG::File` expects a bare elementary stream and would not
  behave correctly against a RIFF container.
