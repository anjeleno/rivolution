# 0004 — MP3 gain-patch normalization for passthrough imports

**Date:** 2026-06-21

## Goal

Spec 0003 and this session's follow-up fix made `web/rdxport/import.cpp`'s
MP3-to-MP3 passthrough path correctly require
`normalization_level==0 && autotrim_level==0` — a Dropbox requesting
normalization or autotrim now falls through to the full
decode→process→re-encode `RDAudioConvert` path instead of silently
having its request ignored (the bug spec 0003 found and fixed). That fix
is correct, but it has a real cost: the user normalizes essentially every
single Dropbox-driven MP3 import (commonly to -13dBFS), so the
passthrough optimization is now effectively dead for the real workload —
every import pays the full decode/LAME-re-encode cost again.

Goal: get most of that speed back without reintroducing the correctness
bug, by applying normalization directly to the MP3 bitstream — patching
the `global_gain` field each frame already carries — instead of decoding
to PCM, scaling samples, and re-encoding.

## Background — verified against source and real package archives, not assumed

### The existing PCM normalization formula, which this must stay consistent with

`RDAudioConvert`'s normalization (`lib/rdaudioconvert.cpp:1005-1006`):

```cpp
gain = (float)normalizationLevel() - 20.0*log10f(conv_peak_sample);
ratio = exp10f(gain/20.0);
```

`normalizationLevel()` (`lib/rdsettings.cpp:161-167`) is a plain `int`,
hundredths of a dB — e.g. `-1300` means -13.00dBFS — set per-Dropbox via
the existing RDAdmin UI, never hardcoded anywhere in the pipeline. Peak
measurement is `fabsf`-based, tracked via `UpdatePeak()`
(`lib/rdaudioconvert.cpp:2017-2038`), peak-relative dBFS — **not** a
loudness/RMS measure. Any new normalization mechanism must target the
same peak-relative semantic, or a file normalized via the fast path and
one normalized via the slow path would land at audibly different levels
for the same configured target.

### `do_passthrough`'s current state (landed this session, spec 0003)

```cpp
// web/rdxport/import.cpp:246-248
bool do_passthrough=source_is_mp3&&(effective_format==3)&&
  (source_sample_rate==rda->system()->sampleRate())&&
  (normalization_level==0)&&(autotrim_level==0);
```

`rdimport` (what a Dropbox actually runs) confirmed to reach this same
code path: it calls `RDAudioImport::runImport()`
(`lib/rdaudioimport.cpp:156-164`), which POSTs `NORMALIZATION_LEVEL`/
`AUTOTRIM_LEVEL` via `libcurl` to this same `rdxport.cgi` `Import`
command — not a separate, divergent path.

### Why a from-scratch bitstream parser was considered and rejected

A full ISO/IEC 11172-3/13818-3 side-info bit-level parser/patcher was
drafted during planning (walking `main_data_begin`, `scfsi` (MPEG1 only),
and per-granule `part2_3_length`/`big_values`/`global_gain`/
`scalefac_compress`/window-switching fields, byte-aligned nowhere,
varying by MPEG version and channel mode). Rejected: real risk surfaced
immediately (an unresolved question about whether the existing
`__side_data_offset` table at `lib/rdwavefile.cpp:3771-3776` already
accounts for the optional 2-byte CRC after the frame header, or has
simply never been exercised against a CRC-protected file), on top of the
inherent complexity of variable-width per-granule fields and
MPEG1-vs-MPEG2/2.5 layout differences. This is exactly the kind of
"touching something that could break" risk this fork's own conventions
already push against (`~/CLAUDE.md`: simplicity first, no
speculative/risky code where a simpler path exists).

### `mp3gain` — the established tool for exactly this technique, confirmed available everywhere this project targets

`mp3gain` has implemented direct MP3 `global_gain` bitstream patching
(no decode/re-encode) for ~20 years — this is not a novel technique,
it's the canonical reference implementation or the algorithm itself
inside Rivendell's own ISO/IEC research. Confirmed against the real
package archives (not assumed) that it's packaged for every OS/arch
combination this project targets, matching `rivendell-installer`'s
existing ARM64/Debian support matrix:

| | amd64 | arm64 |
|---|---|---|
| Ubuntu 24.04 (noble) | `1.6.2-2build1` | `1.6.2-2build1` |
| Debian trixie | `1.6.2-2+b1` | `1.6.2-2+b2` |

Same small C codebase across all four (only runtime dependency:
`libmpg123`) — no architecture-specific surprise the way QtWebKit/id3lib
had package-naming differences during the `rivendell-installer` ARM64
work.

Shelling out to an external CLI tool is not a new pattern for this
codebase: `lib/rddisclookup.cpp:467-469` already invokes
`/usr/bin/cdda2wav` via `QProcess` for CD ripping, and
`web/rdxport/podcasts.cpp:922-931` does the same in this exact directory
for podcast processing.

Each `global_gain` unit step changes amplitude by exactly `2^(1/4)`, i.e.
`20*log10(2^0.25) ≈ 1.505 dB` (not a rounded 1.5, though that's the
commonly-cited approximation in `mp3gain`'s own docs) — gain only lands
on these discrete steps. This must be reported honestly (requested vs.
actually-achieved level), never silently hidden.

## Design — division of responsibility

Rivendell keeps deciding **how much** gain to apply, using the exact
same formula already in production for PCM (`lib/rdaudioconvert.cpp:1005-1006`).
`mp3gain` only does the mechanical part it has already proven correct:
patching that computed dB shift into the bitstream via `-d <shift>` (a
direct relative adjustment), **not** its own built-in `-r`/`-a`
loudness-target modes (which target a fixed ReplayGain-style 89dB
loudness reference — a different, incompatible normalization concept
from Rivendell's peak-based dBFS target). This keeps the gain-patch path
numerically consistent with what the PCM path would produce for the same
file, just applied via a different mechanism.

## Approach

### New class: `RDMpegGainPatch` (`lib/rdmpeggainpatch.h`, `lib/rdmpeggainpatch.cpp`)

Same shape as `RDAudioConvert` — construct, configure via setters, call
one verb method, read back a result — not a method on `RDWaveFile` or a
new `RDAudioConvert` stage, since this never touches
`RDAudioConvert`'s `SNDFILE`/libsndfile pipeline and is a fundamentally
different operation (a mechanical bitstream patch via subprocess, not a
decode/convert/encode stage).

API:
- `setSourceFile(QString)` / `setDestinationFile(QString)`
- `setNormalizationLevel(int)` — hundredths of dB, same contract as
  `RDSettings::normalizationLevel()`
- `patch()` → `ErrorCode` (`Ok`, `NotApplicable`, `ClippingRisk`,
  `ToolNotFound`, `ToolError`)
- `achievedLevel()` — actual level reached (hundredths of dB), valid
  after `Ok`, for honest reporting

`patch()` flow:
1. Measure the source file's actual peak sample — first check whether
   `mp3gain`'s own analysis output (`-s c`/`-o`) reports a usable
   peak/max-amplitude value directly (one subprocess call doing both
   analysis and giving us what we need, no new decode code in
   Rivendell); otherwise fall back to reusing the existing
   `LoadEnergyMpegLayer3()`-style decode pass (`lib/rdwavefile.cpp:4671`
   on) purely for peak measurement. Either way: no new bit-level MP3
   parsing code gets written, in either branch.
2. Compute `gain_dB = normalizationLevel()/100.0 - 20*log10f(peak_sample)`
   — the same formula as the PCM path, kept consistent (consider a
   small shared free function both call, decided at implementation
   time).
3. **Decrease (`gain_dB <= 0`):** always safe. Run
   `mp3gain -d <gain_dB> -c -o <scratch copy>` via `QProcess`, mirroring
   the `cdda2wav`/`podcasts.cpp` invocation pattern. Parse the achieved
   gain from stdout.
4. **Increase (`gain_dB > 0`):** real clipping risk — raising
   `global_gain` on already-quantized data can push the decoded signal
   past full scale in a way Rivendell can't see without decoding. Use
   `mp3gain`'s own `-k` (auto-lower to avoid clipping) rather than
   reimplementing that logic — it already solved this. Report the
   actually-applied (possibly capped) gain via `achievedLevel()`.
5. `mp3gain` modifies files in place by default — always operate on a
   scratch copy, never the original Dropbox-dropped source, consistent
   with how the rest of the import pipeline treats the source as
   read-only until safely copied.
6. Not found / unexpected exit / free-format or otherwise unhandleable
   file → `NotApplicable`/`ToolError`. Caller falls back to the full
   `RDAudioConvert` path; this class never retries or guesses.

### `web/rdxport/import.cpp` integration

```cpp
bool passthrough_eligible=source_is_mp3&&(effective_format==3)&&
  (source_sample_rate==rda->system()->sampleRate())&&
  (autotrim_level==0);
bool do_passthrough=passthrough_eligible&&(normalization_level==0);
bool do_gain_patch=passthrough_eligible&&(normalization_level!=0);
```

`autotrim_level!=0` always forces the full path regardless of
normalization — autotrim needs real sample-accurate start/end editing
(`RDCut::autoTrim()`, called at `import.cpp:341` after the full
conversion path), unrelated to a global gain shift and not expressible
as a bitstream patch.

`do_gain_patch`: construct `RDMpegGainPatch`, call `patch()`. On `Ok`,
proceed exactly like the existing `do_passthrough` branch (WAV-wrap,
reopen, force `hasEnergy()` — the LEVL-persistence pattern fixed earlier
this session) sourcing bytes from the gain-patched file instead of a raw
copy. On any other result, fall through to the existing `RDAudioConvert`
`else` branch unchanged — failure here just means this file didn't get
the fast path, not an error condition.

### Honest achieved-level reporting

When `achievedLevel() != normalizationLevel()` (clipping cap, or the
inherent ~1.505dB step quantization), `rda->syslog()` requested vs.
achieved so an operator watching logs can see it — never silent. Check
at implementation time whether `RDWaveFile::getNormalizeLevel()`/
`setNormalizeLevel()` (`lib/rdwavefile.h:250-251`) is currently
live/displayed anywhere in RDLibrary; if so, persist the achieved value
per-cut there too rather than only logging it.

## Files

- New: `lib/rdmpeggainpatch.h`, `lib/rdmpeggainpatch.cpp`
- Modified: `web/rdxport/import.cpp` (passthrough-eligibility split above)
- Modified: `lib/Makefile.am` (register the new source files)
- Separate repo, separate follow-up (not this branch's scope):
  `rivendell-installer`'s `roles/base/tasks/main.yml` package list needs
  `mp3gain` added once this lands, so a fresh golden-image build actually
  has the dependency this shells out to.

## Verification plan

1. Confirm `mp3gain`'s exact CLI syntax/output format for real
   (`mp3gain --help`, `man mp3gain`, a real invocation) before writing
   integration code, rather than relying on remembered flag names —
   specifically `-d`'s exact semantics, whether `-o`/`-s c` reliably
   reports a usable peak value, and `-k`'s exact reported behavior when
   it caps an increase.
2. Build-verify (`make`/`sudo make install` — the user runs this).
3. Real Dropbox test at -13dBFS: a file louder than -13dBFS (common
   case — should gain-patch, fast, correct level) and a file quieter
   than -13dBFS (gain-increase/clipping-risk case — should gain-patch to
   a capped level with an honest log line, or cleanly fall back).
4. Confirm Edit Markers still works correctly on a gain-patched file
   (this session's LEVL persistence work must still apply cleanly).
5. Confirm the fallback path works correctly when `mp3gain` is not
   installed — must cleanly fall through to `RDAudioConvert`, not error.

## Implementation deviations

*(to be filled in as found, matching spec 0003's convention of
documenting corrections discovered during implementation rather than
silently absorbing them)*
