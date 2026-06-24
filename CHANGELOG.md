# Changelog

Notable changes to the Rivendell v6 fork. Newest entries first.

Pre-fork history (through 2026-06-15) is preserved unchanged in
`ChangeLog.upstream-v4`, which is no longer appended to.

## 2026-06-23

- Qt6 migration complete: `./configure && make` now succeeds
  end-to-end against Qt6 on Ubuntu 26.04. Beyond the patterns already
  logged below (2026-06-22), full-build verification surfaced 18 more
  distinct Qt6 API removals/changes the original grep sweep missed —
  `QString::sprintf()`→`asprintf()` (57 occurrences, 20 files);
  `QPalette::Background`/`Foreground`→`Window`/`WindowText` (169
  occurrences, 39 files); `QFontMetrics::width(QString)`→
  `horizontalAdvance()` (77 occurrences, 23 files);
  `Qt::TextColorRole`/`BackgroundColorRole`; `Qt::MidButton`;
  `QDate::shortDayName()` and siblings; `QDesktopWidget` removed
  entirely (→ `QScreen`); `QMouseEvent`/`QDropEvent` position
  accessors; `QWheelEvent::orientation()`/`delta()`; `QVariant::Type`
  on `QMimeData`'s virtual interface; `QTextStream::setCodec()`
  removed (Core5Compat split); a second `QList::swap(int,int)` fix
  idiom (`swapItemsAt`) alongside the first; a `QMap`/`QMultiMap`
  iterator-type split; a missing `QFile` include; `QWidget::enterEvent`'s
  widened `QEnterEvent*` signature; `QDateTime(const QDate&)`; and
  `QLabel::pixmap()`'s value-vs-pointer return change. Full detail in
  `docs/specs/0006-qt6-migration.md`.
- Version bumped to `6.0.0int0` (from `4.4.1int3`) in
  `versions/PACKAGE_VERSION`. `versions/README.txt` gained a short
  explanation of the existing `intN` pre-release suffix convention
  (inherited from upstream, not new) and a note that
  `debian/changelog` is regenerated from `debian/changelog.src` by
  `autogen.sh`, not hand-edited.
- `debian/changelog.src`'s maintainer line updated to this fork's own
  identity, separate from upstream's.
- Ubuntu 26.04 build-compatibility fixes unrelated to Qt6 itself:
  MusicBrainz pkg-config module names renamed (`libmusicbrainz5`→
  `libmusicbrainz5cc`, `libcoverart`→`libcoverartcc`), with the
  matching `librd_la_LIBADD` linkage added in `lib/Makefile.am`;
  ImageMagick detection tries the unversioned `Magick++` pkg-config
  name first, falling back to the old `Magick++-6.Q16` name (Ubuntu
  26.04 ships ImageMagick 7 only, under which the old name doesn't
  exist); `LT_OUTPUT` added to `configure.ac` since libtool ≥2.4.7 no
  longer generates `./libtool` as a side effect of
  `AC_PROG_LIBTOOL`/`LT_INIT`, which the existing rpath workaround
  needs present at configure time.

## 2026-06-22

- Qt6 migration (in progress, `feature/qt6-migration`, not yet merged):
  `configure.ac` now requires Qt6 (`Qt6Core`/`Qt6Widgets`/`Qt6Gui`/
  `Qt6Network`/`Qt6Sql`/`Qt6Xml`/`Qt6WebEngineWidgets`) instead of Qt5,
  with `QT_DISABLE_DEPRECATED_BEFORE=0x060000` added as a build-time
  completeness check. `moc`/`uic`/`rcc`/`lupdate`/`lrelease` detection
  rewritten for Qt6's real packaging (no `-qt5`-style suffix
  convention, unsuffixed binaries outside `PATH`, and a `qtchooser`
  trap that silently resolves to an old Qt5 install if not handled
  explicitly). `QRegExp` replaced with `QRegularExpression` in the four
  files using it; `QString::KeepEmptyParts`/`SkipEmptyParts` replaced
  with `Qt::KeepEmptyParts`/`SkipEmptyParts` everywhere (94 occurrences,
  49 files); every `Makefile.am`'s `-std=c++11` bumped to `-std=c++17`
  (Qt6's own hard minimum). `QWebView` replaced with `QWebEngineView`
  in `RDAirPlay`'s message-display widget, including a real behavioral
  fix (`QWebEnginePage` has no `mainFrame()` — scrollbar hiding moves to
  a `QWebEngineSettings::ShowScrollBars` page setting instead). See
  `docs/specs/0006-qt6-migration.md` and
  `docs/specs/0009-qtwebengine-migration.md`.
- Fixed: MP3 gain-patch normalization (added 2026-06-21) silently never
  applied any gain shift. The requested level was read as hundredths of
  a dB, but every consumer of this setting elsewhere in the pipeline
  (`RDAudioConvert`'s own normalization, and `rdimport`'s own conversion
  before sending it over the wire) has always used plain whole dB —
  e.g. a Dropbox configured for -13dBFS was read as -0.13dB, rounding
  the computed gain-patch step to zero. `mp3gain` still ran and rewrote
  some header bytes, so the import completed normally with no error,
  just a still-unnormalized file. See
  `docs/specs/0004-mp3-gain-patch.md`.
- Added the configured Target Audio Format (PCM16/PCM24/MPEG Layer 2/
  MPEG Layer 3) to the Dropbox-flags dump at the top of `rdimport.log`,
  alongside the other already-logged per-Dropbox settings.

## 2026-06-21

- Added MP3 gain-patch normalization: a same-format MP3-to-MP3 import
  that requests normalization (the common case for most Dropboxes) can
  now still take a fast path — the requested gain is patched directly
  into each frame's `global_gain` field via `mp3gain`, instead of always
  falling through to a full decode/re-encode. Falls back to the existing
  conversion path whenever the patch isn't cleanly applicable. New
  runtime dependency: `mp3gain` (packaged for Ubuntu and Debian, amd64
  and arm64). See `docs/specs/0004-mp3-gain-patch.md`.
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
