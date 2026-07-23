# Installing Rivolution From the `.deb` Package

For installing a pre-built release .deb package instead of building from
source. See [[Build From Source|Build-From-Source]] if you want to
build your own `.deb` from a checkout instead — everything from "Verify
it installed" onward on this page applies there too, once it's
installed. Either way, [[Start Here|Start-Here]]'s OS/desktop setup
(OS/hostname setup, creating the `rd` user, installing a desktop, xRDP
if this is a cloud box) is a prerequisite not repeated on this page.

> [!TIP]
> This walk-through assumes you have already installed virgin Ubuntu 26.04 or 24.04 server, either on physical hardware, a local VM, or on a cloud VPS.

## 1. Download and install the package

Check the [Releases page](https://github.com/anjeleno/rivolution/releases)
for the current version tag — the commands below use `v6.0.0-rc1-5` as
a real, concrete example; substitute whatever tag is current.

```bash
wget https://github.com/anjeleno/rivolution/releases/download/v6.0.0-rc1-5/rivolution_6.0.0.rc1-5_amd64.deb
```

```bash
sudo apt install ./rivolution_6.0.0.rc1-5_amd64.deb
```

> [!NOTE]
> The downloaded filename uses a plain dot (`6.0.0.rc1-5`), not the
> `~` the package's own internal version string uses (`6.0.0~rc1-5`) —
> GitHub Releases silently substitutes `.` for `~` in uploaded asset
> filenames. This is cosmetic only; `apt`/`dpkg` read the real version
> from the package's own control data, not the filename.

Verify it installed:

```bash
dpkg -l rivolution
```

The `rivapi` dashboard is reachable at `http://<this-box's-address>:8080`
once the install finishes — that's where the remaining steps happen.
See [[Web Dashboard|Web-Dashboard]] for a full walkthrough of every
page, including how to log in.

> [!TIP]
> RDAdmin's Help → System Information dialog shows the full installed
> version, including the Debian revision (e.g. `6.0.0~rc1-5`, not just
> `6.0.0~rc1`) — useful for confirming exactly which build is on a box
> without dropping to a terminal.

---

## 2. Set the audio driver to JACK — required, every install

Launch **RDAlsaConfig** — Applications menu → Rivolution → Configuration
→ RDAlsaConfig.

RDAlsaConfig's device list now has an explicit **PipeWire/JACK** entry
at the top, alongside any real ALSA device it finds — select it the
same way you'd select any other device in the list, no hidden
convention involved.

- **On a VM or cloud box with no audio hardware** (the common case —
  confirmed on a real DigitalOcean droplet): only **PipeWire/JACK**
  will be listed. Select it and click **Save**.
- **On a box that does list a real device** (physical hardware, or a
  hypervisor-exposed one like "Intel HDA" under some VM platforms):
  select **PipeWire/JACK** to use this fork's JACK/PipeWire routing —
  the driver path this fork's dashboard, Stereo Tool integration, and
  VLC routing (below) all depend on. Selecting **PipeWire/JACK**
  automatically deselects any selected real device, and vice versa —
  they're enforced mutually exclusive live in the dialog itself, not
  just by convention.

**Verify:** open RDAdmin → the Audio Devices/Ports editor for your
station. Card 0's **Card Driver** should read **PipeWire/JACK**.

> [!IMPORTANT]
> On an xRDP virtual desktop specifically (not a physical desktop),
> RDAlsaConfig needs to run as root and would otherwise fail to open a
> window at all with an X11/xcb authorization error — as of this
> release, `postinst` symlinks root's `.Xauthority` to `rd`'s own
> automatically on every install, so this no longer needs a manual
> step. If you're on an older install predating this fix, run
> `sudo ln -s /home/rd/.Xauthority /root/.Xauthority` once yourself.

---

## 3. Set Program Source, then Save & Deploy once

Rivolution's audio routing has two separate controls that both need to
be set, in order, the first time:

- **`/patchbay`**'s **Program Source** field — what feeds Stereo Tool
  (or every stream directly, if you're not using Stereo Tool).
- **`/broadcast`**'s **Save & Deploy** button — the action that
  actually applies your current Program Source to Stereo Tool's live
  audio routing.

1. Go to `/patchbay` and set **Program Source** to whatever actually
   feeds your streams (typically Stereo Tool, if you're using it).
2. Go to `/broadcast` and click **Save & Deploy** — even if nothing
   else on that page changed. This step is what actually applies the
   routing.

**Verify:** back on `/patchbay`, the Connections list should show real,
**Saved** links — both `rivendell_0` output ports connected to Stereo
Tool's input, and both Stereo Tool output ports connected to your
stream(s).

> [!TIP]
> If this drifts out of sync later — Stereo Tool's routing target
> changes underneath it, or its ALSA-JACK bridge's Device ID ever gets
> reset — the dashboard self-heals it automatically within 30 seconds,
> and surfaces an actionable banner with a one-click fix on `/broadcast`
> if it detects a target that genuinely can't resolve on its own. You
> shouldn't need to repeat steps 1–2 above after the first time unless
> you're deliberately changing Program Source.

---

## 4. Routing VLC into Rivolution's input (optional)

VLC (Applications menu → Sound & Video → VLC media player) is already
wired to feed Rivolution's system-scope JACK graph directly — this is
for remote broadcast via an Icecast relay, replacing a rack of outboard
RPU/codec gear: a remote encoder (e.g. BUTT) streams to an Icecast
source (like an AzuraCast relay in the middle), and VLC at the studio end plays that stream with its output patched persistently into Rivendell's input. A normal desktop-launched VLC would otherwise land in a completely separate, per-session audio graph that never reaches Rivolution at all; this install's VLC shortcut already points at a wrapper that avoids that, and a default connection into Rivolution's first input bus is seeded automatically the first time the dashboard starts.

Nothing to configure for the default case — open VLC, play a stream,
and check `/patchbay` for the live connection. If you want it routed
somewhere other than the first input bus, just redraw the connection
on `/patchbay` and save — your own choice is remembered and won't be
overwritten by the automatic default.

---

## Troubleshooting

**Stereo Tool's own Log window shows "Error opening (2 ch, srate
48000): I/O error" repeating every second, for both Input and Normal
output** — confirm step 2's RDAdmin check (Card 0 driver = PipeWire/JACK)
first, then step 3's `/patchbay` Connections check.

**RDAlsaConfig's device list only shows PipeWire/JACK, no real ALSA
device** — expected on any box with no real audio hardware (every bare
cloud VM). Not an error; select PipeWire/JACK, per step 2 above.

**Stereo Tool shows up as an Input under `/patchbay` Connections but
never as an Output** — same root cause as the Log window error above;
its output client can't fully register until its ALSA-JACK bridge
actually has somewhere valid to connect to (step 3).
