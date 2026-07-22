# The Web Dashboard (`rivapi`)

Rivolution ships with `rivapi`, a new Go-based web dashboard for day-to-day orchestration of service control, streaming configs, PipeWire/JACK patches, tasks (nightly SQL backups, log generation and support for custom scripts), Standalone/Server/Client mode switching, processing, and config backup / restore. Use the features you want, skip the ones you don't. No terminal required.

It's installed and running automatically as part of a normal package install (`rivapi.service`, managed alongside the rest of the broadcast stack — see [[Deb Package Install|Deb-Package-Install]]).

> [!TIP]
> Everything here is optional to use — Rivolution runs fine with none
> of it touched, the same way stock Rivendell always has. Use the
> pages that help, skip the ones that don't.

---

## Logging in

Reachable at `http://<this-box's-address>:8080` once installed.

There's exactly one account: username **`admin`**, password is the
`JwtSecret` value in `/etc/rd.conf`'s `[dashboard]` section — a random
64-character string generated automatically on a fresh install (see
that value with `grep '^JwtSecret=' /etc/rd.conf`). This is
deliberately independent of both Rivendell's own user database and any
Linux system account — no PAM, no shadow-group membership needed. The
actual password is everything *after* `JwtSecret=` on that line.

---

## System

Live status for every service in the broadcast stack — `rivendell`,
`mariadb`, `apache2`, `pipewire-system`, `wireplumber-system`,
`stereo-tool`, `rivapi` itself, and more — each with start/stop/restart
controls. Updates automatically without a page reload.

> [!WARNING]
> Restart Stack **will** interrupt audio, but gives you a popup warning before executing.

---

## Streaming

Configures the actual broadcast chain: Icecast connection details
(host/port/mount/passwords), the JACK client this feeds from, sample
rate, and log path. Clicking **Save & Deploy** writes the config and
restarts the underlying `ffmpeg`-based stream services — this is the
step that actually takes effect; editing the fields alone doesn't
change anything live until deployed.

If Stereo Tool's own routing target can't resolve (e.g. it drifted out
of sync with what's actually live), this page surfaces an actionable
banner naming the real, detected JACK client and a one-click "Use this
and redeploy" button, instead of just a warning that describes the
problem.

---

## Patchbay

The JACK/PipeWire connection matrix — choose and save connections between
any two clients in the audio graph (`rivendell_0`, Stereo Tool, `ffmpeg`
stream encoders, VLC, real hardware). Once at least one connection is
saved here, this becomes authoritative over the whole graph: anything
connected that isn't in the saved set gets automatically disconnected
within about 30 seconds, and anything saved that isn't currently
connected gets automatically reconnected — connections don't survive
either endpoint restarting on their own, so this self-heals that
continuously rather than requiring a manual redraw after every reboot.

**Program Source** here is what actually feeds Stereo Tool (or your
streams directly, if you're not using Stereo Tool).

### Set Program Source, then Save & Deploy once

*(Also documented on [[Deb Package Install|Deb-Package-Install]]'s
step 3 — kept here too so this page is self-contained; update both if
this step ever changes.)*

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

---

## Mode

Switches the station between three network topologies:

- **standalone** — everything local: database, audio store, desktop.
- **server** — standalone, plus the database and audio store exposed
  to other Rivolution hosts over NFS.
- **client** — only the Rivolution application itself, pointed at a
  remote MySQL/MariaDB host and a remote NFS-mounted audio store.

Applying a mode change shows a real step-by-step log as it happens, not
just success/failure, so you can see exactly how far it got if
something stops partway through.

---

## Tasks

Scheduled jobs, each with its own interval (hourly/daily/weekly/monthly,
or a raw systemd calendar expression for anything more specific):

- **Database backup** — a `mysqldump` of the Rivendell database on a
  schedule.
- **Log generation** — `rdlogmanager -g`, generating a new log for a
  service some configurable number of days ahead.
- **Merge traffic log** — `rdlogmanager -t`, merging an imported
  traffic log into an existing one.
- **Log reconciliation** — `rdlogmanager -r Reconcile`, fixing log
  discrepancies for a service.
- **Custom command** — runs any script or command you point it at
  (`/home/rd/apps/my-script.sh`, or anything else on the box), on the
  same scheduling mechanism as the built-in task types.

Each task's last-run status is shown right on the list — no need to dig
through `journalctl` to check whether last night's backup actually ran.

---

## Backup

Exports every dashboard-configured setting — Streaming, Patchbay, Mode,
Tasks — plus Stereo Tool's own state (`~/.stereo_tool.rc` and any saved
presets, which live outside this project's own config entirely) into
one importable JSON file. Useful for migrating to new hardware or
disaster recovery.

> [!WARNING]
> An exported file contains real secrets — Icecast passwords, and a
> remote MySQL password if Mode is set to client. Treat it like any
> other credentials file, not something to share casually.

---

## Processing

> [!NOTE]
> Stereo Tool itself is downloaded directly from
> [Thimeo](https://www.thimeo.com/)'s own public URL, never bundled
> with Rivolution.

> [!TIP]
> Depending on your specific needs, we recommend purchasing a Stereo
> Tool license for full features. Available directly from
> [Thimeo's own site](https://www.thimeo.com/).

---

> [!NOTE]
> Visiting Stereo Tool's web interface Processing Page 
> (`http://<this-box's-address>:8079`) from a machine whose IP isn't in
> that whitelist box, you'll get an "Access denied — not whitelisted"
> page instead of the interface — it shows the exact IP to add.

### Enabling Stereo Tool's own web interface

Needed for `/broadcast`'s Stereo Tool integration to actually reach
it — a one-time, per-machine setup done through its own GUI, not the
dashboard:

1. On the System page, under the Stereo Tool binary, tap **Launch GUI**.
2. In the Stereo Tool GUI, go to **Configure → Configure**.
3. Scroll down to **Web Interface** and tap the power icon to enable it.
4. Change the port to **8079**.
5. If there's already an IP in the whitelist box, add a space after it,
   then enter your own IP to whitelist yourself too.

---

### Setting up Stereo Tool I/O so PipeWire/JACK patches can route audio

1. With the Stereo Tool GUI open, tap the speaker-icon **I/O** button
   at the top.
2. Then tap **Audio I/O**.
3. Set the sample rate to match Rivolution: **48 kHz**.
4. Under **Input**, choose **ALSA / jack (ALSA)**.
5. Under **Output**, choose **ALSA / jack (ALSA)**.

> [!CAUTION]
> Make sure you close the Stereo Tool GUI afterward. Otherwise you'll
> have two copies of Stereo Tool running at the same time, creating a
> processing loop.

---

## What's next

A couple of pieces are still early: a Tailscale integration page (the
Unified Installer already supports enabling Tailscale at install time —
see [[Unified Installer|Unified-Installer]] — but the dashboard's own
activation/status page isn't built yet), service control, broadcast tooling, and PipeWire came first. See [Roadmap](https://github.com/anjeleno/rivolution/blob/main/ROADMAP.md)
for what's planned next.