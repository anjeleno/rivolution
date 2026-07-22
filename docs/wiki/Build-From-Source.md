# Building Rivolution From Source

Builds a real `.deb` package from a git checkout, then installs it the
same way as [[Deb Package Install|Deb-Package-Install]] — this is the
only from-source path that gets `debian/postinst`'s full automation
(systemd units, PipeWire/JACK wiring, VLC routing, the database seed,
and everything else that page's steps rely on). A plain
`./configure && make && sudo make install` never runs `postinst` at
all — it's a completely separate mechanism from packaging, so nothing
covered there ever reaches a raw autotools build. Use this page
instead, even for a one-off local build.

Use this page if you need to build from an unreleased branch, a
private fork, or a target with no published release asset yet (Debian
Trixie, currently). If you just want to run Rivolution and don't need
to change anything, download the release `.deb` instead — see
[[Deb Package Install|Deb-Package-Install]].

---

## 0. Before you start

This page assumes you've already done [[Start Here|Start-Here]]'s
OS/desktop setup (updates, hostname, timezone, the `rd` user, a
desktop, xRDP if this is a cloud box) — it isn't repeated here.

---

## 1. Install packaging + build dependencies

```bash
sudo apt install -y git g++ automake autoconf autoconf-archive libtool \
  libltdl-dev make debhelper dpkg-dev golang-go \
  qt6-base-dev qt6-base-dev-tools qt6-l10n-tools qt6-webengine-dev \
  qt6-webengine-dev-tools libqt6sql6-mysql \
  libexpat1-dev libid3-dev libcurl4-gnutls-dev libcoverart-dev \
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
  apache2 mariadb-server mariadb-client mp3gain
```

`golang-go` builds `rivapi` (the Go-based dashboard/API) — packaged
into the `.deb` right alongside the C++ binaries, so it needs to be
present at build time even though nothing at runtime needs Go
installed.

Don't forget to secure MariaDB and set a root password:

```bash
sudo mariadb-secure-installation
```

Pointing this Rivolution host at a database on a *different* box
instead of running one locally? Drop `mariadb-server` — you only need
the client to build.

---

## 2. Switch to normal user: rd, and clone the repository

```bash
su rd
```

```bash
cd ~ && git clone https://github.com/anjeleno/rivolution.git && cd rivolution
```

---

## 3. Build the package

```bash
scripts/rebuild-deb.sh
```

This bumps the Debian revision, cleans any stray build artifacts from
a previous run, regenerates `debian/control`/`debian/rules`/
`debian/changelog` from their `.src` templates, and runs
`dpkg-buildpackage` — producing `rivolution_<version>_<arch>.deb` (plus
several sub-package `.deb`s) one directory above the checkout.

A few flags worth knowing:

- **`--no-bump`** — build whatever revision is already committed,
  instead of minting a new one. Use this for a from-tag rebuild, not a
  new local change.
- **`--version=X.Y.Z`** — set a new upstream version (e.g. crossing
  from `6.0.0~beta1` to `6.0.0~rc1`) instead of bumping the Debian
  revision. Resets the revision to `1`.

The script leaves the version bump uncommitted — review and `git
commit` it yourself once you're happy with the build.

---

## 4. Install it

```bash
cd ..
```

```bash
sudo apt install ./rivolution_*.deb
```

From here, everything is identical to a downloaded release —
continue with [[Deb Package Install|Deb-Package-Install]] starting at
its "Verify it installed" step: setting the audio driver in
RDAlsaConfig, Program Source/Save & Deploy, and VLC routing.

---

## Rebuilding after a source change

```bash
scripts/rebuild-deb.sh --no-bump
```

then repeat step 4. `--no-bump` here is deliberate for local
iteration — pass no flags at all once you're ready to commit a real
new revision.

---

## Troubleshooting

**`dpkg-checkbuilddeps: error: unmet build dependencies`** — re-check
step 1; the exact missing package name is in the error text.

**`cannot open shared object file` after installing** — run `sudo
ldconfig`. This shouldn't be needed for a `.deb` install (`postinst`
runs it automatically), so seeing this means something didn't install
cleanly — check `dpkg -l rivolution` shows the package as fully
configured (`ii`, not `iU` or similar).

**RDAdmin/RDAirplay icons in the applications menu do nothing when
clicked** — the menu shortcut swallows any crash output. Launch the
binary directly from a terminal instead to see the actual error.
