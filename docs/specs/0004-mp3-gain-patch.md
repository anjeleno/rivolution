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

`patch()` flow (verified against the real installed `mp3gain` 1.6.2,
not assumed — see Implementation deviations below for what changed
from the original plan during this verification):
1. Measure the source file's actual peak: `mp3gain -s r -o -x <file>`
   (force fresh analysis, tab-delimited, skip the loudness-suggestion
   calc) reports a `Max Amplitude` column on a 16-bit-equivalent scale
   where `32768` = full scale/0dBFS. Convert:
   `peak_sample_equiv = max_amplitude / 32768.0`. One subprocess call,
   no new decode code in Rivendell.
2. Compute
   `gain_dB = normalizationLevel()/100.0 - 20*log10f(peak_sample_equiv)`
   — the same formula as the PCM path, kept consistent (consider a
   small shared free function both call, decided at implementation
   time). Convert to whole `global_gain` steps:
   `step_count = round(gain_dB / (20*log10(pow(2.0,0.25))))` — compute
   the divisor as a real `double`, don't hardcode `1.505`.
3. **Decrease (`step_count <= 0`):** always safe. Run
   `mp3gain -g <step_count> -c -o <scratch copy>` via `QProcess`,
   mirroring the `cdda2wav`/`podcasts.cpp` invocation pattern (`-g`
   applies the exact step count with zero analysis — **not** `-d`, see
   deviation below). Re-derive `achievedLevel()` from `step_count`
   directly (exact, since `-g` is unconditional/literal — no need to
   re-run analysis afterward to confirm).
4. **Increase (`step_count > 0`):** real clipping risk — raising
   `global_gain` on already-quantized data can push the decoded signal
   past full scale in a way Rivendell can't see without decoding.
   **`mp3gain`'s `-k` does not help here** (see deviation below — it
   only constrains `-r`/`-a`'s own automatic suggestions, not a manual
   `-g` value). Cap `step_count` ourselves before calling `mp3gain` at
   all: `max_safe_steps = floor(4*log2(32768.0/max_amplitude))` (derived
   from the same `2^(steps/4)` relationship used in step 2, solved for
   the step count that keeps the resulting peak at or under full
   scale), then `step_count = min(step_count, max_safe_steps)`. Apply
   the capped value via `-g`, and report the *actually applied* level
   via `achievedLevel()` (derived from the capped `step_count`, not the
   originally requested one) whenever capping occurred.
5. `mp3gain` modifies files in place by default — always operate on a
   scratch copy, never the original Dropbox-dropped source, consistent
   with how the rest of the import pipeline treats the source as
   read-only until safely copied. Always pass `-c`: without it,
   `mp3gain` blocks waiting on an interactive stdin confirmation
   whenever it detects clipping risk — fatal in a `QProcess` context
   with no controlling tty.
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

1. ~~Confirm `mp3gain`'s exact CLI syntax/output format~~ — done, see
   Implementation deviations below: `-g` (not `-d`) is the correct
   direct-application flag, `-s r -o -x` is the correct peak-measurement
   invocation, and `-k` does not apply to manual `-g` values (clipping
   safety is computed in `RDMpegGainPatch` itself).
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

- **Verified against the real installed `mp3gain` 1.6.2 (2026-06-21),
  three corrections to the plan above:**

  1. **`-d` is not a standalone direct-dB-shift flag.** Per its actual
     `--help` text, `-d <n>` "modify suggested dB gain by floating-point
     n" — it only adjusts whatever `-r`/`-a`'s automatic loudness
     analysis already suggested; it doesn't apply on its own. The
     correct flag for "apply a Rivendell-computed step count with zero
     analysis" is `-g <i>` ("apply gain i without doing any analysis").
     Confirmed empirically: `mp3gain -g -9 -c -o <file>` shifted every
     frame's `global_gain` by exactly -9 (`Max global_gain` 188→179,
     `Min global_gain` 129→120, both shifted identically), and the
     measured peak dropped by ~13.54dB — matching the
     `9 × 1.50515dB ≈ 13.55dB` math from earlier discussion almost
     exactly. `-r`/`-a` target a fixed ReplayGain-style 89dB loudness
     reference, confirmed via a real recalculation
     (`mp3gain -s r -o <file>` suggested `-5` steps/`-7.21dB` on a test
     file with a peak already above full scale) — entirely the wrong
     semantic for Rivendell's peak-based dBFS target, so `-r`/`-a` are
     not used at all; `-g` with a step count computed from Rivendell's
     own existing peak formula is the only mechanism actually used.

  2. **`-k` does not constrain a manual `-g` value.** Tested directly:
     `mp3gain -g 20 -k -c -o <file>` and `mp3gain -g 20 -c -o <file>`
     (no `-k`) produced byte-for-byte identical results — `Max
     global_gain` went 188→208 in both, and the measured peak ballooned
     to ~1,081,968 (vastly over the 32768 full-scale reference) in both.
     `-k`'s "automatically lower Track/Album gain to not clip" applies
     only to `-r`/`-a`'s own automatic suggestions, not to a
     caller-specified `-g`. Since `-g` is the only mechanism this
     feature uses (per deviation 1), **clipping safety must be computed
     entirely in `RDMpegGainPatch`'s own code** — capping `step_count`
     using the peak measured in step 1 before ever invoking `-g` — not
     delegated to `mp3gain` as the original plan assumed.

  3. **Peak measurement can't be skipped for the decrease case.** The
     original plan suggested skipping the peak-measurement pass
     entirely for files needing a decrease, as an optimization. That's
     backwards: the measurement is what determines *whether* a file
     needs an increase or a decrease in the first place (Rivendell's
     own `gain = target - 20*log10(peak)` formula needs the peak to
     compute anything at all) — it's needed unconditionally, every
     file, every time. The actual optimization opportunity (not yet
     decided) is whether a *second*, separate decode/analysis pass can
     be avoided after that, not the first one.

  Confirmed `mp3gain -s r -o -x <file>` (force fresh analysis,
  tab-delimited, skip the loudness-suggestion calc) reports `Max
  Amplitude` on a 16-bit-equivalent scale where `32768.0` = full
  scale/0dBFS — this is the value `RDMpegGainPatch` parses and divides
  by `32768.0` before plugging into Rivendell's existing peak-dBFS
  formula.

- **`import.cpp` integration details, decided during implementation:**
  - `do_gain_patch`'s attempt runs before the `if(do_passthrough)` block
    and, on success, sets `do_passthrough=true` to reuse that block's
    WAV-wrap-and-finish logic unchanged (`passthrough_source_file`
    points at the gain-patched scratch copy instead of the original
    upload in that case) — avoids duplicating ~40 lines of
    `createWave()`/copy-loop/`hasEnergy()` logic for what is, from that
    point on, identical handling regardless of which path produced the
    bytes.
  - The scratch file (`<upload>.gainpatch`) is deleted right after the
    copy loop consumes it, in the same place `src_wave` itself is
    deleted — necessary because the per-request upload temp directory
    gets `rmdir()`'d at the very end of `Import()`, which fails if
    anything besides the original upload is left in it.
  - Achieved-level logging only fires when the deviation from the
    requested level exceeds 100 (hundredths of a dB, i.e. >1.00dB) —
    deliberately above the ~0.75dB (half a `global_gain` step) maximum
    possible from ordinary discrete-step rounding alone, so the log line
    only fires on a genuine clipping-safety cap, not on the routine
    quantization every single gain-patched import has. A
    successfully-applied, non-capped gain-patch logs nothing at all —
    consistent with this fork's general preference for quiet success
    paths.
