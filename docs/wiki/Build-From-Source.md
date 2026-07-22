# Building Rivolution From Source on Ubuntu 26.04 / 24.04

A chronological walkthrough of building and standing up Rivolution
on a virgin Ubuntu 26.04 box, based on a real first-time build — including the gaps that tripped it up and how they were resolved. Each step assumes the previous one finished cleanly. 

> [!TIP]
> Verified on a DigitalOcean Basic droplet (8 vCPU / 16 GB RAM / 320 GB disk) running Ubuntu 24.04 — full build in ~15 minutes, no changes needed to the steps below.

> [!TIP]
> Also verified on Ubuntu 26.04 arm64 (UTM guest on Apple Silicon) — same ~15 minute build, no changes needed to the steps below. Rivolution builds natively on either x86_64 or arm64; nothing in the build needs cross-compilation or an architecture-specific step.

---

## 0. Before you start: OS and desktop setup

This walk-through assumes you have already installed virgin Ubuntu
26.04 or 24.04 server, either on physical hardware, a local VM, or on a cloud VPS. Start by installing updates, setting your hostname and timezone.

```bash
apt update && apt dist-upgrade -y
```

> [!TIP]
> NOTE: System hostname and Rivolution hostname must match.

```bash
hostnamectl set-hostname [hostname]
```

```bash
sudo sed -i "s/^127\.0\.1\.1[[:space:]].*/127.0.1.1\t$(hostname)/" /etc/hosts
```

---

> [!TIP]
> This opens the debconf timezone chooser — an interactive menu to pick your region and city instead of setting the zone by name.

```bash
sudo dpkg-reconfigure tzdata
```
```bash
sudo timedatectl set-ntp yes
```
```bash
timedatectl
```

---

## Set a root password and create a normal user

```bash
passwd root
```

```bash
adduser rd
```
```bash
usermod -aG sudo rd
```

---

Then, you'll need to install a desktop. After extensive testing on physical hardware, local UTM and Cloud VPS installs, we recommend minimal MATE (no bloat). SSH into your machine and install as root with:

```bash
apt update && apt install -y --no-install-recommends ubuntu-mate-core mate-system-monitor
```

If this is a Cloud install, add xRDP for easy remote desktop access:

```bash
apt install -y xrdp dbus-x11
```

---

There's a bug in the current version of xRDP that causes the default
session with running GUI applications to crash and then become
orphaned — very annoying.

**The workaround:** disable the accelerated graphics check (this
doesn't disable accelerated graphics on a supported system, only the
check):

```bash
sudo mv /usr/libexec/mate-session-check-accelerated /usr/libexec/mate-session-check-accelerated.disabled
```
```bash
sudo mv /usr/libexec/mate-session-check-accelerated-gl-helper /usr/libexec/mate-session-check-accelerated-gl-helper.disabled
```
```bash
sudo mv /usr/libexec/mate-session-check-accelerated-gles-helper /usr/libexec/mate-session-check-accelerated-gles-helper.disabled
```

---

> [!WARNING]
> A separate, unrelated xRDP problem on a fresh Ubuntu 26.04 install
> specifically: When 26.04 defaults to a Wayland session, which xRDP can't
> drive at all — connecting over RDP immediately-crashing session, even
> with the accelerated-graphics workaround above already applied.
> Confirmedon a real fresh 26.04 install.

**The workaround:** force an X11 (not Wayland) MATE session for xRDP
specifically, without changing the console/physical-display session at
all.

```bash
cat << 'EOF' > ~/.xsession
unset DBUS_SESSION_BUS_ADDRESS
unset SESSION_MANAGER
export XDG_SESSION_TYPE=x11
mate-session
EOF
chmod +x ~/.xsession
```

```bash
sudo apt install -y xorgxrdp xserver-xorg-core
```

`/etc/xrdp/startwm.sh` ends with a default `test -x /etc/X11/Xsession
&& exec /etc/X11/Xsession` line — since `exec` replaces the running
process outright, that line has to actually be removed, not just
followed by something else, or it wins before your own session command
is ever reached:

```bash
sudo cp /etc/xrdp/startwm.sh /etc/xrdp/startwm.sh.bak
```

```bash
sudo sed -i '/test -x \/etc\/X11\/Xsession/d;/exec \/bin\/sh \/etc\/X11\/Xsession/d' /etc/xrdp/startwm.sh
```

```bash
sudo tee -a /etc/xrdp/startwm.sh << 'EOF'

unset DBUS_SESSION_BUS_ADDRESS
unset XDG_RUNTIME_DIR
exec mate-session
EOF
```

```bash
echo "allowed_users=anybody" | sudo tee /etc/X11/Xwrapper.config
```

```bash
sudo systemctl restart xrdp
```

---

## 1. Install build dependencies

Nothing here gets installed for you later — install everything below
*before* touching the source tree:

```bash
sudo apt install -y git g++ automake autoconf autoconf-archive libtool \
  libltdl-dev make debhelper \
  qt6-base-dev qt6-base-dev-tools qt6-l10n-tools qt6-webengine-dev \
  qt6-webengine-dev-tools libqt6sql6-mysql \
  libexpat1-dev libid3-3.8.3-dev libcurl4-gnutls-dev libcoverart-dev \
  libdiscid-dev libmusicbrainz5-dev libcdparanoia-dev libsndfile1-dev \
  libpam0g-dev libvorbis-dev libsamplerate0-dev libsoundtouch-dev \
  libsystemd-dev libjack-jackd2-dev libasound2-dev libflac-dev \
  libflac++-dev libmp3lame-dev libmad0-dev libtwolame-dev libssl-dev \
  libtag1-dev libmagick++-dev \
  alsa-utils alsa-tools alsa-tools-gui alsa-firmware-loaders \
  libasound2-plugins \
  docbook5-xml docbook-xsl-ns xsltproc fop libxml2-utils \
  python3 python3-pycurl python3-pymysql python3-serial python3-requests \
  python3-venv python3-virtualenv python3-build twine \
  apache2 mariadb-server mariadb-client mp3gain gedit
```

---

Don't forget to secure MariaDB and set a root password:

```bash
sudo mariadb-secure-installation
```

This list is verified directly against a real working build. Pointing
this Rivendell host at a database on a *different* box instead of
running one locally? Drop `mariadb-server` — you only need the client
to build and connect.

---

## 2. Switch to normal user: rd and Clone the repository

```bash
su rd
```
```bash
cd ~ ; git clone https://github.com/anjeleno/rivolution.git ; cd rivolution
```

---

## 3. Configure, build, and install

```bash
./configure_build.sh
```
```bash
make -j$(nproc)
```
```bash
sudo make install
```
```bash
sudo ldconfig
```

A few things going on in those four lines that aren't obvious from
the outside:

- **Use `configure_build.sh`, not `./configure` directly.**
  `configure_build.sh` auto-detects your distro and calls `./configure`
  with the right flags, including `--prefix=/usr`. Run `./configure`
  yourself instead and you'll end up building against a non-standard
  prefix with its own gotchas — see [Known Issues](https://github.com/anjeleno/rivolution/blob/main/KNOWN_ISSUES.md).
- **`sudo ldconfig` at the end is required, not optional.** Without
  it, every Rivendell binary fails immediately with `error while
  loading shared libraries: librd-*.so: cannot open shared object
  file: No such file or directory`, even though the file genuinely
  exists — `make install` doesn't refresh the linker's cache itself.
  See [Known Issues](https://github.com/anjeleno/rivolution/blob/main/KNOWN_ISSUES.md) for why.

---

## 4. Enable CGI processing for the web import service

Every audio import — whether from `rdimport`, a dropbox, or RDLibrary's
own Import dialog — actually goes over HTTP to `rdxport.cgi`, not
straight to disk. Without this step, Apache has no handler registered
for `.cgi` files and just serves the compiled binary itself as a raw
file download; the request still comes back `HTTP 200`, so the import
silently reports success — including deleting the source file in
dropbox mode — while nothing was actually imported. RDLibrary's disk
free-space gauge hits the same dead endpoint and gets stuck showing
`0h 00m`.

```bash
sudo ln -sf ../mods-available/cgid.conf /etc/apache2/mods-enabled/cgid.conf
```
```bash
sudo ln -sf ../mods-available/cgid.load /etc/apache2/mods-enabled/cgid.load
```
```bash
sudo systemctl restart apache2
```

---

## 5. Set up and launch Rivendell

One script handles all of it in a single pass — system users, `/var/snd`
permissions, the database, and first launch — including generating a
real random database password for you (the sample config ships with a
fixed, public default, `hackme`, that this script never uses):

```bash
sudo bash ~/rivolution/scripts/rivolution-first-run.sh
```

Specifically, it: creates the `rivendell`/`pypad` system users and
groups; creates and sets ownership/permissions on `/var/snd`; copies
`conf/rd.conf-sample` to `/etc/rd.conf` and writes the generated
password into its `[mySQL]` section; creates the database, schema, and
test-tone audio (`rddbmgr --create --generate-audio`); disables
PulseAudio so `caed`/ALSA get uncontended access to the sound device;
and starts (and enables at boot) the `rivendell` service.

---

## 6. Broadcast stack, PipeWire/JACK bridge, and the `rivapi` dashboard

The steps above build and run Rivendell's own C++ core. This step
covers the separate runtime layer around it — Icecast, ffmpeg (one
process per configured broadcast stream, encoding and pushing to
Icecast), Stereo Tool, PipeWire, and the Go-based `rivapi`
dashboard/API — none of which link into the Rivendell binaries
themselves. Verified working end-to-end on a real Ubuntu 26.04
install.

**Go toolchain (for `rivapi`):**

```bash
sudo apt install golang-go
```

**Broadcast stack packages:**

```bash
sudo apt install icecast2 ffmpeg fdkaac vlc vlc-plugin-jack
```

VLC is used for ad-hoc live audio capture into Rivolution — not a
systemd-managed service, launched manually as needed, but required to
be present.

**PipeWire/JACK bridge:**

```bash
sudo apt install pipewire-jack
```

> [!IMPORTANT]
> One extra step is required, not optional. `libjack-jackd2-dev` (in
> the build dependencies, step 1) pulls in a real `libjack.so.0` that
> conflicts with the `pipewire-jack` shim — `ldconfig` resolves it
> alphabetically by `/etc/ld.so.conf.d/*.conf` filename, and the
> standard architecture conf sorts first, so every JACK client
> silently links against the real library instead of the shim, with
> no error — they just never reach PipeWire.

```bash
sudo mv /etc/ld.so.conf.d/pipewire-jack-$(uname -m)-linux-gnu.conf /etc/ld.so.conf.d/00-pipewire-jack-$(uname -m)-linux-gnu.conf
```
```bash
sudo ldconfig
```

Verify the shim now wins — the `pipewire-0.3/jack/libjack.so.0` line
must appear first:

```bash
ldconfig -p | grep "libjack.so.0 "
```

**Stereo Tool's ALSA-JACK bridge:** Stereo Tool reaches JACK only
through an ALSA plugin, not as a native JACK client, and its stock
config hardcodes hardware port names that don't exist under
system-scope PipeWire:

```bash
cp ~/rivolution/conf/alsa/rd.asoundrc ~/.asoundrc
```

> [!NOTE]
> Stereo Tool's web UI (port 8079 by default) shows an "Access
> denied... not whitelisted" page the first time you visit it. This is
> Stereo Tool's own upstream default, not anything this project
> configures — open its GUI directly on the machine it's running on
> (not over the network) and whitelist your IP from its own settings.
> The access-denied page itself shows the exact IP to add. Only needed
> once per machine.

**`rivapi` and `conf/` deployment:** see [`docs/specs/0010-systemd-stack-orchestration.md`](https://github.com/anjeleno/rivolution/blob/main/docs/specs/0010-systemd-stack-orchestration.md)'s
Deployment section in the main repo for the full, current file-by-file
list (it changes as new units are added, so it isn't duplicated here).
In short:

```bash
cd ~/rivolution/rivapi && go build -o rivapi .
```
```bash
sudo install -m 755 ~/rivolution/rivapi/rivapi /usr/local/bin/rivapi
```
```bash
sudo cp ~/rivolution/conf/systemd/rivapi.service /etc/systemd/system/
```
```bash
sudo cp ~/rivolution/conf/sudoers.d/rivapi /etc/sudoers.d/rivapi && sudo chmod 440 /etc/sudoers.d/rivapi
```
```bash
sudo systemctl daemon-reload && sudo systemctl enable --now rivapi.service
```

`scripts/rivapi-rebuild.sh` automates the build+install+restart
sequence for subsequent source changes.

---

## Quick reference commands

To easily restart the Rivendell service, run:

```bash
systemctl restart rivendell
```

> [!TIP]
> If this is a VM/Cloud install running xRDP remote desktop, run the following command to fix Qt/XCB errors for root-run Rivendell tools under xRDP

```bash
sudo ln -s /home/rd/.Xauthority /root/.Xauthority
```

Launch `RDAdmin` and `RDAirplay` from the Rivolution applications menu.

## Troubleshooting

**`QSqlDatabase: can not load requested driver` (empty or `QMYSQL3`)**
— either `/etc/rd.conf` doesn't exist yet (step 5), or you're on an
older checkout that still has the stale `QMYSQL3` driver name (fixed
in source as of this writing — Qt6's actual driver names are
`QMYSQL`/`QMARIADB`, not the legacy Qt3-era `QMYSQL3`). Pull latest and
rebuild if you still see this.

**`cannot open shared object file`** — see step 3; run `sudo ldconfig`.

**Import reports success and deletes the source file, but the cart
shows up red with no audio, and RDLibrary's disk-space gauge is stuck
at `0h 00m`** — see step 4; `mod_cgid` isn't enabled, so Apache serves
`rdxport.cgi`'s raw binary instead of executing it. Verify with
`apache2ctl -M | grep cgi` — if that prints nothing, the module isn't
loaded.

**RDAdmin/RDAirplay icons in the applications menu do nothing when
clicked** — the menu shortcut swallows any crash output. Launch the
binary directly from a terminal instead to see the actual error.
