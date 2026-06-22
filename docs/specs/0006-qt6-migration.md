# 0006 — Qt6 migration

**Date:** 2026-06-21

## Goal

Migrate the native C++/Qt desktop applications (`RDAirplay`,
`RDLibrary`, `RDAdmin`, `RDLogEdit`, `RDPanel`, and the rest of the
Qt-based tooling) from Qt5 to Qt6. Qt5 is deprecated upstream and is
not packaged on Ubuntu 26.04, which this project is already targeting.

This migration has no technical dependency in either direction on the
Go API work (`0005-go-api-foundation.md`) — the Go layer has no Qt
dependency, and this migration doesn't touch it. The two can proceed in
parallel; this is a scheduling/priority decision, not a technical
sequencing one.

## Background

Current Qt5 dependency, `configure.ac:96`:

```
PKG_CHECK_MODULES(QT5,Qt5Core Qt5Widgets Qt5Gui Qt5Network Qt5Sql Qt5Xml Qt5WebKitWidgets,,[AC_MSG_ERROR([*** Qt5 not found ***])])
```

`Qt5Core`/`Qt5Widgets`/`Qt5Gui`/`Qt5Network`/`Qt5Sql`/`Qt5Xml` are core
modules with stable APIs across the Qt5→Qt6 boundary. `Qt5WebKitWidgets`
is the one module requiring a real component swap (see below).

### Confirmed migration items

- **`QWebView` → `QWebEngineView`.** Used in
  `rdairplay/messagewidget.{cpp,h}`; an unused include also exists in
  `rdairplay/topstrip.h`. `QWebEngineView`'s signal/slot API
  (`loadFinished`, `setUrl`) is unchanged from `QWebView`'s, and is
  itself stable across Qt5↔Qt6 — the swap from `Qt5WebKitWidgets` to
  `Qt5WebEngineWidgets` is a real, separate prerequisite step (already
  scoped on `feature/qtwebengine-migration`) independent of the broader
  Qt6 jump; once done, the same `QWebEngineView` code works unchanged
  under `Qt6WebEngineWidgets`.
- **`QRegExp` → `QRegularExpression`.** `QRegExp` is removed in Qt6.
  Four files: `rdadmin/add_schedcodes.cpp`, `lib/rdconfig.cpp`,
  `lib/rddisclookup.cpp`, `importers/nexgen_filter.cpp`.
- **`QString::KeepEmptyParts` → `Qt::KeepEmptyParts`.** The enum moved
  from `QString`'s scope to `Qt`'s scope in Qt6. Approximately 21
  files; one instance already corrected as precedent (commit
  `4878686f`).
- **`configure.ac`'s Qt module detection.** Rename `Qt5Core` →
  `Qt6Core` (and the same pattern for `Widgets`/`Gui`/`Network`/`Sql`/
  `Xml`), and the moc/lupdate/lrelease tool-name checks (`configure.ac`
  ~lines 98-105) from their `-qt5` variants to `-qt6`. `Makefile.am`
  files reference only the generic `@QT5_CFLAGS@`/`@QT5_LIBS@`
  substitution variable names, not Qt5-specific content, so this is a
  contained, single-file change plus a variable-name rename if desired
  for clarity (not functionally required).

### Explicitly not a migration item

`SIGNAL()`/`SLOT()` macro-based signal/slot connections (used
extensively — roughly 2,510 occurrences across 318 files) compile
unchanged under Qt6. No action is needed for this pattern; it is not
part of this spec's scope.

### Build-time verification mechanism

Add `DEFINES += QT_DISABLE_DEPRECATED_BEFORE=QT_VERSION_CHECK(6,0,0)`
to the build during this migration. This standard Qt macro makes the
compiler error on any usage of an API deprecated before Qt 6.0 — a
build-time forcing-function that catches every instance of the
patterns above, plus anything a manual grep sweep misses, automatically
and exhaustively. This is the actual verification mechanism for
completeness of this migration, not a supplementary nice-to-have.

### Explicitly out of scope

- **A global code-formatting pass** (e.g. `clang-format` applied
  uniformly across the codebase). The existing code style (2-space
  indentation, the project's established `NULL`-not-`nullptr`
  convention — confirmed via direct inspection: zero `nullptr`, dozens
  of `NULL` usages, across multiple core files) is already internally
  consistent. A global reformat would touch nearly every line of the
  codebase for no functional benefit and carries real risk of
  introducing mechanical-transform errors at that scale. If a file is
  already being touched for a real Qt6-migration reason, matching its
  surrounding style is expected as normal practice — a dedicated global
  pass is not.
- **A broader "string formatting and memory allocation" sweep.**
  `QString::asprintf` requires no change for Qt6 — it is unchanged and
  available as-is. No further memory-allocation-pattern changes are
  required by the Qt5→Qt6 transition itself; any such initiative would
  need its own independent justification and is not part of this spec.

## Verification

1. `QT_DISABLE_DEPRECATED_BEFORE=0x060000` (see Implementation
   deviations below for why this is the literal hex value rather than
   `QT_VERSION_CHECK(6,0,0)`) is added before any other migration work,
   so the build itself reports the true, complete list of remaining
   deprecated-API usages rather than relying on the grep-based list
   above being exhaustive.
2. Full `./configure && make` succeeds against Qt6 development
   packages on the target OS (Ubuntu 26.04).
3. Each affected application launches and exercises the specific
   migrated code path (e.g. `RDAirplay`'s messaging panel for the
   `QWebEngineView` swap) to confirm behavioral parity, not just a
   clean compile.

## Implementation deviations

- **C++17 is a real, necessary migration item this spec's original
  grep-based investigation missed entirely.** Qt6 itself requires a
  C++17 compiler as a hard minimum (confirmed directly: a real build
  attempt failed immediately on Qt6's own headers with `#error "Qt
  requires a C++17 compiler"`, not just from reading Qt's
  documentation). Every `Makefile.am` in this tree hardcoded
  `-std=c++11` (49 files, all uniformly — no file had already moved to
  a newer standard), predating any Qt6 awareness. This is exactly the
  kind of gap `QT_DISABLE_DEPRECATED_BEFORE` itself can't catch (it's a
  compiler-standard mismatch, not a deprecated Qt API), and exactly why
  a real `./configure && make` attempt matters as the actual
  verification mechanism, not just a grep sweep against the codebase.
  Fixed by bumping `-std=c++11` to `-std=c++17` (the literal minimum
  Qt6 requires, not a newer standard) across all 49 files uniformly.
- **`QT_DISABLE_DEPRECATED_BEFORE`'s value uses the literal hex
  constant (`0x060000`), not `QT_VERSION_CHECK(6,0,0)` as originally
  written above.** This flag's value passes through several layers of
  shell before reaching the compiler (the `Makefile` recipe, libtool's
  wrapper script, the final `/bin/bash -c` invocation), and the macro
  call's unescaped parentheses/commas get parsed as shell syntax
  partway through — confirmed directly: a real build failed with
  `syntax error near unexpected token '('`. `QT_VERSION_CHECK`'s own
  definition is `((major<<16)|(minor<<8)|patch)`, so `(6,0,0)` is
  simply `0x060000` — using that directly avoids the multi-layer
  escaping problem entirely rather than trying to backslash-escape
  parens through three separate shells.
- **`QWebView` → `QWebEngineView`, already done elsewhere.** Rather
  than redo this from scratch, pulled in the already-implemented,
  already-spec'd fix from `anjeleno/rivendell`'s
  `feature/qtwebengine-migration` branch (now `0009-qtwebengine-migration.md`
  in this repo) via `git cherry-pick`, since that work already found a
  real behavioral gap (`QWebEnginePage` has no `mainFrame()`) that a
  naive mechanical rename would have missed.
- **`moc`/`lupdate`/`lrelease` detection needed real logic, not a
  suffix swap.** The original plan assumed Qt6 follows Qt5's `-qt5`-
  suffixed binary-name convention (e.g. `moc-qt6`). Confirmed directly
  against the real installed Ubuntu packages: Qt6 drops that
  convention entirely, placing `moc`/`uic`/`rcc` unsuffixed in
  `/usr/lib/qt6/libexec/` and `lupdate`/`lrelease` unsuffixed in
  `/usr/lib/qt6/bin/`, neither on `PATH`. A plain `PATH`-based
  `moc`/`lupdate`/`lrelease` lookup is actively unsafe here, not just
  incomplete: this same packaging also installs `qtchooser` symlinks at
  those bare names in `/usr/bin/`, which resolved to Qt 5.15.13
  silently when tried — caught only by checking the resolved binary's
  own `-v` output, not by `./configure` exiting cleanly. `configure.ac`
  now checks the known Qt6 path explicitly first, falling back to a
  `PATH` search only if that specific file doesn't exist.
