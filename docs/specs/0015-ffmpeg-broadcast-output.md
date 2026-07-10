# Spec 0015 — ffmpeg broadcast output and Stereo Tool JACK routing

## Problem

The original broadcast signal chain was caed → Stereo Tool → Liquidsoap
→ Icecast, with Liquidsoap's `input.jack()` binding pulling audio from
Stereo Tool's processed output and `output.icecast()` calls pushing each
configured stream to Icecast. Liquidsoap's `input.jack` has a
reproducible upstream bug under `pipewire-jack` specifically (not real
jackd2): a busy-wait in its clock thread pegs a full CPU core, and under
sustained load the process hard-crashes (`Failure("jack_read")`), which
`systemd`'s `Restart=on-failure` silently papers over as a crash-loop
rather than a stable service. Liquidsoap's actual job in this chain was
always narrow — JACK capture, encode, push to Icecast, none of its
scripting/crossfade/playlist machinery in use — so it's replaced here by
one `ffmpeg` process per configured stream.

Replacing the downstream client surfaced a second, unrelated problem:
Stereo Tool's own JACK target was hardcoded to the literal name
`liquidsoap`, and once that stopped existing, Stereo Tool's own
auto-connect logic retried forever, leaking a fresh JACK client
registration every cycle without ever completing initialization. Fixing
that surfaced how Stereo Tool actually reaches JACK at all, which turned
out not to match this project's own prior documentation.

## Architecture

```
caed (rivendell_0, native JACK client)
  -> Stereo Tool (ALSA "jack (ALSA)" device, see "Stereo Tool's actual
     JACK mechanism" below)
  -> one designated broadcast stream's ffmpeg process (the "anchor" --
     see "Dynamic anchor selection" below)
  -> patchbay reconciler fans the same live output out to every other
     currently-configured stream
  -> each stream's own ffmpeg process pushes directly to Icecast
```

There is no intermediary process between Stereo Tool and the streams.
An earlier iteration of this design used a permanent PipeWire virtual
bus (a `libpipewire-module-loopback` instance) as Stereo Tool's fixed
target, on the theory that Stereo Tool needed something permanent and
stream-count-independent to connect to. Confirmed live this was itself
part of the problem: Stereo Tool's ALSA-JACK bridge connects reliably
to a genuine native JACK client (verified against a real ffmpeg stream
process, stable for 14+ continuous seconds with zero errors) but not
reliably to a loopback-module node (constant connection churn — a
fresh client registration roughly every second, matching the exact
symptom the dead `liquidsoap` target originally produced). The bus was
removed entirely; the design below has no PipeWire-specific
intermediary at all.

## Piece 1: ffmpeg replaces Liquidsoap

`rivapi/store/ffmpeg_generator.go` replaces `liquidsoap_generator.go`.
`BroadcastConfig`/`StreamConfig` (station identity, per-stream
codec/bitrate/mount) needed no changes — already fully encoder-agnostic.
Each configured stream becomes its own always-on systemd service
(`Type=simple`, `Restart=on-failure`, no timer — streams are continuous,
not periodic), reusing the scheduled-tasks feature's existing control
scripts (`install-unit.sh`/`remove-unit.sh`/`task-systemctl.sh`,
`rivapi/store/tasks_deploy.go`) rather than a new deployment mechanism.

Each stream's JACK client is named `<JackInputID>-<sanitized mount>`
(`streamJackClientID`) — one shared base name (the dashboard's "JACK
input ID" field, held over from the Liquidsoap-era config shape to
avoid a migration) plus the stream's own mount, since one client name
can't be shared across N independent ffmpeg processes.

`ffmpegPipeline` builds the command per codec:
- **mp3**: `ffmpeg -f jack -i <client> -c:a libmp3lame -b:a <rate>k ...
  -f mp3 icecast://...`
- **ogg**: same shape, `-c:a libvorbis -q:a <quality> ... -f ogg
  icecast://...`
- **he-aac-v1/v2**: a three-stage pipe — `ffmpeg -f jack -i <client> -f
  s16le ... - | fdkaac ... -f 2 -o - - | ffmpeg -f aac -i - -c:a copy
  ... -f adts icecast://...` — identical to the external-fdkaac pattern
  `radio.liq`'s own `%external` encoder already used in production;
  ffmpeg's own AAC encoder is worse (distro builds don't link
  libfdk-aac, a licensing restriction), so raw PCM is piped through the
  same `fdkaac` binary, just with ffmpeg producing and consuming the
  pipe instead of Liquidsoap.

**The trailing `-f mp3`/`-f ogg`/`-f adts` on each pipeline is required,
not cosmetic.** ffmpeg's `icecast://` output protocol has no filename
extension to infer a container format from; without an explicit `-f`,
every stream fails immediately with "Unable to choose an output format"
(confirmed live — this affected all three codec paths identically until
each got its own explicit flag).

`ffmpeg` was added to `debian/control.src`'s `Depends` (previously
absent entirely — the original install had no dependency declaring it
at all, only working because it happened to already be present from
earlier manual setup); `liquidsoap` was removed.

## Piece 2: Stereo Tool's actual JACK mechanism

Stereo Tool's config UI labels its audio device "jack (ALSA)". This
project's own prior documentation read that as evidence of a *native*
JACK client — wrong. Confirmed live (2026-07-10): the installed build
resolves that device through ALSA's own `type jack` PCM plugin
(`libasound_module_pcm_jack.so`, package `libasound2-plugins`), which
makes the real `libjack` calls internally. The `libjack.so.0` linkage
and native `jack_*` symbols visible via `ldd`/`strings` against the
Stereo Tool binary are a transitive dependency through that plugin, not
evidence Stereo Tool's own code calls `jack_connect()` directly — a
generically-worded static analysis (linked libraries, exported symbols)
is not proof of *which* layer of a dependency chain actually makes a
given call; only live behavioral testing settled this.

Practical implication: Stereo Tool's actual JACK target for its
"Normal output" device comes from `~/.asoundrc`'s `pcm.jack` block
(repo source `conf/alsa/rd.asoundrc`) — specifically `playback_ports`.
`~/.stereo_tool.rc`'s own `[Soundcard - Normal output]` `Jack ID 1`/
`Jack ID 2` fields (patched by `patchStereoToolJackIDs`,
`rivapi/store/stereo_tool_install.go`) are kept set as a harmless
secondary measure in case a future Stereo Tool build or device mode
reads them, but they were not what actually mattered for the installed
build. `[Soundcard - Input]`'s blank-field default (`rivendell_0:
playout_0L/R`) was never the problem — only `[Soundcard - Normal
output]`'s blank-field default (a Thimeo-hardcoded fallback of
`liquidsoap:in_0/in_1`) was.

## Piece 3: dynamic anchor selection and reconciler fan-out

Stereo Tool can only ever be configured with one fixed
`playback_ports` target — there is no way to hand its ALSA-JACK bridge
N target names for N streams the way a native multi-port JACK client
could simply be told to connect to multiple destinations. Rather than
hardcode a specific stream's mount as that fixed target (which would
break identically the moment that specific stream were ever renamed or
removed, on any installation, not just this one), the target is derived
purely from whatever an operator has actually configured:

- `primaryStreamMount` (`rivapi/store/ffmpeg_generator.go`) sorts
  `cfg.Streams` by mount and returns the first — deterministic, and
  re-evaluated on every deploy, so it tracks whatever's actually
  configured rather than a value fixed at some earlier point in time.
- `syncStereoToolTarget` patches `~/.asoundrc`'s `playback_ports` to
  that stream's ffmpeg JACK client (`PatchAsoundrcTarget`,
  `rivapi/store/stereo_tool_install.go`) and restarts
  `stereo-tool.service` — but only when the target actually changed
  (a fresh install, or the previous anchor stream was removed), not on
  every deploy, since Stereo Tool's own already-established connection
  is otherwise left alone.
- `BroadcastConfig.ProgramSource` gained a sentinel value,
  `ProgramSourceStereoTool` ("stereo_tool"), alongside its existing
  plain-JACK-client-name behavior (still used as-is by a station with
  no local processing at all — e.g. `rivendell_0` directly). The
  `/broadcast` dropdown offers "Stereo Tool (local processing)" as a
  fixed extra option alongside the live-discovered client list
  (`store.ListOutputClients`, `rivapi/store/patchbay.go`).
- `syncStreamPatchLinks` fans Stereo Tool's *live* output out to every
  configured stream, anchor included. Because Stereo Tool's own JACK
  client name embeds its process ID (a fresh one every restart), it
  can never be matched by the literal `client:port` prefix matching
  every other kind of `ProgramSource` uses — `stereoToolOutputPorts`
  pattern-matches instead (`^stereo_tool\.P\.\d+\.\d+:`), reusing the
  same PID-agnostic identity `patchbay.go`'s `normalizePortName` already
  established for exactly this reason.

  The anchor stream's own link is included in this fan-out, not
  skipped as a "redundant" duplicate of Stereo Tool's own direct
  connection — an earlier iteration skipped it on that reasoning and
  broke live: `ReconcileLinks`' removal half treats *any* live
  connection not present in the saved set as unwanted and tears it
  down, so omitting the anchor's link from the saved set actively
  undid `syncStereoToolTarget`'s own connection every 30-second
  reconcile cycle instead of just harmlessly duplicating it.

## Piece 4: reconciler self-healing across a Stereo Tool restart

`rivapi/store/patchbay.go`'s `ReconcileLinks` already normalized port
names (`normalizePortName`) when deciding whether a saved link was
"already satisfied" — but the actual `Link()`/`Unlink()` calls used the
literal saved name regardless, so a saved link naming a now-stale
Stereo Tool PID could never actually reconnect: the comparison would
correctly recognize the intent was unmet, then try to connect a port
that no longer exists, forever, until the next full `/broadcast`
deploy rewrote the saved set with a fresh PID.

`resolvePortName` closes this gap: before calling `Link()`/`Unlink()`
for an unsatisfied saved link, it checks whether the saved name is
still live; if not, it looks for a currently-live port with the same
*normalized* identity and substitutes that instead. Confirmed live:
after restarting Stereo Tool independently of any dashboard deploy
(picking up a brand new PID), the very next reconcile cycle correctly
re-established every fan-out connection using the new PID, with
`patchbay.json` still holding the old one on disk.

## Piece 5: real-time scheduling

PipeWire ships its own real-time scheduling grant via a PAM-based
mechanism (`/etc/security/limits.d/*-pipewire.conf`, gating
`rtprio`/`nice`/`memlock` to members of the `@pipewire` group). This
does not apply to a plain systemd service started via `User=`/`Group=`
directives — PAM only fires for an actual PAM-authenticated session
(an interactive login, or a unit with `PAMName=` set explicitly, which
none of this project's audio services use). Confirmed live: `rd`'s own
interactive shell and every audio-stack systemd service both showed
`LimitRTPRIO=0` regardless of `rd`'s group membership.

`pipewire-system.service`, `wireplumber-system.service`, and
`stereo-tool.service` (`conf/systemd/`) each gained `LimitRTPRIO=95`,
`LimitMEMLOCK=256M`, `Nice=-19` directly in their `[Service]` sections
— the same ceiling PipeWire's own PAM grant would have provided, set
where it actually takes effect. `LimitMEMLOCK` was initially set to
`infinity` (matching the PAM grant file's own unbounded-looking
intent) and later tightened to a concrete `256M`: real-time audio
buffer locking needs tens of megabytes, not an unbounded ceiling, and
`infinity` across three separate services is an unnecessary risk on a
memory-constrained host with no corresponding benefit.

This does not fully resolve Stereo Tool's own connection stability on
its own — it was tested and ruled out as a complete explanation before
the actual `~/.asoundrc`/loopback-bus root causes above were found —
but it is independently correct configuration for a real-time audio
pipeline regardless, and stays.

## What's still open

Phase 2 of the broader PipeWire migration (caed, Stereo Tool, and every
stream as fully native PipeWire clients, no ALSA bridge or per-app
auto-connect guessing anywhere) remains the real long-term fix and is
still unscoped — see `BACKLOG.md`'s entry on this same signal chain.
