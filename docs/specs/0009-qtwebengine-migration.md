# 0004 — Replace QtWebKit with QtWebEngine

**Date:** 2026-06-20

## Goal

`Qt5WebKitWidgets`/`QWebView` is the one remaining hard blocker for
building this fork on Debian 13 (trixie) — see
`rivendell-installer`'s `docs/specs/0002-arm64-debian-support.md`.
WebKit has been dead upstream for years; Debian dropped the package
(binary and source) starting trixie, while Ubuntu still carries it for
now but won't indefinitely. Replace it with `Qt5WebEngineWidgets`/
`QWebEngineView` — Qt's own official, still-maintained successor —
rather than working around its absence.

Explicitly scoped as a narrow, isolated swap: everything else in this
codebase stays on Qt5 exactly as it is today. Not a Qt6 migration. But
deliberately chosen because `QWebEngineView`'s API is essentially
unchanged between Qt5WebEngine and Qt6WebEngine (Qt preserved it
across the major-version bump — differences are in include
paths/build system, not the API surface used here) — so this also
happens to be the first real, forward-compatible piece of any future
Qt6 migration, without requiring one now.

## Background — verified against source, not assumed

### Confirmed scope: three files, one feature

`grep`ped the whole tree for `QtWebKit`/`QWebView`/`webkitwidgets` —
exactly three files reference it at all:

- `rdairplay/messagewidget.cpp` — `d_view=new QWebView(this);`
  (line 52), connected to `loadFinished(bool)`, and `setUrl(url)`
  called elsewhere in the same file. This is the actual feature: an
  embedded web view in RDAirPlay's message-display widget, loading a
  real URL (not just rendering a fixed HTML string) — confirmed via
  `setUrl()`'s presence, ruling out a lighter rich-text-only widget
  (`QTextBrowser`) as a substitute, since that doesn't render
  JS/CSS-driven pages.
- `rdairplay/messagewidget.h` — `#include <QWebView>` and the
  `QWebView *d_view;` member declaration.
- `rdairplay/topstrip.h` — `#include <QWebView>`, but no actual
  usage in that file; it only also includes `messagewidget.h`.
  Likely a stale/redundant include carried over at some point — worth
  just removing rather than swapping, see below.

### Confirmed scope: one build-system touchpoint, not several

`configure.ac:96` is the *only* place the module name appears in the
build system:

```
PKG_CHECK_MODULES(QT5,Qt5Core Qt5Widgets Qt5Gui Qt5Network Qt5Sql Qt5Xml Qt5WebKitWidgets,,[AC_MSG_ERROR([*** Qt5 not found ***])])
```

Checked `rdairplay/Makefile.am` directly rather than assume — it only
references the generic `@QT5_CFLAGS@`/`@QT5_LIBS@` substitution
variables this one `PKG_CHECK_MODULES` call already produces, no
separate WebKit-specific flags anywhere. So swapping the module name
in this one line is sufficient; no other `Makefile.am` changes needed.

### Confirmed package availability on every target this fork builds on

`qtwebengine5-dev` (providing `Qt5WebEngineWidgets`) is present on
Ubuntu 24.04 (both amd64 and arm64) and Debian 13 (trixie) — checked
directly against each archive's real package index, not assumed from
Qt's own documentation.

## Implementation plan

### 1. `configure.ac:96`

```
PKG_CHECK_MODULES(QT5,Qt5Core Qt5Widgets Qt5Gui Qt5Network Qt5Sql Qt5Xml Qt5WebEngineWidgets,,[AC_MSG_ERROR([*** Qt5 not found ***])])
```

### 2. `rdairplay/messagewidget.h`

- `#include <QWebView>` → `#include <QWebEngineView>`
- `QWebView *d_view;` → `QWebEngineView *d_view;`

### 3. `rdairplay/messagewidget.cpp`

- `d_view=new QWebView(this);` → `d_view=new QWebEngineView(this);`
- The `loadFinished(bool)` connection and `setUrl()` call should work
  unchanged — both are part of `QWebEngineView`'s API, named and
  behaving the same way, by Qt's own design intent for this migration
  path generally.

### 4. `rdairplay/topstrip.h`

Remove the unused `#include <QWebView>` outright rather than swap it
to `<QWebEngineView>` — confirmed nothing in this file actually
references either type.

## Real risks to verify during implementation, not assumed away

- **Sandboxing.** `QWebEngineView` is Chromium-based, and Chromium's
  sandbox refuses to run as root without an explicit opt-out flag.
  Checked `rivendell-installer`'s `fix-rivendell-user.sh.j2` (the
  Ansible side) directly: `rdairplay` runs as a normal user with
  `audio`-group membership and real-time scheduling limits, not as
  root — so this is unlikely to be a real problem here, but should be
  confirmed by actually running RDAirPlay under the new code rather
  than assumed safe from this alone, since other Rivendell tools
  (`rdselect_helper`) *are* SETUID-root, a meaningfully different
  execution context this spec doesn't directly touch.
- **Heavier runtime footprint.** `QtWebEngine` bundles a Chromium
  instance; `QtWebKit` was a lighter engine. RDAirPlay runs on
  potentially resource-constrained on-air broadcast hardware — worth
  checking actual memory/startup-time impact empirically once built,
  not assuming it's negligible just because the API swap is clean.
- **`QtWebEngineWidgets` initialization quirks across Qt 5.15 minor
  versions.** Some Qt5/QtWebEngine combinations require an explicit
  `QtWebEngineQuick::initialize()`-style call before `QApplication` is
  constructed for proper sandbox setup; whether this fork's exact Qt
  5.15.x build needs it isn't confirmed yet — check during
  implementation, not assumed unnecessary.

## Implementation deviation: `webLoadFinishedData()` needed a real fix, not a rename

The Background section above missed something: `messagewidget.cpp` also
`#include <QWebFrame>` and `webLoadFinishedData()` called
`d_view->page()->mainFrame()->setScrollBarPolicy(...)` to hide
scrollbars after each load. `QWebEnginePage` has no `mainFrame()` at
all — QtWebEngine's process-separated Chromium architecture doesn't
expose synchronous per-frame DOM/scrollbar access the way WebKit did.
This is a real behavioral gap the "should work unchanged" framing above
didn't anticipate.

Verified the fix against the actual installed header rather than
recalled from memory: downloaded and extracted `qtwebengine5-dev`
(5.15.16+dfsg-3, Ubuntu noble) without installing it, and confirmed
directly in `qwebenginesettings.h` that `QWebEngineSettings::WebAttribute`
has a `ShowScrollBars` member, settable via
`QWebEnginePage::settings()->setAttribute(...)` (`settings()` confirmed
present on `QWebEnginePage` in `qwebenginepage.h` too). This is a
declarative, persistent page setting rather than a per-frame call —
functionally equivalent to the old behavior (scrollbars hidden on every
load), just expressed differently:

```cpp
void MessageWidget::webLoadFinishedData(bool state)
{
  d_view->page()->settings()->
    setAttribute(QWebEngineSettings::ShowScrollBars,false);
}
```

`#include <QWebFrame>` → `#include <QWebEngineSettings>`. The slot's
signature, its `loadFinished(bool)` connection, and everything else in
the constructor/`setUrl()`/`clear()`/`resizeEvent()` were unaffected —
confirmed via a final repo-wide grep for
`QWebView|QWebFrame|Qt5WebKit|WebKitWidgets` across every `.cpp`/`.h`/
`.ac`/`.am` file, returning no matches once this fix and the rest of
the plan above were applied.

## Confirmed out of scope

- Any other Qt5 module in this codebase — `Qt5Core`/`Qt5Widgets`/
  `Qt5Gui`/`Qt5Network`/`Qt5Sql`/`Qt5Xml` all stay exactly as they are.
  Not a Qt6 migration; only the one already-dead module gets replaced
  with its own direct, still-maintained Qt5 successor.
- Any change to how `rivendell-installer`'s Ansible playbook installs
  build dependencies — once this lands, `roles/base`'s existing
  Ubuntu-specific `libqt5webkit5-dev` entry becomes
  `qtwebengine5-dev`/`libqt5webenginewidgets5-dev` on *every* target
  OS (no longer Ubuntu-only), removing the one remaining blocker noted
  in that repo's spec 0002 — but that playbook-side change belongs in
  that repo once this spec actually lands, not bundled into this one.
