# Unified Installer

An Ansible playbook that provisions a fresh Ubuntu 24.04/26.04 machine
(x64 or arm64) into a working Rivolution install end to end:
OS/desktop/xRDP setup, audio-hardware detection, installing Rivolution
itself, and network topology (standalone/server/client). Lives in its
own repo,
[`rivolution-unified-installer`](https://github.com/anjeleno/rivolution-unified-installer),
separate from the application repo so the two can move at different
paces.

By default this installs the project's own released `.deb` package —
downloaded straight from GitHub Releases, the same artifact everyone
else installs. From there, `debian/postinst` (maintained in
[the main repo](https://github.com/anjeleno/rivolution)) does the real
provisioning: system users, `/var/snd`, the database, every
broadcast-stack systemd unit, sudoers, udev, `.asoundrc`, Apache
wiring, and more. This installer's job is getting a `.deb` onto the box
and telling `apt` to install it, plus the handful of things that
genuinely can't be a package's job (OS/desktop setup, audio-hardware
detection, network topology, Tailscale).

An opt-in `rivolution_install_method: source` builds a local `.deb`
from a git checkout instead of downloading one — useful for a private
fork, an unreleased branch, or a target with no published release asset
yet (Debian Trixie, currently). Either way, `postinst` runs the exact
same way from an identical `apt install` step.

> [!TIP]
> **Prefer filling in a form over hand-editing a script?**
> [rivolution.dev/configure.html](https://rivolution.dev/configure.html)
> builds the paste-in startup script below for you from a set of
> fields, live, entirely in your own browser — no network requests,
> nothing you type (including any password/key field) ever leaves the
> page.

---

## Quick start

`./configure.sh` is the interactive front end. It asks once for install
method (deb/source), install mode, install user (and, optionally, its
login password), Tailscale, and security hardening, then either pushes
over SSH to a separate host you give it, runs directly against the box
you're already logged into (no SSH needed — this is what you want if
you SSH'd into the target yourself and are running `configure.sh` on it
directly), or writes a fully filled-in script for you to paste into a
cloud provider's startup-script field for a box that doesn't exist yet.
The target itself never has to answer a prompt — by the time anything
runs unattended, every answer is already baked in.

> [!TIP]
> If you choose the "run directly on this box" method and
> `./configure.sh` isn't already running as root, it re-execs itself
> under `sudo` right at that point — you'll be prompted for your
> password there, same as running any other command with `sudo`,
> without having to remember to start the script with `sudo` yourself.
> If Ansible itself isn't installed yet, this method also installs it
> automatically once it's root — safe to do unprompted, since the
> target is already guaranteed to be Ubuntu/Debian. The SSH method
> does neither of these, since it runs on whatever your control
> machine happens to be.

```bash
git clone https://github.com/anjeleno/rivolution-unified-installer.git
```

```bash
cd rivolution-unified-installer && ./configure.sh
```

To skip the interactive prompts entirely (e.g. a non-interactive local
source-method install), pass the install method directly:

```bash
./configure.sh --method=local --install-method=source
```

If you'd rather skip the prompts, all three usage methods below also
work by hand.

---

## Method 1: control node pushes to a target over SSH

Requires Ansible already installed on this machine
(`sudo apt update && sudo apt install -y ansible`). For a Droplet, UTM
VM, or physical box that's already SSH-reachable as root (or any
sudo-capable user), run this **from a separate machine**:

1. Add the target to `inventory/hosts.ini`.
2. Install the required collections:

```bash
ansible-galaxy install -r requirements.yml
```

3. Run the playbook:

```bash
ansible-playbook site.yml
```

---

## Method 2: run directly on the target, no SSH

The manual, by-hand equivalent of choosing "local" in `./configure.sh`
above — not deprecated by it, just the non-interactive version.
Requires Ansible already installed on this box
(`sudo apt update && sudo apt install -y ansible`) — installed
system-wide via `apt`, not via `pip install --user`, since `sudo`
resets `PATH` and won't see a user-local install. If you're already
logged into the box, run this **on that same box**, as root or a user
with passwordless `sudo` (prefix both commands below with `sudo` if
you're not already root):

1. Install the required collections:

```bash
ansible-galaxy install -r requirements.yml
```

2. Run the playbook directly against this machine:

```bash
ansible-playbook -i "localhost," -c local site.yml
```

Add `-e rivolution_install_method=source` (or any other variable
override) to skip the `deb` default without going through
`configure.sh` at all:

```bash
ansible-playbook -i "localhost," -c local site.yml -e rivolution_install_method=source
```

---

## Method 3: paste into a Droplet's startup script (no SSH needed)

`bootstrap.sh` is meant to be pasted directly into DigitalOcean's
Droplet creation screen (Additional Options → Startup scripts (Free)),
or run as-is on a freshly installed UTM VM / physical box. It installs
Ansible and uses `ansible-pull` to fetch the repo and run `site.yml`
against the local machine — no inbound SSH or separate control node
required. Edit the variables at the top of
[`bootstrap.sh`](https://github.com/anjeleno/rivolution-unified-installer/blob/main/bootstrap.sh)
first, then paste the whole script in.

By default this builds the public `anjeleno/rivolution` repo on
`main` — no access or key needed to use the defaults as-is.

---

## Install method: `deb` (default) or `source`

- **`deb`** (default) — downloads the matching release asset from
  [GitHub Releases](https://github.com/anjeleno/rivolution/releases)
  for this host's architecture (and, for amd64, its Ubuntu version —
  26.04 gets the primary build, 24.04 gets the `-noble` build).
  `rivolution_release_tag` (blank = latest published release) can pin
  an exact version instead.
- **`source`** — clones `rivolution_git_repo`/`rivolution_git_ref`
  (defaults to the public repo's `main` branch) and builds a real local
  `.deb` from it, then installs *that* through the identical step the
  `deb` method uses. Use this for a private fork, an unreleased branch,
  or a target with no published release asset — currently, that means
  Debian Trixie.

---

## Install modes

`rivolution_install_mode` (default `standalone`) picks one of three
shapes, applied by driving the dashboard's own `/mode` page over HTTP
once Rivolution is installed and its dashboard is up:

- **standalone** — everything local: database, audio store, desktop.
- **server** — standalone, plus the database and audio store exposed
  to other Rivolution hosts over NFS.
- **client** — only the Rivolution application itself, pointed at a
  remote MySQL/MariaDB host and a remote NFS-mounted audio store
  instead of provisioning either locally.

---

## Setting the install user's password

Optional, and only ever applied at account-creation time — re-running
this playbook against a box that already has the account never resets
a password you've since changed by hand. Leave blank to leave the
account exactly as it was before (locked, login only via SSH key or an
already-authenticated `sudo` session). `configure.sh` prompts for it
securely; `bootstrap.sh` has a matching `RIVOLUTION_USER_PASSWORD`
variable.

---

## Tailscale

`rivolution_tailscale_enabled` (default `false`) installs the
`tailscale` package and enables (but does not start) `tailscaled`, plus
a scoped sudoers grant for the install user to run `tailscale
up`/`cert`/`status`. Leave `rivolution_tailscale_authkey_path` blank to
activate it yourself later (`sudo tailscale up`), or point it at a file
containing a real auth key to activate immediately during
provisioning. The key must be a real Tailscale **auth key** (always
`tskey-auth-<letters/numbers>-<letters/numbers>`, generated from the
admin console's Settings → Keys page) — not the full `tailscale up
--auth-key=...` command, and not an OAuth client secret.

---

## Security hardening

`rivolution_harden_security` (default `false`), independent of
everything else above — installs `ufw` (allowing Icecast's port, SSH,
and optional external-IP/LAN-subnet allowances), then disables SSH
password authentication, but only if an `authorized_keys` file already
exists for the install user. If one doesn't exist yet, SSH hardening is
skipped with a warning instead of risking a lockout. Deliberately does
**not** open the dashboard (`rivapi`, port 8080) or Stereo Tool's web UI
(port 8079) to the public — Tailscale, above, is the intended way to
reach those remotely.

---

## What's intentionally not automated

- Phase 0 — creating the Droplet, or installing a base OS on a UTM VM
  or physical box. This playbook starts from "fresh Ubuntu/Debian,
  reachable as root," not before.
- Disk imaging/cloning a literal golden image — this playbook is the
  replacement for that workflow, not an addition to it. Run it fresh on
  each target instead of cloning a disk image.
- Per-station configuration inside Rivolution itself (Dropboxes, carts,
  schedule codes, RDAdmin host settings, broadcast streams on
  `/broadcast`) — this gets you to a running station with a test tone
  in the library, not a configured one.
- The one remaining manual browser step `postinst` itself calls out at
  the end of every install: opening the dashboard's `/patchbay` page
  and connecting/saving the `caed` → Stereo Tool → stream audio chain.
  Deliberately left as a real operator action, not scripted around.

---

## Re-running this playbook later

Most of this is safe to re-run — it'll just confirm the existing state
and move on (`apt` itself is the idempotency mechanism for actually
installing Rivolution, both for a downloaded release and a locally
built one). The `source` method's local build is guarded by its own
marker file instead: once provisioned, the clone/build steps are
skipped entirely on every later run, even if the checkout itself was
deleted or reset in the meantime. If the checkout still exists with
uncommitted local changes when a rebuild is forced, the build role
refuses to touch it rather than silently discarding them.

See the
[repo's own README](https://github.com/anjeleno/rivolution-unified-installer/blob/main/README.md)
for full detail on every variable and the private-repo/deploy-key path.
