# Changelog

Notable changes to Rivolution, a parallel Qt6 fork of Rivendell
developed independently of Fred Gleason's original project. Newest
entries first.

Pre-fork history (through 2026-06-15) is preserved unchanged in
`ChangeLog.upstream-v4`, which is no longer appended to.

## 2026-07-09

- `/tasks`: first real-box run of the log generation task surfaced the
  bug its "not yet verified on a real box" caveat was written for --
  `rivapi/store/tasks_deploy.go`'s log_gen helper script called
  `rdlogmanager -t` (merge the Traffic log into an existing log), never
  `-g` (generate one), so every run failed with "log does not exist"
  since there was never a log to merge into. Fixed to call `-g`, and
  split traffic-log merging out into its own new task type, `log_merge`
  (`log-merge.sh`, same `-t` invocation as before) -- generating a log
  and merging the Traffic log into it are two separate `rdlogmanager`
  modes with different preconditions, and were already two separate
  steps in the hand-maintained cron scripts this feature replaces, so
  they stay separate here too rather than one task trying to do both.
- `/tasks`: database backup tasks can now set an optional file name
  prefix (`FilePrefix`/`file_prefix`), used instead of the database's
  own name (read from `/etc/rd.conf`, currently "Rivendell" -- see
  `BACKLOG.md`) when naming the backup file. Defaults to the old
  behavior if left blank.
- `/system`: dropped the now-stale fixed `liquidsoap.service` row (see
  the ffmpeg replacement above) and its JACK-log-scraping warning
  check. Replaced with one status row per currently-deployed broadcast
  stream, read live from the streams manifest rather than a fixed
  list, since the number of stream services depends on how many mounts
  are configured on `/broadcast` -- start/stop/restart controls work
  on these the same as any other managed unit. Also fixes a real bug
  this surfaced: `DeployFfmpegStreams` was enabling each stream via
  `task-systemctl.sh`'s `enable` action, which only ever targets a
  task's `.timer` -- streams have no timer, only a `.service`, so every
  stream deploy would have failed outright. Added a new `enable-service`
  action for always-on services with no paired timer.
- `/export`: import/restore copy still described the old
  Icecast/Liquidsoap restart behavior; updated to describe the current
  Icecast-restart-plus-stream-redeploy behavior.

## 2026-07-07

- Fixed "No fade on segue out" (`segueGain()==0`) being silently
  ignored by the segue-end stop mechanism. `RDPlayDeck::pointTimerData()`
  was unconditionally calling `stopPlay()` at every cart's segue-end
  marker regardless of that flag, and `RDLogPlay::StartEvent()`'s Segue
  branch was unconditionally marking the outgoing element `Finishing`
  and scheduling a timed stop against it -- both only ever gated the
  *fade curve*, never whether a stop happened at all. In practice this
  meant any cart with "no fade" checked was still hard-stopped (or
  abruptly cut) right at its segue-end point, clipping any trailing
  audio (reverb tails, echoes) meant to be preserved past that marker.
  Combined with segue back-timing (`docs/specs/0002-segue-backtiming.md`),
  this produced inconsistent, seemingly random walk-in behavior:
  back-timing's delay math was correct, but the outgoing element kept
  getting truncated at segue-end regardless, before its tail could
  actually finish. Both call sites now skip the stop when
  `segueGain()==0`; the outgoing element instead runs out via its
  ordinary natural-completion path (`RDLogPlay::Finished()`), the same
  one already used for any cart with no segue at all.
- Fixed a regression introduced by the fix above: letting a "no fade"
  element run out via `RDLogPlay::Finished()` exposed a second,
  previously-unreachable bug in that same natural-completion path.
  `Finished()` called `FinishEvent()` unconditionally, which
  auto-advances to whatever the next not-yet-started line is --
  correct when nothing else was ever chained off this line's own segue
  markers, but wrong once an element could legitimately reach real
  natural completion *after* its successor had already been started by
  an earlier segue. `GetNextPlayable()` skips already-running lines
  when searching, so it would find the line *after* the real successor
  and hard-start it via an unconditional `Play`-transition stop-
  everything, killing the legitimately-playing successor a few seconds
  in. `Finished()` now only calls `FinishEvent()` if the very next line
  isn't already `Playing`/`Finishing`.
- Fixed RDLibrary's "Talk" column showing nothing unless both intro
  markers had been explicitly set. `RDLibraryModel` was computing it as
  `talkEndPoint() - talkStartPoint()` (a length), which produces a
  meaningless result once either point is left at its unset default --
  but the column is meant to show the single final intro-marker time
  (where the vocal actually starts), not a span between two markers.
  Now displays `talkEndPoint()` directly whenever it's set, independent
  of whether the first marker has ever been touched.

## 2026-07-06

- x64 packages for this revision were built directly on real
  x86-64-v2-class hardware instead of via the GitHub Actions CI
  runner. Ubuntu 26.04's package archive is CPU-ISA-tiered (separate
  `amd64` and `amd64v3` components), and a modern CI runner is served
  the `v3`-tier build toolchain for its entire toolchain -- including
  the CRT startup objects linked into every binary -- regardless of
  any `-march` flag this project's own build passes. The
  `x86-64-v2` cap added 2026-07-05 remains in place as the intended
  compatibility target, but cannot by itself override ISA requirements
  baked into pre-compiled objects supplied by the build machine's own
  toolchain; building on genuinely v2-tier hardware does. Confirmed
  via `readelf -n` on the real shipped `rddbmgr` (`x86-64-baseline`
  only, versus `x86-64-baseline, x86-64-v2, x86-64-v3` on every prior
  build) and a real install/reboot/library-restore round trip on
  genuine pre-Haswell hardware.

## 2026-07-05

- `debian/rules.src`: the x86-64-v2 cap below now also applies to
  `LDFLAGS`, not just `CFLAGS`/`CXXFLAGS`. This toolchain's
  `dpkg-buildflags` enables `-flto=auto` by default; under LTO, real
  target-specific code generation happens at the link step, not each
  file's compile step, so a cap that only touches `CFLAGS`/`CXXFLAGS`
  has no effect on the actual shipped binaries. Confirmed via a real
  install on genuine pre-Haswell hardware hitting the identical crash
  the cap below was supposed to fix, and via `readelf -n` showing the
  shipped `rddbmgr`'s ISA requirement was unchanged by the
  `CFLAGS`/`CXXFLAGS`-only version of this cap.

- `debian/rules.src`: amd64 builds now compile with `-march=x86-64-v2`
  instead of whatever the build machine's toolchain defaults to.
  Every C++ binary this project builds (confirmed via `readelf -n`
  across the whole package) required x86-64-v3 (AVX2/BMI1/BMI2/FMA/
  MOVBE/LZCNT) despite no `-march`/`-mtune` ever being set explicitly
  anywhere in this project's build -- a real crash on genuine
  pre-Haswell hardware (`rddbmgr: CPU ISA level is lower than
  required`, `postinst` exit 127), not a virtualization artifact.
  MariaDB itself was not the cause -- Ubuntu's own `mariadb-server`
  package already targets the universal `x86-64-baseline`. arm64
  builds are unaffected (`-march` is an x86-only concept).

- `debian/control.src`: replaced `python3-pymysql` with `python3-mysqldb`
  in `rivolution`'s `Depends`. Every PyPAD script (`apis/pypad/api/
  pypad.py`, shared by all of them) and `rdautocheck`/`rdautoback`
  import `MySQLdb` directly -- not `pymysql`, a different package with a
  different import name -- so `python3-pymysql` never actually satisfied
  anything any script in this repo imports; confirmed via a full-repo
  search that nothing anywhere imports `pymysql` at all. Found via a
  real PyPAD instance failing with `ModuleNotFoundError: No module named
  'MySQLdb'` even after 2026-07-04's unrelated stale-path fix (which
  only fixed the "Add" button's template picker, not this).

- `rivapi/store/mode_apply.go`: client mode's `/var/snd` `fstab` entry now
  carries `x-systemd.after=tailscaled.service`. Found via a real reboot
  that hung for minutes: `tailscaled.service` had already stopped by
  the time `var-snd.mount` tried to unmount, and a `hard` NFS mount
  (deliberate, for data safety) never gives up retrying against a now
  ­unreachable server, so the unmount sat there until systemd's stop
  job timeout forcibly killed it. Ordering only, not
  `x-systemd.requires=` -- that would fail the mount outright on any
  deployment where `tailscaled.service` ends up masked/disabled
  entirely (a non-Tailscale remote host), where `After=` alone is
  simply inert. Server mode needed no equivalent change: its exports
  are local bind mounts, never touching the network at unmount time.

- `rivapi/store/mode_apply.go`, `conf/sudoers.d/rivapi`: Server mode's
  `/srv/nfs4/...` NFS export tree is now bind-mounted onto the real
  audio store and staging directories instead of being permanently
  empty. Found on the first real client<->server test: `/etc/exports`
  and `createNFSExportDirs` were both correct, and a client mounted the
  export successfully, but `/srv/nfs4/var/snd` had always been a
  structurally separate, empty directory from the real `/var/snd` --
  nothing anywhere bind-mounted (or otherwise connected) the two, so
  every client saw nothing regardless of how much real audio existed
  on the server. `rd_xfer`/`music_export`/`music_import`/
  `traffic_export`/`traffic_import` are upstream Rivendell dropbox-
  location conventions this project doesn't use itself but continues
  to support for operators who do -- created empty if they don't
  already exist, rather than left unexported entirely.

- `rivapi/store/mode_apply.go`: `ApplyMode` now writes `rd.conf`'s
  `[AudioStore]` `MountSource`/`MountType` fields for client mode
  (cleared back to empty for standalone/server). Found on the first
  real client<->server test: the remote audio store was genuinely
  mounted correctly (fstab entry, live NFS mount, all confirmed), but
  Rivendell's own `RDAudioStoreValid()` (`lib/rdstatus.cpp`, used by
  `rdmonitor`/`rdselect`) decides "local vs. remote" purely from
  whether `[AudioStore] MountSource` is set, and for the remote case
  confirms the mount by matching `MountSource` against `/etc/mtab`'s
  source field. With that field left blank, Rivendell expected a plain
  local directory, found a real NFS mount sitting on top of it
  instead, and reported the station unhealthy despite the mount
  working perfectly.

- `rivapi/store/mode_apply.go`: `/mode`'s `apt-get install` calls
  (mariadb-server, nfs-kernel-server, nfs-common/autofs) now retry for
  up to 3 minutes on dpkg lock contention instead of failing outright.
  Found on the first real test of client-mode switching: the install
  step failed immediately because `unattended-upgrades` held
  `/var/lib/dpkg/lock-frontend` at that exact moment -- a routine,
  transient condition on any Ubuntu box, unrelated to anything this
  dashboard does. Nothing else was touched by the failed attempt (rd.conf,
  MariaDB, and `/etc/fstab` were all still untouched), so this was a
  clean, safe failure -- just one worth not surfacing as an error when
  waiting a few seconds and retrying would have succeeded.
- `rivapi/store/patchbay.go`: fixed `ReconcileLinks`/`DisconnectUnsaved`
  tearing down Stereo Tool's own auto-connection on every reconcile
  cycle. Found via a real reboot-and-watch test: `~/.asoundrc`'s
  `pcm.jack` auto-connect formed the correct link within a few seconds
  of every restart, then the bidirectional reconciler removed it again
  at the next tick, because Stereo Tool's JACK client name embeds its
  process ID (see 2026-07-04's entry) and the saved `patchbay.json`
  entry's PID never matches the live one after a restart. Comparisons
  now go through a new `normalizedLinkKey`/`normalizePortName` pair that
  collapses Stereo Tool's PID segment before comparing saved links
  against live ones, so a saved link is recognized as already satisfied
  (and a live link recognized as already saved) regardless of which PID
  Stereo Tool happens to have this run. The actual connect/disconnect
  calls still use the real, current port names -- only the "is this
  already there" comparison is normalized.

## 2026-07-04

- `conf/systemd/stereo-tool.service`: fixed Stereo Tool's audio patch not
  surviving a reboot without manually restarting the whole stack.
  `~/.asoundrc`'s `pcm.jack` block already auto-connects Stereo Tool's
  ALSA-JACK bridge to stable target port names on open (PID-agnostic,
  unlike the dashboard's own patchbay Save/Reconcile feature -- Stereo
  Tool's JACK client name embeds its own PID, so a saved connection can
  never match again after a restart, see `BACKLOG.md`) -- but
  `After=rivendell.service` alone wasn't enough to guarantee the target
  ports actually existed yet: confirmed via a real reboot that
  `rivendell.service`'s first start attempt commonly fails when
  `mariadb.service` isn't accepting connections yet, and a *failed*
  unit still counts as "reached" for systemd ordering purposes, so
  Stereo Tool started and gave up on the connection (its ALSA plugin
  only attempts once, at open time) well before `rivendell.service`'s
  automatic retry actually got `caed`'s JACK ports up. Fixed with an
  `ExecStartPre` that polls for the real JACK port to exist, same
  readiness-check pattern `rivendell.service`'s own `ExecStartPost`
  already uses for `caed`'s control port.
- `/patchbay`: new "Disconnect unsaved" button
  (`store.DisconnectUnsaved`) removes every live connection not in the
  saved set in one click. Found via real testing: Stereo Tool's ALSA/
  JACK driver probing multiple device instances while its I/O was
  being configured left 19 unwanted auto-detected connections behind
  on a fresh box with nothing saved yet -- `ReconcileLinks` doesn't
  touch these on its own (deliberately, since nothing has a saved
  opinion yet), and clicking "Remove" on each one individually doesn't
  scale. Also fixed the page's own description text, stale since
  today's earlier 5s-to-30s reconcile-interval change.
- `conf/sudoers.d/rivapi`: fixed a real, live regression found on a fresh
  install -- every wildcarded `Cmnd_Alias` argument (the NFS-mount
  remote host, the per-task systemd unit filenames) failed to parse
  under Ubuntu 26.04's `sudo-rs` (Ubuntu's Rust reimplementation of
  sudo, which rejects wildcards in command arguments outright, unlike
  traditional GNU sudo), silently dropping every `NOPASSWD` rule in the
  file -- not a lockout (normal password-based `sudo` still worked),
  but the dashboard's whole point of not prompting for a password
  broke entirely. Fixed by moving the varying value into small,
  fixed-path helper scripts (`store/tasks_deploy.go`'s
  `install-unit.sh`/`remove-unit.sh`/`task-systemctl.sh`,
  `store/mode_apply.go`'s `mount-var-snd.sh`), whitelisted by bare path
  with no arguments specified (which grants any arguments) and
  validated internally in shell -- works identically under both sudo
  implementations. Verified with `visudo -c` this time, not just by
  reasoning about it. See `ARCHITECTURE.md`'s new mistake-class
  write-up.
- `/mode` now requires re-entering your password before applying a
  switch, and shows a prominent warning against running it on a
  machine already live in production. Reuses `auth.CreateTicket`
  (renamed from the unexported `createTicket`) — the same real
  Rivendell-account credential that already gates the whole dashboard
  — rather than a separate app-only secret or the Linux account's own
  system password (which would need a new PAM dependency and either
  `shadow`-group membership or a privileged helper to verify at all).
  Checked before anything is saved or touched; a failed confirmation
  changes nothing.
- New dashboard page, `/export`: bundles the broadcast/Icecast/
  Liquidsoap config, patchbay routing, install mode, and scheduled
  tasks — plus Stereo Tool's own `~/.stereo_tool.rc` and saved
  `~/.stereo_tool.presets/*.sts` presets, base64-encoded since they're
  not JSON-native — into one downloadable/importable JSON file, for
  migration or disaster recovery. Two of the four dashboard configs
  were already single-source-of-truth JSON (`broadcast.json`,
  `patchbay.json`); this just bundles what already existed rather than
  retrofitting anything. Import restores every file, regenerates
  `icecast.xml`/`radio.liq` and restarts Icecast/Liquidsoap, and
  redeploys every scheduled task's systemd units — but deliberately
  does **not** auto-apply the restored install mode (mounting NFS,
  restarting MariaDB), since that's a much bigger side effect than
  restoring a config file; the operator applies it deliberately from
  `/mode` afterward. New files: `rivapi/store/export.go`,
  `rivapi/dashboard/handlers_export.go`,
  `rivapi/dashboard/templates/export.html`. **Not yet verified on a
  real box**, same caveat as `/mode` and `/tasks`.
- New dashboard page, `/tasks`: closes the gap tracked in `BACKLOG.md`
  since the old Ansible `broadcast_advanced` role was removed
  2026-07-01 (nightly DB backup and log generation, previously
  hand-maintained crontab entries with no dashboard equivalent). Each
  task gets its own systemd service+timer pair (visible in
  `systemctl list-timers`), not a re-implementation of cron. Four task
  types: database backup (reads connection details from `/etc/rd.conf`
  at run time via a small fixed helper script, rather than storing a
  password in the task itself — the exact bug class found in one of
  the scripts this replaces), log generation, log reconciliation
  (`rdlogmanager` wrappers), and a custom-command escape hatch for
  anything else. Task IDs are always server-generated 16-character hex
  strings, never derived from operator-entered text, before they're
  ever used in a systemd unit name or file path — the wildcarded
  sudoers entries this needs (`conf/sudoers.d/rivapi`) only ever need
  to match rivapi's own generated filenames as a result. New files:
  `rivapi/store/tasks.go`, `rivapi/store/tasks_deploy.go`,
  `rivapi/dashboard/handlers_tasks.go`,
  `rivapi/dashboard/templates/tasks.html`. **Not yet verified on a
  real box**, same caveat as `/mode` below.
- New dashboard page, `/mode`: switches a station between standalone/
  server/client network topologies in one click, replacing what used to
  require a full Ansible re-provision. Standalone/server ensure
  `mariadb-server` is installed and running with the correct
  bind-address (loopback vs. all-interfaces — the SQL-level grants for
  both are already created unconditionally by
  `scripts/rivolution-first-run.sh`, so this is the actual gate on
  remote reachability, not the grant); server additionally exports
  `/var/snd` and the staging directories over NFS. Client mounts the
  remote audio store, persists it via `/etc/fstab` and `autofs`, and
  points `/etc/rd.conf`'s `[mySQL]` section at a remote host instead of
  provisioning anything locally. Every privileged step goes through a
  small, fixed sudoers whitelist (`conf/sudoers.d/rivapi`) — package
  installs, mount/umount, and a "regenerate whole file, then one
  whitelisted `install`" pattern for `rd.conf`/`/etc/exports`/
  `/etc/fstab`/the autofs maps, matching how `icecast.xml`/`radio.liq`
  already deploy. Never uninstalls anything or drops data when
  switching away from a mode. New files: `rivapi/store/mode.go`,
  `rivapi/store/mode_apply.go`, `rivapi/dashboard/handlers_mode.go`,
  `rivapi/dashboard/templates/mode.html`. **Not yet verified on a real
  box** — built following this project's existing patterns, but this
  needs the same real-install testing regimen as everything else in
  this fork before being trusted in production.
- `rivapi/store/patchbay.go`, `rivapi/main.go`: the patchbay link
  reconciler now removes live connections that aren't in the saved
  set, not just adds missing ones — previously additive-only, which
  let WirePlumber's own default auto-linking connect ports on its own
  at boot and left them there indefinitely alongside the saved
  patches, only ever cleared by a manual full stack restart. Reconcile
  interval widened from 5s to 30s at the same time, since the
  reconciler is now authoritative and a too-short interval would tear
  out an ad-hoc test connection before there's time to listen to it.
  See `KNOWN_ISSUES.md`.
- `debian/postinst`: the `rivendell`/`pypad` system account creation
  (`groupadd`/`useradd`) now checks `getent` first instead of running
  unconditionally under `set -e`. Found via a real install that failed
  for an unrelated reason (see `KNOWN_ISSUES.md`'s CPU-ISA entry), then
  failed again on retry with `groupadd: group 'rivendell' already
  exists` — a self-inflicted failure caused by re-running `postinst`
  against its own partially-completed prior attempt, not the original
  problem. See `ARCHITECTURE.md`'s "`postinst` must tolerate
  re-running against its own partial output."
- `lib/rdpaths.h.in`: fixed four install-path constants
  (`RD_PYPAD_SCRIPT_DIR`, `RD_CDN_SCRIPT_DIR`,
  `RD_DEFAULT_RDAIRPLAY_SKIN`, `RD_DEFAULT_RDPANEL_SKIN`) that still
  pointed at this project's pre-rebrand paths after the corresponding
  install rules had already moved to their current locations. Silent
  on a dev box carrying stale files from an older build (both old and
  new directories exist there), but caused RDAdmin's PyPAD "Add"
  button to silently fail to open its template picker on a genuinely
  fresh install. See `KNOWN_ISSUES.md`.
- A follow-up forensic sweep for the same stale-path pattern found and
  fixed twelve more instances: RDAirPlay's top-strip logo
  (`rdairplay/topstrip.cpp`), the `LOGO_PATH`/`SKIN_PATH` column
  defaults applied during fresh-install and downgrade schema
  migrations (`utils/rddbmgr/updateschema.cpp`,
  `utils/rddbmgr/revertschema.cpp`), the Akamai CDN purge script's key
  file location (`apis/cdn/scripts/aka_purge.sh`), and the RSS feed
  XSL stylesheet paths used by feed generation and reporting
  (`lib/rdfeed.cpp`, `rdadmin/feedlistview.cpp`,
  `rdcastmanager/rdcastmanager.cpp`). The XSL paths failed loudly (a
  visible `xsltproc` error dialog); the rest failed silently, same as
  the PyPAD bug above. RPM packaging (`rivendell.spec.in`) and the
  unused legacy `build_debs.sh.in`/`configure.ac` tarball-naming path
  were confirmed to have the same old paths but are dead code not
  exercised by this project's actual Debian packaging pipeline —
  deliberately left alone.

## 2026-07-03

- `lib/rdwavefactory.cpp`, `lib/rdmarkerview.cpp`: fixed the Edit
  Markers waveform going blank or truncated on long cuts at high zoom
  (same root cause as upstream `ElvishArtisan/rivendell` issue #835,
  open since 2022 with only a zoom-level cap as a stopgap). The
  waveform previously rendered into a single oversized image that
  silently corrupted past the display toolkit's maximum image width;
  it's now rendered as a strip of bounded-width tiles, and the
  now-unnecessary zoom-level cap has been removed. A separate, known
  ~26ms zoom precision floor (from the underlying peak data's own
  resolution) remains and is documented in `BACKLOG.md`/
  `KNOWN_ISSUES.md` as deferred.
- `debian/control.src`: added `mp3gain` to `rivolution`'s `Depends`. The
  MP3 gain-patch passthrough path (spec 0004) silently falls back to a
  full decode/re-encode whenever `mp3gain` isn't present, so its absence
  produced no obvious symptom on its own -- only surfaced once the
  fallback conversion's own separate bug (below) turned it into an
  outright import failure on a real install lacking the binary.
- `debian/postinst`, `scripts/rivolution-first-run.sh`: `/var/snd` is
  now `rd:rivendell` (was `rivendell:rivendell`) in both the `.deb` and
  from-source install paths, applied unconditionally so an existing
  install self-heals on upgrade rather than only fixing fresh installs.
  `rd` owns it outright for native `rdimport`/`caed` writes; `rivendell`
  keeps write access via the group for `rdxport.cgi`'s setuid-drop
  identity; and unlike relying on supplementary group membership alone,
  this also allows a third-party desktop sync client staging files in
  as `rd` to write into `/var/snd` directly.
- `web/rdxport/import.cpp`: when the MP3 gain-patch passthrough fails
  (`mp3gain` missing, or any other patch failure) and falls back to a
  full conversion, the destination format/bitrate are now taken from
  the station's own configured library default instead of carrying the
  request's format override forward. Previously the fallback kept
  targeting whatever format the passthrough attempt had been aiming
  for (typically MP3) using the library's configured bitrate, which is
  only ever set (and otherwise sits at `0`) when that library's default
  format is itself MP3/Layer 2 -- on a library whose default is PCM,
  falling back to MP3 encoding at bitrate `0` failed outright
  ("Unable to create destination file"), and re-encoding into another
  lossy format on a fallback path would have added an avoidable second
  generation of lossy compression regardless.
- `web/rdxport/import.cpp`: MP3 passthrough imports now support
  autotrim. `RDWaveFile::startTrim()`/`endTrim()` already decode MP3 in
  memory via `libmad` to measure real sample-accurate trim points
  (`GetEnergy()`/`LoadEnergyMpegLayer3()`) -- the same decode already
  run against every passthrough import's destination file for LEVL/peak
  persistence, whose result persists into the file's own LEVL chunk and
  is reused rather than recomputed. Requesting autotrim no longer
  disqualifies a file from the passthrough path.
- `.github/workflows/build-deb.yml`: added GitHub Actions CI to build
  x64 `.deb` packages on `ubuntu-26.04` runners, triggered on version
  tag pushes (and manually via `workflow_dispatch`). On a tag push it
  attaches the built packages to that tag's GitHub Release and appends
  an x64 download/install section to the release notes, leaving any
  existing arm64 notes untouched. `scripts/rebuild-deb.sh` gained a
  `--no-bump` flag so CI builds the revision a tag already points at
  instead of minting a new one.
- `.github/workflows/build-deb.yml`: tags ending in `-test` (e.g.
  `v6.0.0-1-test`) are now excluded from the x64 build trigger, so the
  manual ARM64 tag/release flow can be exercised without also kicking
  off a real x64 build. `scripts/rebuild-deb.sh` gained a
  `--version=X.Y.Z` flag to change the upstream version string itself
  (writing `versions/PACKAGE_VERSION` and resetting the Debian revision
  to 1), separate from its existing same-version revision auto-bump.
- `.github/workflows/build-deb.yml`, `scripts/rebuild-deb.sh`: fixed
  broken wget/`apt install` links in release notes for any version
  containing `~` (e.g. `6.0.0~beta1-1`). GitHub Releases silently
  renames `~` to `.` in uploaded asset filenames, so a download command
  built from the real Debian version string 404s. `build-deb.yml`'s
  x64 release-notes step now substitutes `.` for `~` before building
  the URL; `rebuild-deb.sh` prints the correct dot-form filename at the
  end of a build so the manual arm64 release notes use the right name
  too. Caught on the first `v6.0.0~beta1-1` release, whose notes
  shipped with 404ing links in both the arm64 and x64 sections until
  directly verified by curling the URL.
- `.github/workflows/build-deb.yml`: added a second x64 build target,
  Ubuntu 24.04, alongside the existing Ubuntu 26.04 primary target --
  best-effort and temporary, for cloud providers that don't yet offer a
  26.04 image. The build job is now a `strategy.matrix` over both OSes
  (`max-parallel: 1`, since the release-notes step does a
  read-modify-write against the same release and can't safely run for
  both legs concurrently). The 24.04 build's `.deb` filenames get a
  `-noble` suffix to avoid colliding with the 26.04 build's identically
  -named packages; the arch-independent opsguide package isn't
  re-uploaded a second time under a different name, since its content
  is identical either way. No dependency-list changes were needed
  between the two OSes -- `rivolution.wiki`'s own `Build-From-Source.md`
  had already verified the same package list builds cleanly on both.
  `workflow_dispatch` gained a `target` input (`both`/`26.04`/`24.04`)
  so a single leg can be tested manually without spending runner time
  building the other one too.

## 2026-07-02 (continued, 7)

- `debian/postinst`: added `rd` to the `rivendell` group. `/var/snd` is
  created `rivendell:rivendell` mode `775` -- correct under the
  pre-`User=rd` model, but `caed`/`rdimport` running as `rd` only ever
  got that mode's "other" bits there (`r-x`, no write). Audio import
  failed outright ("Unable to create destination file") on a real
  install -- unlike this session's other packaging bugs, not masked by
  dev-box state (this dev box has the same gap), but by audio import
  specifically never having been exercised in any of this fork's real-
  system verification until now. Bumped the Debian revision to `-6`.

## 2026-07-02 (continued, 6)

- `debian/control.src`: added `libasound2-plugins` to `rivolution`'s
  `Depends`. Found via a real install of `v6.0.0int0-4`: Stereo Tool's
  ALSA `jack` PCM plugin was never installed at all -- already present
  on the dev box from earlier manual setup, so its absence went
  completely unnoticed. Without it, Stereo Tool's ALSA layer falls back
  to real hardware and fails outright instead of ever reaching JACK.
- `debian/rules.src`, `debian/postinst`: `conf/alsa/rd.asoundrc` is
  deployed to the `rd` user's `~/.asoundrc` again, correcting an
  earlier decision (recorded in spec 0010) that it was fully superseded
  by `/patchbay`'s dynamic routing. It isn't -- `/patchbay` only
  reconnects ports that already exist; `rd.asoundrc`'s job (giving
  Stereo Tool's ALSA `jack` PCM type valid, existing port names instead
  of the stock definition's hardcoded nonexistent ones) is a separate,
  still-required baseline underneath it.
- Debian revision bumped to `-5`.

## 2026-07-02 (continued, 5)

- `debian/postinst`: stopped calling `dpkg-architecture` to compute the
  multiarch triplet for the `pipewire-jack` `ld.so.conf.d` fix. Found
  via a real install of `v6.0.0int0-3` on a clean arm64 system:
  `dpkg-architecture` lives in `dpkg-dev`, a build-time-only package --
  present on this fork's own dev box (where every `.deb` gets built,
  which is why this passed local testing) but not on a plain install
  target, so `postinst` failed outright with `dpkg-architecture: not
  found` (exit 127). Now reads the triplet straight off the filesystem
  (`/usr/lib/*-linux-gnu`, always present on any multiarch Debian/
  Ubuntu system) instead of depending on any external tool.

## 2026-07-02 (continued, 4)

- `debian/control.src`: `pipewire-jack` is now a hard `Depends`, not an
  alternative with `libjack-jackd2-0`. Found via a real install of
  `v6.0.0int0-2` on a clean arm64 system: apt satisfied the alternative
  with `libjack-jackd2-0` instead, which has no relationship to this
  fork's system-scope PipeWire instance -- `/patchbay` showed no JACK
  sources at all as a result, since `caed`/`liquidsoap` were linking
  against a real, unrelated JACK library with no daemon behind it.
  `debian/shlibs.local`'s override simplified to match (`pipewire-jack`
  only, dropping the alternative).
- `debian/postinst`: the `ld.so.conf.d` ordering fix now copies
  `pipewire-jack`'s conf snippet into place before renaming it.
  Previously only renamed an *existing* file -- confirmed on the same
  real install that `pipewire-jack` ships this file solely as a doc
  example (`/usr/share/doc/pipewire/examples/ld.so.conf.d/`), never
  auto-installed to `/etc/ld.so.conf.d/`, so the rename-only step
  silently found nothing to act on and JACK never worked. Both bugs
  compounded: even with `pipewire-jack` now guaranteed installed, this
  fix was still required for the ordering itself to take effect.
- `debian/control.src`: added `gedit` to `rivolution`'s `Depends`
  (already noted in `INSTALL.md`'s build-dependency list, never
  reflected in the actual package).
- Debian revision bumped to `-3` (`debian/changelog.src` + the four
  exact-version `Depends` in `debian/control.src`) -- real content
  changes on top of the already-published `v6.0.0int0-2`.

## 2026-07-02 (continued, 3)

- `debian/control.src`: added `mariadb-server` to `rivolution`'s
  `Depends` (was `mariadb-client` only). Found via a real install on a
  clean arm64 system: `postinst`'s new database-creation step (see the
  previous entry) needs a running `mysqld` to connect to, which was
  never guaranteed to exist — `mariadb-client` only provides the `mysql`
  CLI tool, not the server daemon. `apt install` now pulls in
  `mariadb-server` automatically, whose own `postinst` starts `mysqld`
  before `rivolution`'s `postinst` runs (guaranteed by dependency
  ordering), so the connection succeeds.

## 2026-07-02 (continued, 2)

- `debian/postinst`: full rewrite folding in `scripts/rivolution-first-run.sh`'s
  logic (JWT/DB-password generation, empty-DB creation + seed +
  test-tone via `rddbmgr --create --generate-audio`, PulseAudio
  disable, `rd`/`audio` group membership, `rtprio`/`memlock` limits)
  gated on `$2` (Debian policy's postinst "previously configured
  version" argument) being empty -- fresh installs create/seed the
  database, upgrades only run `rddbmgr --modify` against the existing
  one, preserving whatever secrets/data are already there. Also now
  deploys the full spec 0007/0008/0010 broadcast/PipeWire/rivapi layer
  automatically: every `conf/systemd/*` unit and drop-in, the
  `ld.so.conf.d` ordering fix (idempotent, upgrade-safe), sudoers rule,
  udev rule, and enables/starts `pipewire-system.service`/
  `wireplumber-system.service` before `rivendell.service` restarts (the
  JACK driver needs the socket present at startup). The DB migration
  now explicitly runs before the `rivendell.service` restart it was
  previously positioned after -- a schema/binary mismatch hard-blocks
  startup. `scripts/rivolution-first-run.sh` left in place for from-source
  installs that skip `dpkg-buildpackage` entirely, with a note
  pointing at `postinst` as the primary path now.
- `debian/control.src`: added `golang-go` to `Build-Depends`;
  `icecast2`, `liquidsoap`, `fdkaac`, `vlc`, `vlc-plugin-jack`,
  `pipewire`, `wireplumber` to `rivolution`'s `Depends` -- previously
  entirely unreferenced by packaging despite being required for the
  verified-working broadcast stack.
- `debian/rules.src`: added a `go build` step for `rivapi` alongside
  the existing autotools build, and staged the full broadcast/PipeWire
  layer (`conf/systemd/*`, `conf/sudoers.d/rivapi`,
  `conf/udev/99-ptp.rules`, the `rivapi` binary) into the `rivolution`
  package under `/usr/share/rivolution/` for `postinst` to deploy, same
  pattern as the existing `rd.conf-sample` staging.
- `debian/rules.src`: `dh_dwz -Xrivapi` -- Go's linker compresses its
  own DWARF debug info in a format `dwz` can't process; `dh_dwz`
  doesn't skip just that file, it aborts its entire batch (every C++
  binary's debug-info step failed too). Excluding `rivapi` from `dwz`
  only affects the separate `-dbgsym` debug-symbol package (a `gdb`
  crash-debugging aid, unused during normal operation); `dh_strip`
  runs on `rivapi` immediately afterward, unaffected -- the shipped
  binary itself is unchanged.
- `conf/systemd/rivapi.service`, `conf/sudoers.d/rivapi`,
  `scripts/rivapi-rebuild.sh`: standardized the installed `rivapi`
  binary path from `/usr/local/bin/rivapi` to `/usr/bin/rivapi` --
  `/usr/local` is reserved for locally-installed, non-package-managed
  software; a `.deb`-shipped binary shouldn't write there. All three
  install paths (`.deb`, `scripts/rivapi-rebuild.sh` dev workflow, and
  the sudoers `RIVAPI_INSTALL` alias) now agree.
- `dpkg-buildpackage -us -uc -b` verified producing all 7 packages
  cleanly with the complete broadcast/PipeWire/rivapi automation
  included, not just the core Rivendell build from earlier today.

## 2026-07-02

- `rivapi/dashboard/handlers.go`: added `serverHost()`, deriving the
  hostname from the current request (honoring `X-Forwarded-Host` under
  `TrustProxyHeaders`) instead of a hardcoded `localhost` or a
  separately-configured public hostname. `baseData` now carries
  `ServerHost`/`StereoToolWebPort`; `base()` takes `*http.Request`
  (all call sites across `handlers.go`, `handlers_broadcast.go`,
  `handlers_patchbay.go` updated).
- `rivapi/config/config.go`: added `StereoToolWebPort`
  (`RIVAPI_STEREO_TOOL_WEB_PORT`, default 8079, matching
  `conf/systemd/stereo-tool.service`'s `-p` flag).
- `rivapi/dashboard/templates/base.html`: added a "Processing" nav link
  to Stereo Tool's web config UI, built from `ServerHost` so it resolves
  correctly whether the operator is on localhost, the LAN, or connected
  over Tailscale — no per-install configuration needed.
- `rivapi/dashboard/templates/broadcast.html`: Icecast Admin link's
  hostname switched from the configured public listener hostname
  (`icecast.hostname`, meant for stream URLs, not necessarily how the
  operator's browser reached the dashboard) to `ServerHost`, same
  reasoning as the Processing link. Port still comes from the live
  Icecast config.
- `debian/rules.src`: fixed a real GNU Make pitfall blocking
  `dpkg-buildpackage` entirely. `build-arch`/`build-indep`/`binary`/
  `binary-indep` had no recipe of their own, so Make also matched the
  catch-all `%: dh $@` pattern rule and silently ran a second, full
  `dh build-arch` sequence with debhelper's own default flags right
  after our custom `build:` recipe finished — discarding our custom
  `--libdir`/`--libexecdir` and reconfiguring/rebuilding the whole tree
  a second time. Fixed with explicit `@:` no-op recipes on every
  formerly-empty target.
- `debian/shlibs.local` (new), `debian/control.src`: `caed`/`ripcd`
  link against `pipewire-jack`'s `libjack.so.0` shim, which ships no
  `.shlibs`/`.symbols` file at all (confirmed via `dpkg -L
  pipewire-jack`), so `dpkg-shlibdeps` couldn't resolve a dependency.
  `-xpipewire-jack` looked plausible but doesn't fix this — per
  `dpkg-shlibdeps(1)` it's for same-package self-dependency avoidance
  only. Fixed with the documented override mechanism,
  `debian/shlibs.local`, plus `pipewire-jack | libjack-jackd2-0` added
  to `rivolution`'s `Depends`. `dpkg-buildpackage -us -uc -b` now
  succeeds end-to-end, all 7 packages build.

## 2026-07-01 (continued, 28)

- `rivapi/dashboard/templates/base.html`, `templates/home.html`:
  hidden the Groups and Carts nav links/buttons (template comments,
  routes/handlers untouched) — not yet meaningful standalone; see
  `BACKLOG.md`, revisit once RDAdmin porting reaches them.

## 2026-07-01 (continued, 27)

- `rivapi/dashboard/handlers_patchbay.go`, `templates/patchbay.html`:
  replaced the output x input matrix table with a connections list
  (Output -> Input rows, Remove button) plus an "Add connection" form
  (two dropdowns). The matrix didn't fit on screen at a readable zoom
  level with more than a handful of ports — this scales with the
  number of connections instead of outputs x inputs. `/patchbay/toggle`
  split into `/patchbay/connect` and `/patchbay/disconnect` to match.
  Reconciler-based persistence (added in the previous entry) verified
  working live — survived a real Liquidsoap restart with the saved
  patch silently reconnecting, no manual action needed.

## 2026-07-01 (continued, 26)

- `rivapi/store/patchbay.go`: added `SaveDesiredLinks`/`LoadDesiredLinks`/
  `ReconcileLinks`, and a background poll loop in `main.go` (every 5s)
  that re-applies any saved link missing from the live graph. This is
  the persistence mechanism for `/patchbay`, not WirePlumber policy —
  verified empirically that WirePlumber's declarative `target.node`
  metadata mechanism does not apply to JACK-bridged ports (three
  independent findings: no link forms, an internal WirePlumber script
  throws a Lua exception on these nodes, and the metadata itself is
  keyed by an ephemeral node.id that changes every restart anyway).
  Full writeup in `docs/specs/0007-pipewire-audio-engine.md`'s
  Implementation deviations.
- `dashboard/handlers_patchbay.go`, `templates/patchbay.html`: added a
  "Save current patch" button and a 4-state indicator per cell
  (connected+saved / connected only / saved only / neither).

## 2026-07-01 (continued, 25)

- `rivapi/store/patchbay.go`, `dashboard/handlers_patchbay.go`,
  `dashboard/templates/patchbay.html` (new): MVP visual patchbay —
  `/patchbay` shows every PipeWire output x input port as a clickable
  connect/disconnect matrix, backed directly by `pw-link`. Replaces
  needing to hand-edit `conf/alsa/rd.asoundrc` or SSH in to run
  `pw-link` manually. Intentionally minimal (no client grouping, no
  live refresh, no persistence across a client restart) — see
  `BACKLOG.md` for what's deliberately deferred to a later pass.
- `conf/systemd/rivapi.service`: added `Environment=XDG_RUNTIME_DIR=/run/pipewire-system`
  (needed for the patchbay's `pw-link` calls) and `After=pipewire-system.service`.

## 2026-07-01 (continued, 24)

- `conf/systemd/rivapi.service` (new): rivapi now runs as a real systemd
  service — survives reboot, no more manual `go build && ./rivapi`.
  Deliberately independent of `rivolution-stack.target` (it's the
  process that controls the stack from the dashboard, so it can't be
  stopped along with it). No `EnvironmentFile=` needed — reads DB
  credentials and the dashboard JWT secret from `/etc/rd.conf`, same as
  every other Rivendell binary.
- `scripts/rivapi-rebuild.sh` (new): one-command build + install +
  restart, replacing the manual `cd rivapi && go build -o rivapi . &&
  ./rivapi` dev workflow.
- `conf/sudoers.d/rivapi`: added `restart rivapi.service` to
  `RIVAPI_SYSTEMCTL` and the `/usr/local/bin/rivapi` install path to
  `RIVAPI_INSTALL`, both needed by the rebuild script.

## 2026-07-01 (continued, 23)

- `conf/systemd/stereo-tool.service`: added `-p 8079` to expose Stereo
  Tool's web config UI (needed since it runs headless, no X11 display).
- `conf/alsa/rd.asoundrc` (new): overrides the stock `pcm.jack` ALSA
  definition (`/usr/share/alsa/alsa.conf.d/50-jack.conf`), which
  hardcodes `system:capture_1/2`/`system:playback_1/2` — real jackd+ALSA
  hardware port names that don't exist under system-scope PipeWire.
  Stereo Tool's "jack (ALSA)" I/O option routes through this ALSA-JACK
  bridge plugin, not as a native JACK client, so this was needed for it
  to reach caed/Liquidsoap at all. Pins caed stream 0 -> Stereo Tool
  input, Stereo Tool output -> Liquidsoap for now — temporary/hardcoded
  until the dashboard's visual patch matrix (dynamic WirePlumber-backed
  routing) replaces it; see BACKLOG.md.

## 2026-07-01 (continued, 22)

- `conf/systemd/stereo-tool.service`: added the missing
  `Environment=XDG_RUNTIME_DIR=/run/pipewire-system` (same pattern as
  `rivendell.service.d`/`liquidsoap.service`) and `After=pipewire-system.service`.
  Without it, Stereo Tool had no way to find the system-scope PipeWire
  socket and was very likely falling back to the user-session PipeWire
  instance instead — its own log showed repeated JACK auto-connect
  failures (`cannot connect system:capture_1 to stereo_tool...`),
  and it never appeared in the system-scope PipeWire graph. Also fixed
  the unit's description comment, which had the signal chain backwards
  (said "downstream of Liquidsoap"; it's upstream — caed -> Stereo Tool
  -> Liquidsoap).

## 2026-07-01 (continued, 21)

- `rivapi/store/broadcast_config.go`: added `StreamConfig.Quality`
  (float64) for Vorbis VBR quality, separate from `Bitrate`.
  `rivapi/store/liquidsoap_generator.go`: ogg streams now generate
  `%vorbis(quality=...)` instead of `%vorbis(bitrate=...)` — Liquidsoap's
  Vorbis encoder has no `bitrate` parameter at all ("unknown parameter
  name (bitrate)"), it's quality/VBR-based, same class of API-version
  mismatch as the AAC argument-name fixes above.
  `rivapi/dashboard/templates/broadcast.html`: the stream form now shows
  a Quality slider (-0.2 to 1.0) instead of the Bitrate field when codec
  is "ogg", so the dashboard only ever presents fields the encoder
  actually accepts.

## 2026-07-01 (continued, 20)

- `rivapi/store/liquidsoap_generator.go`: added the missing `-f 2`
  (ADTS transport format) flag to the `fdkaac` command line. Without
  it, fdkaac defaults to muxing into an M4A container, which needs to
  seek back and write its moov box and so refuses to stream to stdout
  ("stdout streaming is not available on M4A output") — Liquidsoap saw
  this as the encoder process dying immediately, surfacing as a
  "Broken pipe in write()" crash loop. ADTS is a continuous streamable
  bitstream instead.

## 2026-07-01 (continued, 19)

- `rivapi/store/liquidsoap_generator.go`: fixed the `fdkaac` command line
  for AAC streams — Ubuntu's `fdkaac` 1.0.0 has no `-i` flag (input is a
  positional argument, not `-i <file>`); the old `-i - -o -` failed with
  "invalid option -- 'i'". Also switched `--profile` from 5/29 (HE-AAC)
  to 2 (AAC-LC): Ubuntu's `libfdk-aac2` ships with SBR encoding disabled
  (patent-restricted), so the HE-AAC profiles error "unsupported
  profile". he-aac-v1/v2 now produce plain AAC-LC; see `BACKLOG.md` for
  the real fix.

## 2026-07-01 (continued, 18)

- `rivapi/store/liquidsoap_generator.go`: AAC stream's `output.icecast()`
  also needs `send_icy_metadata=true` explicitly — Liquidsoap can infer
  this for `%mp3`/`%ogg` but not for the `%external` encoder AAC streams
  use, and errors at load time ("Could not guess send_icy_metadata for
  this format") if left unset. Found immediately after fixing the
  `format=` argument name above, during the same verification pass.

## 2026-07-01 (continued, 17)

- `rivapi/store/liquidsoap_generator.go`: AAC stream output was passing
  `content_type="audio/aacp"` to `output.icecast()`, an argument name
  from an older Liquidsoap API. Liquidsoap 2.4.x's `output.icecast()`
  has no `content_type` argument at all — the equivalent is `format`.
  Fixed the generated field name; caught when the AAC stream's
  `radio.liq` failed to parse and Liquidsoap auto-restart-looped.

## 2026-07-01 (continued, 16)

- `lib/dbversion.h`: bumped `RD_VERSION_DATABASE` 378 -> 379.
  `utils/rddbmgr/updateschema.cpp` and `revertschema.cpp`: widen
  `STATIONS.JACK_VERSION` from `char(16)`/`varchar(16)` to `varchar(64)`.
  The column was sized for a real jackd's short version string; the
  PipeWire JACK shim's `jack_get_version_string()` returns a longer
  descriptive string that was silently failing to save.

## 2026-07-01 (continued, 15)

- `rdservice/rdservice.cpp`: removed `geteuid()!=0` root check (lines
  89–92). rdservice can now run as the `rd` user; the `User=rd` drop-in
  in `conf/systemd/rivendell.service.d/rivolution.conf` is now safe to
  deploy once the rebuilt binary is installed.
- `conf/systemd/pipewire-system.service` (new): system-scope PipeWire
  running as `rd`, socket at `/run/pipewire-system/pipewire-0`. Ubuntu
  26.04 ships only user-scope PipeWire units; this fills the gap.
- `conf/systemd/wireplumber-system.service` (new): system-scope
  WirePlumber bound to `pipewire-system.service`.
- `conf/systemd/rivolution-stack.target`: added `Wants=` for both new
  PipeWire services.
- `conf/systemd/rivendell.service.d/rivolution.conf` and
  `conf/systemd/liquidsoap.service`: added
  `Environment=XDG_RUNTIME_DIR=/run/pipewire-system` so caed's JACK
  calls and Liquidsoap's `input.jack()` route to the system PipeWire
  socket via `pipewire-jack`.
- `rivapi/store/service_status.go`: added PipeWire and WirePlumber to
  the managed unit list (System page).
- `docs/specs/0007-pipewire-audio-engine.md`: added Phase 1
  implementation section documenting the system-scope setup, runtime
  prerequisites, and known gap vs. Phase 2.

## 2026-07-01 (continued, 14)

- `rivapi/dashboard/templates/home.html`: new home page — four nav buttons
  (System, Broadcast, Groups, Carts). Placeholder for future system status.
- `rivapi/dashboard/handlers.go`: `Root` now renders the home page instead
  of redirecting to `/groups`. Login already redirected to `/`; landing page
  is now home rather than groups.

## 2026-07-01 (continued, 13)

- `rivapi/store/service_status.go`: switched from `systemctl is-active`
  to `systemctl show --property=ActiveState,SubState` for richer state
  reporting; added `Detail` field to `ServiceStatus` surfacing informative
  sub-states (e.g. "activating (auto-restart)", "activating (start-post)");
  added `Warn` field and `liquidsoapWarn()` health check that reads the last
  8 KB of the Liquidsoap log for a JACK device failure within the last 10
  minutes — clears automatically once PipeWire bridge is running.
- `rivapi/dashboard/templates/system_status.html`: render Detail in muted
  text and Warn as a warning line (⚠) below the state chip.
- `rivapi/dashboard/templates/broadcast.html`: moved "+ Add stream" and
  "Save & Deploy" to separate rows so they don't overlap on narrow viewports.

## 2026-07-01 (continued, 12)

- `rivapi/dashboard/templates/broadcast.html`: moved "+ Add stream"
  button from the streams section header to below the last stream card,
  paired with "Save & Deploy" in a bottom action row. New stream fields
  now appear immediately above the button when clicked.

## 2026-07-01 (continued, 11)

- `docs/specs/0010-systemd-stack-orchestration.md`: added implementation
  deviation note and deployment prerequisite warning for the
  `rivendell.service.d/rivolution.conf` drop-in. `rdservice/rdservice.cpp`
  lines 89–92 contain a hardcoded `geteuid()!=0` check that exits with
  `ExitNoPerms` when the service runs as `rd`; combined with
  `Restart=always` and a 30-second `ExecStartPost` probe this creates a
  ~32-second infinite restart cycle. The drop-in must not be deployed
  until the check is removed and `rdservice` rebuilt.

## 2026-07-01 (continued, 10)

- `rivapi/store/liquidsoap_generator.go`: `liqStreamURL()` — auto-
  generates the `url=` parameter in each `output.icecast()` call as
  `http://icecast_host:icecast_port/mount` (the direct playback URL
  Icecast renders as a "Listen Live" link). A per-stream URL override
  is still accepted for reverse-proxy deployments. `station.url` no
  longer falls through to this field — it is the station website only.
- `rivapi/dashboard/templates/broadcast.html`: per-stream "Stream URL
  override" field placeholder now shows the auto-computed value so the
  operator can see what will be generated without filling it in.

## 2026-07-01 (continued, 9)

- `rivapi/store/icecast_generator.go`: removed `<burst-on-connect>`,
  `<queue-size>`, `<client-timeout>`, `<header-timeout>`,
  `<source-timeout>` from the `<limits>` block — all were removed from
  the Icecast 2.5.0 XML schema, causing config validation failure and
  the "obsolete tags" + "did not validate" errors in the admin dashboard.
- `rivapi/store/liquidsoap_generator.go`: touch the log file
  (`O_CREATE|O_APPEND`) after creating the log directory — Liquidsoap
  opens the path on start and fails if the file doesn't exist.
- `rivapi/dashboard/templates/broadcast.html`: Icecast Admin ↗ button
  (computed URL from hostname:port, opens `/admin/` in new tab); per-
  stream computed stream URL display (`http://host:port/mount`);
  station URL field re-labeled "Station website URL" with explanatory
  note; per-stream website URL override changed from `type="url"` to
  `type="text"` (field accepts paths like `/192`, not only full URLs).

## 2026-07-01 (continued, 8)

- `rivapi/store/icecast_generator.go`: added `<mount type="normal">`
  sections to the generated icecast.xml, one per configured stream, so
  mount points are visible in the config file (previously absent).
- `rivapi/store/broadcast_config.go`: fixed default Liquidsoap log path
  from `/home/rd/logs/liquidsoap.log` (wrong) to `/home/rd/Log/liquidsoap.log`
  (correct path on the host); Liquidsoap fails on start if the path is wrong.
- `rivapi/store/liquidsoap_generator.go`: added `os.MkdirAll` for the
  configured log directory at deploy time so Liquidsoap can open its log
  immediately on first start.
- `conf/systemd/liquidsoap.service` (new): full standalone unit — the
  Ubuntu `liquidsoap` package ships no service unit, so the drop-in
  ExecStart override had nothing to apply to.
- `conf/systemd/liquidsoap.service.d/rivolution.conf`: removed `[Service]`
  ExecStart lines (now in the main unit above); kept `[Unit]` After/PartOf.

## 2026-07-01 (continued, 7)

- `rivapi/store/broadcast_config.go` (new): `BroadcastConfig` type
  (station defaults, per-stream overrides, Icecast and Liquidsoap
  config structs), `LoadBroadcastConfig` (reads JSON, returns defaults
  when file absent), `SaveBroadcastConfig` (atomic write).
- `rivapi/store/icecast_generator.go` (new): generates `icecast.xml`
  from `BroadcastConfig`; `<sources>` auto-calculated from stream
  count; installs to `/etc/icecast2/icecast.xml` via scoped
  `sudo install`.
- `rivapi/store/liquidsoap_generator.go` (new): generates `radio.liq`
  from `BroadcastConfig`; MP3 uses `%mp3`, HE-AAC v1/v2 use
  `%external` + `fdkaac` CLI with `content_type="audio/aacp"`, OGG
  uses `%ogg(%vorbis)`.
- `rivapi/config/config.go`: added `BroadcastConfigPath` /
  `RIVAPI_BROADCAST_CONFIG` (default
  `/home/rd/etc/rivolution/broadcast.json`).
- `rivapi/dashboard/handlers_broadcast.go` (new): `Broadcast` (GET
  `/broadcast`) and `BroadcastSave` (POST `/broadcast/save`). Save
  flow: write JSON → generate icecast.xml → install → generate
  radio.liq → restart icecast2 → restart liquidsoap (exit code 5
  treated as warning, not error). Result banner embedded in page.
- `rivapi/dashboard/templates/broadcast.html` (new): three
  collapsible sections — Station, Icecast, Liquidsoap — plus a
  dynamic stream list managed by Alpine.js. Add/remove streams; codec
  dropdown (MP3/HE-AAC v1/HE-AAC v2/OGG Vorbis); per-stream metadata
  override expander. Streams serialized as JSON into a hidden field
  before submit.
- `rivapi/dashboard/templates/base.html`: added Broadcast nav link.
- `rivapi/main.go`: wired `GET /broadcast`, `POST /broadcast/save`.
- `conf/sudoers.d/rivapi`: added `RIVAPI_INSTALL` alias for the
  scoped `sudo install` command that writes `icecast.xml`.
- `conf/systemd/liquidsoap.service.d/rivolution.conf`: added
  `[Service]` section with `ExecStart` override pointing to
  `/home/rd/etc/liquidsoap/radio.liq`.

## 2026-07-01 (continued, 6)

- `docs/specs/0008-broadcast-tool-suite-integration.md`: added Phase 1
  implementation plan — `BroadcastConfig` data model (station defaults,
  per-stream overrides, Icecast and Liquidsoap config structs), Icecast
  XML and Liquidsoap `.liq` generators, Save & Deploy flow, `/broadcast`
  dashboard UI design (stream list with add/remove/codec/bitrate,
  per-stream metadata overrides), `fdkaac` bundled dependency decision,
  deployment topology note (source-server vs public streaming), and
  Verification section. Critical note updated to reference the spec 0010
  stack start-order solution.

## 2026-07-01 (continued, 5)

- `conf/systemd/rivendell.service.d/rivolution.conf`,
  `conf/systemd/icecast2.service.d/rivolution.conf`,
  `conf/systemd/liquidsoap.service.d/rivolution.conf`: added
  `PartOf=rivolution-stack.target` to each. `Wants=` in the target
  propagates start only; without `PartOf=` on the services, stopping
  or restarting the target had no effect on the running units.
  `stereo-tool.service` already had this; `tailscaled` intentionally
  excluded (stopping the broadcast stack should not drop VPN).

## 2026-07-01 (continued, 4)

- `rivapi/store/service_status.go`: `ControlUnit` now detects `systemctl`
  exit code 5 ("unit not loaded") and returns a friendly error naming the
  file to copy and the `daemon-reload` command needed, rather than a raw
  process error.

- `docs/specs/0010-systemd-stack-orchestration.md`: added Deployment
  section covering manual install steps for all `conf/` files (sudoers,
  systemd units, drop-ins, udev rule), a unified-installer placeholder,
  and a deb-package placeholder. Implementation deviations section
  updated.

- `BACKLOG.md`: added entry documenting that `conf/` deployment files
  have no automated install path — lists all files needing coverage and
  what both the Ansible role and the `postinst` deb hook need to do.

- `docs/handoff/2026-07-01.md` (gitignored): session handoff — full
  account of architecture decisions, what was built, what's working,
  what needs manual deployment, the installer gap, and open items for
  the next session.

## 2026-07-01 (continued, 3)

- `rivapi/dashboard/handlers.go`: fixed Stereo Tool GUI launch — was
  hardcoding `DISPLAY=:0` which misses xRDP sessions on `:10` or other
  display numbers. `launchEnv()` now uses the process's inherited DISPLAY
  if set, otherwise auto-detects from `/tmp/.X11-unix/` (picks the first
  available socket). Returns nil (no-op with error message) if no display
  is found. Added `"os"` import.

## 2026-07-01 (continued, 2)

- `rivapi/store/service_status.go`: renamed display label for
  `rivendell.service` from "Rivendell" to "Rivolution".

- `rivapi/dashboard/handlers.go`: `SystemAction` now always returns HTTP
  200 — control errors are embedded in the returned status fragment via
  `ActionError` rather than returning a 4xx that htmx silently ignores.
  Added 400ms settle delay after a successful action so systemd state is
  current before querying. Added `StereoToolLaunch` handler (`POST
  /system/stereo-tool/launch`): starts the binary with `DISPLAY=:0` in
  the background; returns a result fragment.

- `rivapi/dashboard/templates/system_status.html`: renders `ActionError`
  as a visible banner when set.

- `rivapi/dashboard/templates/system.html`: added "Launch GUI" button for
  Stereo Tool (`POST /system/stereo-tool/launch`).

- `rivapi/main.go`: wired `POST /system/stereo-tool/launch`.

## 2026-07-01 (continued)

- `rivapi/store/stereo_tool_install.go` (new): `StereoToolArch` (detects
  server architecture via `runtime.GOARCH`), `StereoToolDownloadURL`
  (constructs the correct Thimeo URL for the arch and optional pinned
  version — `jack_64` for amd64, `jack_pi2_64` for arm64),
  `StereoToolInstalled` (checks path exists), `InstallStereoTool`
  (downloads binary, atomic temp-file+rename install, 5-minute timeout).

- `rivapi/config/config.go`: added `StereoToolPath` / `RIVAPI_STEREO_TOOL_PATH`
  (default `/home/rd/bin/stereo_tool`; `rd`-owned path avoids privilege
  escalation for dashboard-driven installs).

- `rivapi/dashboard/handlers.go`: extracted `systemData()` helper (shared
  by `System`, `SystemAction`, and populated Stereo Tool fields).
  Added `StereoToolInstall` handler (`POST /system/stereo-tool/install`):
  reads optional `version` form param, calls `store.InstallStereoTool`,
  returns `stereo_tool_result.html` fragment.

- `rivapi/dashboard/templates/system.html`: added Stereo Tool binary section
  below the service status table — shows arch, install path, install status,
  "Install Latest" button, and "Install Version" form (version input +
  submit). htmx posts to `/system/stereo-tool/install` and swaps result
  into `#stereo-tool-result`.

- `rivapi/dashboard/templates/stereo_tool_result.html` (new): install
  outcome fragment (success message or error).

- `rivapi/main.go`: wired `POST /system/stereo-tool/install`.

- `conf/systemd/stereo-tool.service`: updated `ExecStart` path to
  `/home/rd/bin/stereo_tool` to match default `StereoToolPath`.

## 2026-07-01

- `rivapi/store/service_status.go` (new): `QueryStackStatus` polls
  `systemctl is-active` for each managed stack unit and returns current
  state; `ControlUnit` runs `sudo systemctl start/stop/restart` for
  allowed units only (allowlist-validated to prevent injection). Units
  not yet installed surface as `"unknown"` rather than erroring.

- `rivapi/dashboard/handlers.go`: `System` handler (`GET /system`) and
  `SystemAction` handler (`POST /system/service/{unit}/{action}`) added.
  Full page renders via `system.html`; htmx requests return
  `system_status.html` fragment. Action buttons use `hx-confirm` for
  category-3 (audio-path) services per spec 0010 live-playout protection.

- `rivapi/dashboard/templates/system.html` (new): System page — full-stack
  Start/Stop/Restart buttons at top; status table loaded via htmx on page
  load.

- `rivapi/dashboard/templates/system_status.html` (new): htmx-swappable
  service status table with per-unit Start/Stop/Restart buttons and
  state badges.

- `rivapi/dashboard/templates/base.html`: added System nav link.

- `rivapi/dashboard/static/app.css`: added `.service-state` badge styles
  for `active`, `inactive`, `failed`, `activating`, `unknown` states.

- `rivapi/main.go`: wired `/system` GET and `/system/service/{unit}/{action}`
  POST routes under `DashboardMiddleware`.

- `conf/sudoers.d/rivapi` (new): targeted NOPASSWD rule for `rd` to run
  `systemctl start/stop/restart` for the five stack units and the target.

- `conf/systemd/rivolution-stack.target` (new): stack target; `Wants=`
  `rivendell.service`, `icecast2.service`, `liquidsoap.service`,
  `stereo-tool.service`, `tailscaled.service`.

- `conf/systemd/rivendell.service.d/rivolution.conf` (new): drop-in setting
  `User=rd`, `Group=rd`, `LimitRTPRIO=99`, `LimitRTTIME=infinity`,
  `IOSchedulingClass=realtime`, `IOSchedulingPriority=0`, plus an
  `ExecStartPost` health probe on caed's TCP port (5005). Resolves the
  root→rd permission prerequisite for PipeWire integration (Phase 1 of
  two-phase caed migration; see spec 0010).

- `conf/systemd/icecast2.service.d/rivolution.conf` (new): adds
  `After=rivendell.service`.

- `conf/systemd/liquidsoap.service.d/rivolution.conf` (new): adds
  `After=rivendell.service` and `After=icecast2.service`.

- `conf/systemd/stereo-tool.service` (new): custom unit for Stereo Tool;
  `After=liquidsoap.service`; `User=rd`; `PartOf=rivolution-stack.target`.

- `conf/udev/99-ptp.rules` (new): assigns `/dev/ptpN` to `ptp` group
  (Phase 1.5 prerequisite for non-root PTP clock access).

## 2026-06-30 (continued, 4)

- `rivapi/auth/auth.go`: dashboard session cookie is now a browser
  session cookie (no `Expires`/`MaxAge`). Browser discards it on close,
  forcing re-login on next open. JWT expiry still enforces token
  lifetime within a session.

## 2026-06-30 (continued, 3)

- `rivapi/store/carts_db.go` (new): `CartDB` — native MariaDB cart reader
  for admin users, bypassing rdxport's USER\_PERMS filter. Queries the CART
  table directly; converts TYPE int (1/2) to "audio"/"macro" string,
  FORCED\_LENGTH/AVERAGE\_LENGTH milliseconds to "HH:MM:SS.T" format, and
  nullable YEAR date column to a year string, matching rdxport XML output.

- `rivapi/store/store.go`, `rivapi/store/groups_db.go`: exported `IsAdmin`
  (was private `isAdmin`) and added it to the `GroupStore` interface so
  dashboard handlers can route admin cart queries to `CartDB` without a
  type assertion.

- `rivapi/dashboard/handlers.go`: `Carts` and `CartDetail` handlers now
  check `GroupStore.IsAdmin` and use `CartDB` for admin users (direct DB
  query) and `CartProxy` for non-admin users (rdxport). Added error checks
  to all `ExecuteTemplate` calls — previously silent failures now return
  HTTP 500.

- `rivapi/main.go`: constructs `CartDB` and passes it to `dashboard.New`.

## 2026-06-30 (continued, 2)

- `rivapi/store/groups_db.go`: admin users (`ADMIN_CONFIG_PRIV='Y'` in
  USERS table) now see all groups — queries GROUPS table directly instead
  of filtering through USER_PERMS, matching RDAdmin's behaviour. Non-admin
  users continue to see only their permitted groups via USER_PERMS.

- `rivapi/config/config.go`: rivapi now reads `/etc/rd.conf` as its
  primary config source, matching the pattern used by all other Rivendell
  binaries. DB credentials come from the `[mySQL]` section (Hostname,
  Loginname, Password, Database keys). JWT secret comes from the new
  `[dashboard]` section (JwtSecret key). Env vars still override file
  values for dev/container use. Missing or unreadable rd.conf is silent —
  falls back to env vars and hardcoded defaults.

- `conf/rd.conf-sample`: added `[dashboard]` section with `JwtSecret=`
  (generated by first-run), plus optional `StationName`, `LogoURL`,
  `AccentColor` keys for per-station dashboard branding.

- `scripts/rivolution-first-run.sh`: generates a 64-character random JWT
  signing secret and writes it to rd.conf's `[dashboard] JwtSecret` entry,
  same pattern as the existing MySQL password generation step.

## 2026-06-30 (continued)

- Added `rivapi/dashboard/` — browser-facing HTML dashboard shell (spec
  0012). Implements: cookie-based browser session (`rivapi_session`
  HttpOnly cookie carrying the same JWT already used by the JSON API;
  `DashboardLoginHandler` sets it, `LogoutHandler` clears it);
  `DashboardMiddleware` redirects unauthenticated browser requests to
  `/login` instead of returning 401; `GET /login` + `POST /login` +
  `GET /logout` routes; `GET /`, `/groups`, `/carts`, `/carts/{number}`
  dashboard views reusing existing `GroupStore`/`CartStore` interfaces;
  htmx partial-render pattern (same route returns fragment on
  `HX-Request`, full page otherwise); server-rendered Go templates with
  Pico.css + htmx + Alpine.js all vendored in
  `rivapi/dashboard/static/vendor/` (no npm/Node dependency). Base
  layout includes user-switchable light/dark mode via Alpine.js
  `themeManager()` (persists preference to `localStorage`; follows
  system default on first load) and branding slots (station name, logo,
  accent colour, configurable via env vars). Network topology fully
  configurable via `RIVAPI_TRUST_PROXY_HEADERS` and
  `RIVAPI_COOKIE_SECURE`; TLS listener available via `RIVAPI_TLS_CERT` /
  `RIVAPI_TLS_KEY` for direct Tailscale cert use.

- Added `docs/specs/0012-dashboard-foundation.md` — session model, cookie
  auth design, network topology config, template layout and routing
  conventions, branding placeholders, CSRF deferral rationale.

- Added `docs/specs/0013-rdadmin-parity-roadmap.md` — classifies all ~48
  RDAdmin management sections into four implementation buckets (rdxport
  full CRUD, rdxport read-only, native Go+DB with established pattern,
  native Go+DB new surface) and establishes build order. Includes
  Rivolution-exclusive sections (Tailscale, Station Branding, systemd
  stack control) as a separate track.

- Added `docs/specs/0014-tailscale-integration.md` — three-piece design:
  installer Ansible role (`roles/tailscale/`), dashboard Network section
  with auth-key activation + status display, MagicDNS/TLS cert
  provisioning via `tailscale cert`. Documents the hard limit that
  enabling HTTPS Certs tailnet-wide requires the Tailscale admin console,
  not the local CLI.

## 2026-06-30

- Fixed three bugs found during rivapi Phase 1 end-to-end verification:
  (1) `config/config.go`: default `RIVAPI_RDXPORT_URL` corrected to
  `http://127.0.0.1/rd-bin/rdxport.cgi` — `localhost` resolves to `::1`
  on this host, which misses rdxport.cgi's IPv4 loopback auth bypass and
  causes a 403; (2) `store/groups_db.go`: `GROUPS` query wrapped in
  `COALESCE()` for all nullable columns — `COLOR` is NULL in a fresh
  install and a bare `string` scan returns an error; (3) `store/store.go`
  and `store/carts_proxy.go`: `ForcedLength`, `AverageLength`, and `Year`
  fields changed from `int` to `string` — rdxport serialises lengths as
  `"MM:SS.S"` time strings and year as an empty string when unset, both
  of which fail XML-to-int decoding.

- Added `rivapi/` — Go REST API foundation (spec 0005, Phase 1).
  Module path `github.com/anjeleno/rivolution/rivapi`, binary name `rivapi`.
  Implements: `POST /api/v1/auth/login` (rdxport ticket acquisition + JWT
  issuance); `GET /api/v1/groups` (native MariaDB, mirrors
  `web/rdxport/groups.cpp:ListGroups` + `lib/rdgroup.cpp:RDGroup::xml()`);
  `GET /api/v1/carts` and `GET /api/v1/carts/{number}` (rdxport proxy,
  commands 6/7). Internal `GroupStore`/`CartStore` interface boundary allows
  proxy and native-DB implementations to be swapped without HTTP handler
  changes. Builds clean on ARM64 (`go build ./...`).

- Renamed `debian/control`, `debian/control.src`, and `debian/control.src2`:
  source package and all binary packages renamed from `rivendell`/`rivendell-*`
  to `rivolution`/`rivolution-*`; Maintainer updated to Anjeleno
  `<la90046@gmail.com>`; Qt5 runtime dependencies replaced with Qt6
  equivalents (`libqt5sql5-mysql` → `libqt6sql6-mysql`; `qttranslations5-l10n`
  removed — bundled with Qt6; `qt5-style-plugins` removed — no Qt6 equivalent
  needed); `python3-mysqldb` → `python3-pymysql` (Ubuntu 26.04).

## 2026-06-28

- Parameterized `scripts/rivolution-first-run.sh`: `RIVENDELL_USER`
  (default `rd`) replaces a hardcoded username, and a new
  `RIVENDELL_SKIP_DB_SETUP` flag skips rd.conf creation and the
  database create/grant/schema steps entirely, for pointing a host at
  a database that already exists elsewhere instead of creating one
  locally. Both default to this script's original unparameterized
  behavior, so existing usage is unaffected.
- Confirmed Rivolution builds natively on arm64 with no changes
  required: verified end-to-end on Ubuntu 26.04 arm64 (UTM guest on
  Apple Silicon), same build time and steps as the existing x86_64
  verification. `debian/control` already declares `Architecture: any`
  for every binary package, and neither `configure.ac` nor
  `configure_build.sh` contain any architecture-conditional logic — the
  build system was already architecture-agnostic by construction, this
  just confirms it empirically on real arm64 hardware.

## 2026-06-27

- Worked around an intermittent JVM crash in the DocBook PDF build
  (`docs/rivwebcapi`, `docs/manpages`, `docs/opsguide`, `docs/dtds`,
  `docs/apis`): `fop` was randomly segfaulting inside JIT-compiled core
  JDK methods unrelated to the document being rendered, roughly 1 run
  in 8 on this OpenJDK build. Added `JAVA_TOOL_OPTIONS=
  "-XX:-TieredCompilation"` to each directory's `fop` invocation in
  `Makefile.am`, which skips the JIT tier where the crash originates;
  confirmed clean across multiple repeated runs with the flag where the
  same input reproducibly crashed without it. See
  [`BACKLOG.md`](https://github.com/anjeleno/rivolution/blob/main/BACKLOG.md)
  for the full root-cause writeup and removal condition.
- Fixed `autoreconf -fi` failing repo-wide with `required file
  './ChangeLog' not found`: automake's default GNU strictness requires
  a literal `ChangeLog` file at the repo root, which this fork doesn't
  have under that exact name (its changelog convention is
  `CHANGELOG.md`). Added `ChangeLog` as a symlink to `CHANGELOG.md`
  rather than a second, separately-maintained file — automake's check
  is existence-only and follows symlinks, so this satisfies it with no
  duplicate content to keep in sync.

## 2026-06-26

- Confirmed this fork's local checkout had been lagging behind its own
  already-established GitHub identity: the remote had been
  `anjeleno/rivolution` all along, not a separately named
  `rivendell-v6` repo, and every push this session had already been
  landing on its `main` branch. Cloned a fresh checkout at
  `~/dev/rivolution` to match (a plain rename was avoided since
  autotools/libtool can cache the old absolute source path in
  already-generated build files); the old `~/rivendell-v6` directory
  was kept in place, unmodified, as a build-verified fallback rather
  than deleted outright.
- Fixed `docs/apis`, `docs/manpages`, `docs/dtds`, and
  `docs/rivwebcapi`'s DocBook PDF/HTML build failing on a freshly
  configured tree: their `Makefile.am` rules referenced
  `$(DOCBOOK_STYLESHEETS)` directly at `make` time, but that variable
  only ever existed inside `configure_build.sh`'s own process — it
  never persists into a separate, later `make` invocation, even when
  chained with `&&`. Now reference `$(top_srcdir)/helpers/docbook`
  instead, the symlink `configure.ac` already creates once at
  configure time, removing the dependency on shell environment state
  entirely. Also moved the stylesheet-path detection itself into
  `configure.ac`, checking the filesystem for the real file rather than
  trusting a distro-name guess in `configure_build.sh` — works for any
  distro sharing the same `docbook-xsl-ns` package layout, and warns
  clearly at configure time if no candidate path is found instead of
  failing later with a cryptic FOP error. Also added `COPYING` to
  `.gitignore`: `automake --add-missing` regenerates a stock GPLv3
  template whenever it's absent, but this project uses GPLv2
  (`LICENSES/GPLv2.txt`) and deliberately removed `COPYING` in 2021 —
  the regenerated file was stray noise from rerunning `autogen.sh`,
  not a real project file.
- Fixed three more Qt6 signal renames that silently fail to connect
  at runtime without any compiler diagnostic, found by auditing every
  `SIGNAL(...)` call site in the tree against the actual installed
  Qt6 headers: `QComboBox::activated(const QString &)` (now
  `textActivated`), `QButtonGroup::buttonClicked(int)` (now
  `idClicked`), and `QAbstractSocket::error(QAbstractSocket::
  SocketError)` (now `errorOccurred`). The `QComboBox` one is why
  `RDLibrary`'s manual "Add Cart" dialog would reject a cart number as
  "outside of the permitted range for this group" after switching
  groups in the dropdown — the auto-fill logic that recalculates the
  next free cart number for the newly-selected group never re-ran.
  Fixed at all 46 occurrences across 32 files; see
  [`docs/specs/0006-qt6-migration.md`](https://github.com/anjeleno/rivolution/blob/main/docs/specs/0006-qt6-migration.md)
  for the full file list.
- Fixed `RDWaveFile::createWave()` clobbering `errno` via an unconditional
  `unlink()` of a nonexistent `.energy` sidecar file between the actual
  file open and the caller's check of whether it succeeded, masking the
  real reason a destination file failed to open.
- Documented `mp3gain` as a required runtime dependency in [`INSTALL.md`](https://github.com/anjeleno/rivolution/blob/main/INSTALL.md)
  and the golden-image package list — without it, an MP3-to-MP3 import
  or export that requests loudness normalization silently falls back to
  a full decode/re-encode instead of the bitstream-level gain-patch,
  with no error, just a much slower conversion. Only affects
  normalization on MP3-passthrough-eligible transfers; normalization on
  any other format pair was never affected.
- Fixed `RDImportAudio::Import()` (RDLibrary's manual Import dialog)
  never actually transmitting the user's selected output format to the
  server, and the Format control being unconditionally disabled in
  Import mode by original design. Added an explicit, opt-in "Override
  library default format" checkbox so manual imports can request MP3
  passthrough deliberately, consistent with the Dropbox/`rdimport`
  format-override controls, and reworked the dialog's layout so the
  checkbox and Format row sit in the actual Import-zone instead of
  overlapping the divider line meant for the Export section.
- Fixed RDLibrary's "Add" cart flow deleting a newly-imported cart's
  audio (both the database row and the file in `/var/snd`) whenever the
  Edit Cart dialog was closed any way other than the explicit OK button
  — now checks for already-persisted audio before allowing any rollback,
  regardless of how the dialog was closed.
- Updated [`INSTALL.md`](https://github.com/anjeleno/rivolution/blob/main/INSTALL.md)'s generic prerequisites list from "Qt5 Toolkit,
  v5.9 or better" to Qt6, listing the actual modules `configure.ac`
  requires and the verified-working version.
- Marked RHEL/CentOS/Fedora/Rocky support as deliberately abandoned,
  with CentOS losing upstream support being the deciding factor —
  the existing build-system code (`configure.ac`'s distro-detection
  branch, `rivendell.spec.in`, `conf/rivendell-rhel.pam`, the `make
  rpm` target) is left in place as a starting point for anyone who
  wants to pick it back up, just no longer tested or developed
  against. Removed a [`BACKLOG.md`](https://github.com/anjeleno/rivolution/blob/main/BACKLOG.md)
  entry about an unverified RHEL stylesheet path accordingly, and
  reframed the related [`KNOWN_ISSUES.md`](https://github.com/anjeleno/rivolution/blob/main/KNOWN_ISSUES.md) mention to match.
- Dropped a personal name from `RD_COPYRIGHT_NOTICE` (`lib/rd.h`,
  shown in RDAdmin's System Info dialog and every CLI tool's
  `--version` output) — only the original author's credit belongs
  there — and updated its year range to 2002–2026.

## 2026-06-24

- Fixed `ripcd` never processing any client's login handshake:
  `connect(...,SIGNAL(mapped(int)),...)` against a `QSignalMapper`
  silently fails to connect under Qt6, since the bare `mapped(int)`
  signal was disambiguated into `mappedInt`/`mappedString`/
  `mappedObject` and no longer exists on the class. This broke
  `ripcd`'s per-connection read routing specifically, which in turn
  caused `RDLibrary`'s group/category list to come up empty and
  `rdimport`'s dropbox-watch mode to never start scanning after
  launch — both depend on the same post-login signal chain. Fixed at
  all 52 occurrences across 32 files; see
  [`docs/specs/0006-qt6-migration.md`](https://github.com/anjeleno/rivolution/blob/main/docs/specs/0006-qt6-migration.md)
  for the full file list and why this evaded the original migration's
  build-clean verification.
- Replaced launcher and in-app icons for `rdadmin`, `rdairplay`,
  `rdcatch`, `rdlibrary`, `rdlogedit`, and `rdlogmanager` (PNG, `.ico`,
  and `.xpm` sets, plus the `RDIconEngine`-embedded window icon for
  each) — the previous set made several modules hard to tell apart at
  a glance. Also added dedicated icons for `rdalsaconfig` and
  `rddbconfig`, which previously had no icon of their own and silently
  reused the generic Rivendell icon and `rdadmin`'s icon respectively.
- Fixed `RDAudioImport::runImport()` and `RDAudioStore::runStore()`
  treating any HTTP 200 from `rdxport.cgi` as success regardless of
  what the response body actually contained. Both relied on
  `RDWebResult::readXml()`/`ParseInt()`, two ad-hoc line-scanning
  parsers that quietly returned success/zero on unrecognizable input
  instead of signaling a parse failure — so a dead or misconfigured CGI
  endpoint (Apache serving the binary itself instead of executing it,
  for instance) looked exactly like a successful import or a genuinely
  empty audio store. `readXml()` now requires a real `<RDWebResult>`
  root tag before extracting fields, and both callers now treat a
  parse failure as a real error instead of defaulting to success. This
  bug predates this fork; found while diagnosing a dropbox import that
  reported success and deleted its source file despite never actually
  storing any audio.

## 2026-06-23

- Published this fork's first public landing page at
  [rivolution.dev](https://rivolution.dev/).
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
  [`docs/specs/0006-qt6-migration.md`](https://github.com/anjeleno/rivolution/blob/main/docs/specs/0006-qt6-migration.md).
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
- Fixed: a fresh database connection (RDDB Config, `rdadmin`, the
  `panel_copy`/`rdcatch_copy` importers) failed with `QSqlDatabase: can
  not load requested driver`. The actual Qt6 MySQL/MariaDB driver
  plugin names are `QMYSQL`/`QMARIADB`; `lib/rd.h`'s
  `DEFAULT_MYSQL_DRIVER`, `conf/rd.conf-sample`'s `Driver=` line,
  `docs/manpages/rd.conf.xml`, and both importers all still referenced
  the legacy Qt3-era name `QMYSQL3`, which doesn't exist as a loadable
  driver under Qt6 (or, in fact, modern Qt5 — this was already stale,
  just never hit until now). All five corrected to `QMYSQL`.

## 2026-06-22

- Established this fork's public identity as Rivolution: the GitHub
  repository was created under that name
  ([`anjeleno/rivolution`](https://github.com/anjeleno/rivolution)),
  distinct from "rivendell-v6," the working name used locally and in
  conversation for some time afterward.
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
  [`docs/specs/0006-qt6-migration.md`](https://github.com/anjeleno/rivolution/blob/main/docs/specs/0006-qt6-migration.md) and
  [`docs/specs/0009-qtwebengine-migration.md`](https://github.com/anjeleno/rivolution/blob/main/docs/specs/0009-qtwebengine-migration.md).
- Fixed: MP3 gain-patch normalization (added 2026-06-21) silently never
  applied any gain shift. The requested level was read as hundredths of
  a dB, but every consumer of this setting elsewhere in the pipeline
  (`RDAudioConvert`'s own normalization, and `rdimport`'s own conversion
  before sending it over the wire) has always used plain whole dB —
  e.g. a Dropbox configured for -13dBFS was read as -0.13dB, rounding
  the computed gain-patch step to zero. `mp3gain` still ran and rewrote
  some header bytes, so the import completed normally with no error,
  just a still-unnormalized file. See
  [`docs/specs/0004-mp3-gain-patch.md`](https://github.com/anjeleno/rivolution/blob/main/docs/specs/0004-mp3-gain-patch.md).
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
  and arm64). See [`docs/specs/0004-mp3-gain-patch.md`](https://github.com/anjeleno/rivolution/blob/main/docs/specs/0004-mp3-gain-patch.md).
- Fixed: MP3 passthrough (import) ignored a Dropbox's configured
  normalization/autotrim level whenever the source was already MP3 and
  the target format was also MP3 — the only acknowledgment was a syslog
  warning, never actually applied. Normalization/autotrim now requires
  falling through to the full decode/process/re-encode path, since
  neither is possible on a byte-for-byte passthrough copy. See
  [`docs/specs/0003-mp3-waveform-energy.md`](https://github.com/anjeleno/rivolution/blob/main/docs/specs/0003-mp3-waveform-energy.md).
- Fixed two more bugs in the new MP3 waveform/peak energy feature, found
  during pre-build review: peaks computed during MP3 import/encoding
  could be undercounted (a signed-value comparison ignored negative-going
  excursions), and a same-format passthrough import could persist a
  permanently-empty peak chunk, leaving that cut's waveform blank
  forever with no recovery. See [`docs/specs/0003-mp3-waveform-energy.md`](https://github.com/anjeleno/rivolution/blob/main/docs/specs/0003-mp3-waveform-energy.md).
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
  [`docs/specs/0003-mp3-waveform-energy.md`](https://github.com/anjeleno/rivolution/blob/main/docs/specs/0003-mp3-waveform-energy.md).

## 2026-06-18

- Fixed: MP3 passthrough (import and export) could produce a file
  whose real sample rate doesn't match the system's output rate,
  which `caed`'s MPEG playback path doesn't resample — audible as
  pitch/speed-shifted ("helium") playback. Passthrough now requires
  the source's real sample rate to match the system rate; otherwise it
  falls through to the existing, correct conversion path. See
  [`docs/specs/0001-mp3-import-format.md`](https://github.com/anjeleno/rivolution/blob/main/docs/specs/0001-mp3-import-format.md).

## 2026-06-17

- Added segue back-timing: when the outgoing element in a segue has
  "No fade on segue out" checked, the next element's start is now
  delayed (when needed) so its intro lands exactly when the outgoing
  element's tail finishes, instead of firing instantly at the segue
  marker regardless of how much lead-in the next element has. No
  effect when "No fade on segue out" is unchecked. See
  [`docs/specs/0002-segue-backtiming.md`](https://github.com/anjeleno/rivolution/blob/main/docs/specs/0002-segue-backtiming.md).
- Added selectable MP3 (MPEG Layer III) as an import coding format,
  alongside the existing PCM16/PCM24/MPEG Layer II options: a new
  `--audio-format=<0|1|2|3>` flag on `rdimport`, a matching override on
  the web import service (`rdxport.cgi`), a per-Dropbox "Target Audio
  Format" setting in RDAdmin, and an MP3 entry in the host-level default
  format dropdown. See [`docs/specs/0001-mp3-import-format.md`](https://github.com/anjeleno/rivolution/blob/main/docs/specs/0001-mp3-import-format.md).
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
