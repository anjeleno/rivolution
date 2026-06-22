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

1. `DEFINES += QT_DISABLE_DEPRECATED_BEFORE=QT_VERSION_CHECK(6,0,0)` is
   added before any other migration work, so the build itself reports
   the true, complete list of remaining deprecated-API usages rather
   than relying on the grep-based list above being exhaustive.
2. Full `./configure && make` succeeds against Qt6 development
   packages on the target OS (Ubuntu 26.04).
3. Each affected application launches and exercises the specific
   migrated code path (e.g. `RDAirplay`'s messaging panel for the
   `QWebEngineView` swap) to confirm behavioral parity, not just a
   clean compile.

## Implementation deviations

None yet — implementation has not started.
