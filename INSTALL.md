# Installing Rivolution

This is the installation file for the Rivolution package.

## HARDWARE REQUIREMENTS
A graphical display capable of at least 1680x1050 pixels resolution.
(1920x1080 or higher recommended).


## MANDATORY PREREQUISITES
You will need the following installed and configured properly on your 
system before building Rivolution:

Apache Web Server
Included with most distros, or available from: http://www.apache.org/.

Expat
A stream-oriented XML parser library. Available at https://libexpat.github.io/.

Icedax
A command-line utility for querying and extracting data from audio CDs.
Included with most distros.

ID3Lib
An audio metadata tag library. Included in most distros, or available at 
http://id3lib.sourceforge.net/.

ImageMagick v6 (Magick++ C++ Language Interface)
A incredibly versatile library/utility for manipulating all sorts of graphical
and visual data. Available at https://imagemagick.org/script/index.php.

LibCurl, v7.19.0 or later
A client-side URL transfer library. Included with most distros, or
available at: http://curl.haxx.se/libcurl/.

LibCoverArt, v1.0.0 or later
A library for accessing the MusicBrainz Cover Art Archive.
Available at https://musicbrainz.org/

LibDiscId, v0.6.2 or later
A library for reading the attributes of audio CDs.
Available at https://musicbrainz.org/

LibMusicBrainz, v5.0.1 or later
A library for accessing the MusicBrainz open music encyclopedia
Available at https://musicbrainz.org/

LibParanoia
A library for ripping audio CDs. Included in most distributions, but also 
available from http://www.xiph.org/paranoia/.

LibSndFile
An audio file support library, written by Erik de Castro Lopo. Included with
most distros, or you can find it at http://www.mega-nerd.com/libsndfile/.

MySQL/MariaDB Database Server
Included in most Linux distributions. See http://www.mysql.com/.

PAM Pluggable Authentication Modules
A suite of shared libraries that enable the local system administrator to 
choose how applications authenticate users. Included with virtually all modern
distros, or see http://www.kernel.org/pub/linux/libs/pam/.

OggVorbis - Open Source Audio Coding Library. Needed for OggVorbis
importing and exporting. Included with most distros, or available at: 
http://www.xiph.org/.

Python, v3.6 or later
Open source scripting language. Included with most distros, or available at:
https://www.python.org/.

PycURL, v7.43.0 or later
Python interface to libcurl. Available at http://pycurl.io/.

PyMySQL, v1.3.12 or later
Python library for accessing MySQL databases. Available at
https://github.com/PyMySQL/mysqlclient-python.

PySerial, v 3.2.1 or later
Python library for accessing serial devices. Available at
https://pythonhosted.org/pyserial/.

Requests, v2.12.5 or later
HTTP transfer library for Python. Available at 
http://docs.python-requests.org/.

Qt6 Toolkit
Most modern Linux distros include this. `configure.ac` requires the
`Qt6Core`, `Qt6Widgets`, `Qt6Gui`, `Qt6Network`, `Qt6Sql`, `Qt6Xml`, and
`Qt6WebEngineWidgets` modules; no specific minimum version is enforced
by the build itself, but this fork is verified working against Qt6
6.10.2 (see the Ubuntu 26.04 section below for the actual package
names). It can also be downloaded directly at http://www.qt.io/.

Secret Rabbit Code
A sample-rate converter library, written by Erik de Castro Lopo. Included
with most distros, or you can find it at http://www.mega-nerd.com/SRC/.

SoundTouch Audio Processing Library
A library for altering the pitch and/or tempo of digital audio data.
Available at http://www.surina.net/soundtouch/.

Systemd System and Service Manager
Most modern Linux distros include this.

TagLib Audio Meta-Data Library, v1.8 or better
A high-quality C++ library for reading and writing a variety of audio metadata
formats. Available at https://taglib.org/.

X11 Window System
Virtually all Linux distros should include this.

---

## OPTIONAL PREREQUISITES
The following components are optional, but needed at build- and run- time in
order for particular features to work:

One or more audio driver libraries. Choices are:

  AudioScience HPI Driver - v3.00 or greater.
  For supporting AudioScience's line of high-end professional audio adapters.
  See http://www.audioscience.com/.

  The JACK Audio Connection Kit
  A low latency audio server, designed from the ground up for
  professional audio work. See http://jackaudio.org/.

  The Advanced Linux Sound Architecture (ALSA) v1.0 or greater.
  The standard soundcard driver for Linux for kernels 2.6.x or later.
  See http://www.alsa-project.org/.

Free Lossless Audio Codec (FLAC), v1.2.x or greater
A "lossless" audio encoding library. Included with most distros, or 
available from: http://flac.sourceforge.net/.

FAAD2 / mp4v2 - AAC/MP4 Decoding Libraries. Needed for MP4 file importation.
Available at http://www.audiocoding.com/faad2.html and
https://code.google.com/p/mp4v2/ respectively.

LAME - MPEG Layer 3 Encoder Library. Needed for MPEG Layer 3 exporting.
Available at http://lame.sourceforge.net/.

MAD - MPEG Audio Decoder Library. Needed for MPEG importing and playout.
Available at http://www.underbit.com/products/mad/.

TwoLAME - MPEG Layer 2 Encoder Library. Needed for MPEG Layer 2 exporting and
capture. Available at http://www.twolame.org/.

---

## DOCUMENTATION

The larger pieces of the Rivolution documentation are written in XML-DocBook5.
The following tools are required to build them:

XML-DocBook5 Stylesheets. Available at 
http://sourceforge.net/projects/docbook/. You will also need to create a
$DOCBOOK_STYLESHEETS variable in your environment that points to the top
of the stylesheet tree. More information can be found at
http://www.docbook.org/tdg5/en/html/appa.html#s.stylesheetinstall. On
RHEL-ish systems, they are also available in the 'docbook5-style-xsl'
package.

xsltproc. Command line XSLT processor. Available at
http://xmlsoft.org/XSLT/xsltproc2.html

Apache FOP. Formatting Objects (FO) processor. Available at
https://xmlgraphics.apache.org/fop/.

For a list of the required set of development packages for various popular
distros, see the 'DISTRO-SPECIFIC NOTES' section, below.

---

## INSTALLATION

There are three major steps to getting a Rivolution system up and
running. They are:

1.  Setting up pre-requisite software

2.  Installing the Rivolution package

3.  Initial configuration

---

### 1. Setting Up Prerequisites

The major prerequisite piece of software needed for a functioning
Rivolution system is the MySQL database engine. This needs to
be accessible from the target system (either by running on the local
host, or on a remote system) before Rivolution installation proper
is commenced. In practice, this means that the 'mysqld' daemon is
running and can be connected to using the mysql(1) client. You will
also need a login name/password for an account on the server with
administrative rights.

The process of configuring mySQL on a given host can be intricate and
is generally beyond the scope of this document. Details can be found
in a number of books on the subject, as well as in the very extensive
documentation that accompanies the server itself.

---

### 2. Installing the Rivolution Package

Once the prerequisites are set up, installation is most often a matter of 
cd'ing to the top of the Rivolution source tree and typing
'./configure_build.sh', 'make', followed by 'sudo make install'. The
'configure_build.sh' will attempt to determine which distribution is
running and automatically invoke the './configure' script with the
appropriate arguments. Should 'configure_build.sh' fail to recognize
the distro environment, './configure' can be run directly. Do
'./configure --help' for a list of the available arguments. This script
will auto-detect what sound drivers (HPI, JACK or ALSA) are available and
enable build support accordingly. To override this behavior, it's possible
to specify '--disable-hpi', '--disable-jack' or '--disable-alsa' as an
argument to './configure'. Be sure to see the important additional
information regarding configuration in the 'docs/JACK.txt' or 'docs/ALSA.txt'
files if you plan on using those sound driver architectures.

The installation of Rivolution's web services components are controlled
by two parameters passed to 'configure', as follows:

--libexecdir     Location to install web scripts and static content

--sysconfdir     Location to install Apache configuration

The specific values to pass will vary widely depending upon the specific
distro in question. For some specific examples for various popular distros,
see the 'DISTRO-SPECIFIC NOTES' section below.

After doing 'make install', be sure to restart the Apache web service.

---

### 3. Initial Configuration

Next, you'll need to install a small configuration file at
'/etc/rd.conf'. A sample can be found in 'conf/rd.conf-sample'. Much
of this can be used unchanged, with the exception of the entries in the 
[Identity] section. These should be changed to reflect the user and group 
name of the system accounts that will be running Rivolution.

The directory for the audio sample data next needs to be created, as
so:

	mkdir /var/snd

This directory should owned, readable, writable and searchable by the user 
and group specified in the 'AudioOwner=' and 'AudioGroup=' entires in
'/etc/rd.conf' and readable and searchable by Others (mode 0775).

Next, create an empty database on the MySQL/MariaDb server, as well as a
DB user to access it. This user should have the following privileges:

       Select
       Insert
       Update
       Delete
       Create
       Drop
       References
       Index
       Alter
       Create Temporary Table
       Lock Tables

In the '[mySQL]' section of the '/etc/rd.conf' file, set the 'Database=',
'Loginname=' and 'Password=' parameters to the DB name, user and password
that you created. Then, create an initial Rivolution database and generate
the audio for the test-tone cart in the audio store audio cart by doing:

       rddbmgr --create --generate-audio

If all goes well, this command should return with no output.

Finally, start up the Rivolution service by doing (as root):

       systemctl start rivendell

You should now be able to run the various Rivolution components from the
Applications menu.

---

## DISTRO-SPECIFIC NOTES

### Ubuntu 26.04 LTS

This list is verified directly against a real working build on this
distro (not carried forward from older, Qt5-era notes above) -- some
package names changed across the Qt5-to-Qt6 migration, and a few packages
present in the 24.04 distro (`libid3-dev`, `hpklinux-dev`) don't
exist at all on 26.04 under those names.

#### Required dependencies

```bash
sudo apt install git g++ automake autoconf autoconf-archive libtool \
  libltdl-dev make debhelper \
  qt6-base-dev qt6-base-dev-tools qt6-l10n-tools qt6-webengine-dev \
  qt6-webengine-dev-tools libqt6sql6-mysql \
  libexpat1-dev libid3-3.8.3-dev libcurl4-gnutls-dev libcoverart-dev \
  libdiscid-dev libmusicbrainz5-dev libcdparanoia-dev libsndfile1-dev \
  libpam0g-dev libvorbis-dev libsamplerate0-dev libsoundtouch-dev \
  libsystemd-dev libjack-jackd2-dev libasound2-dev libflac-dev \
  libflac++-dev libmp3lame-dev libmad0-dev libtwolame-dev libssl-dev \
  libtag1-dev libmagick++-dev \
  docbook5-xml docbook-xsl-ns xsltproc fop libxml2-utils \
  python3 python3-pycurl python3-pymysql python3-serial python3-requests \
  python3-venv python3-virtualenv python3-build twine \
  apache2 mariadb-server mariadb-client mp3gain gedit
```

`mp3gain` is a runtime dependency, not a build-time one: it's never
linked against, only invoked via `QProcess` at import time
(`lib/rdmpeggainpatch.cpp`) to apply MP3 loudness normalization
directly to the compressed bitstream, without decoding/re-encoding.
Without it, a normalized MP3 import still works, but silently falls
back to a full decode/re-encode instead of the fast bitstream-level
passthrough — correct output, much slower, and easy to mistake for an
unrelated bug (see [`docs/specs/0004-mp3-gain-patch.md`](https://github.com/anjeleno/rivolution/blob/main/docs/specs/0004-mp3-gain-patch.md)).

#### Run the following command, which detects the distro and applies the same script invocation below automatically

```bash
./configure_build.sh
```

#### Or to configure the script invocation manually, run:

```bash
./configure --prefix=/usr --libdir=/usr/lib --libexecdir=/var/www/rd-bin --sysconfdir=/etc/apache2/conf-enabled --enable-rdxport-debug MUSICBRAINZ_LIBS="-ldiscid -lmusicbrainz5cc -lcoverartcc"
```

#### Environmental variables

```bash
DOCBOOK_STYLESHEETS=/usr/share/xml/docbook/stylesheet/docbook-xsl-ns
```

`DEBUILD_MAKE_ARGS` only matters if you're building a real `.deb`
package via `debuild`/`dpkg-buildpackage` (it's passed straight
through as `make $(DEBUILD_MAKE_ARGS)` in `debian/rules.src`) — it has
no effect on a normal `./configure_build.sh && make` build, and you
can skip it entirely for that. If you are building a `.deb`, real
values that work:

```bash
DEBUILD_MAKE_ARGS=-j8
```
```bash
DEBUILD_MAKE_ARGS=V=1
```
```bash
DEBUILD_MAKE_ARGS=CXXFLAGS="-O0 -g"
```

`-jN` controls parallel build jobs. `V=1` shows full compiler command
lines instead of automake's default terse `CXX file.o` output.
`CXXFLAGS=`/`CFLAGS=` override the default `-g -O2` optimization/debug
level — safe to use here specifically, since required flags like
`-std=c++17` live in a separate `AM_CPPFLAGS` variable
(`lib/Makefile.am`) that a `CXXFLAGS` override doesn't touch.

#### Apache Web Server configuration: CGI processing must be enabled — run the following commands

```bash
sudo ln -sf ../mods-available/cgid.conf /etc/apache2/mods-enabled/cgid.conf
```
```bash
sudo ln -sf ../mods-available/cgid.load /etc/apache2/mods-enabled/cgid.load
```
```bash
sudo systemctl restart apache2
```

#### Build with

```bash
make -j$(nproc)
```

#### Then, to install, run

```bash
sudo make install
``` 

#### Refresh the linker's cache after install finishes by running

```bash
sudo ldconfig
```

---

### 4. Broadcast stack and PipeWire/JACK bridge (Ubuntu 26.04 LTS)

This section covers what `rivapi` (the Go dashboard/API, see
[`docs/specs/0005-go-api-foundation.md`](https://github.com/anjeleno/rivolution/blob/main/docs/specs/0005-go-api-foundation.md))
and the broadcast pipeline (Icecast, ffmpeg, Stereo Tool, PipeWire —
see [`docs/specs/0007-pipewire-audio-engine.md`](https://github.com/anjeleno/rivolution/blob/main/docs/specs/0007-pipewire-audio-engine.md),
[`docs/specs/0008-broadcast-tool-suite-integration.md`](https://github.com/anjeleno/rivolution/blob/main/docs/specs/0008-broadcast-tool-suite-integration.md),
and [`docs/specs/0015-ffmpeg-broadcast-output.md`](https://github.com/anjeleno/rivolution/blob/main/docs/specs/0015-ffmpeg-broadcast-output.md))
need at runtime. None of this links into the Rivolution C++ binaries
themselves — it's a separate layer, verified working end-to-end on a
real Ubuntu 26.04 install.

#### Go toolchain (for `rivapi`)

```bash
sudo apt install golang-go
```

`rivapi/go.mod` requires Go 1.25.0 or later. Verify with `go version`
after installing — Ubuntu 26.04's `golang-go` package ships 1.26.0,
comfortably past the floor.

#### Broadcast stack packages

```bash
sudo apt install icecast2 ffmpeg fdkaac
```

`fdkaac` is needed for AAC stream output. Its actual command-line
syntax and codec support on this platform differ from what's commonly
documented elsewhere — see spec 0008's "`fdkaac`/Vorbis command-line
reference" section for the exact, verified flags, and `BACKLOG.md` for
why the HE-AAC/SBR codec options currently fall back to plain AAC-LC
(Ubuntu's `libfdk-aac2` ships with SBR encoding disabled).

#### PipeWire/JACK bridge

```bash
sudo apt install pipewire-jack
```

This lets `caed`, each broadcast stream's `ffmpeg` process, and Stereo
Tool (all JACK clients) reach the system-scope PipeWire instance rather
than real hardware/`jackd`.
**One critical extra step, not optional:** if `libjack-jackd2-dev` is
also installed (it's in the required build dependencies above, needed
for `cae/driver_jack.cpp` to compile), its real runtime library
conflicts with the `pipewire-jack` shim — `ldconfig` resolves
`libjack.so.0` alphabetically by `/etc/ld.so.conf.d/*.conf` filename,
and the standard `aarch64-linux-gnu.conf` (or `x86_64-linux-gnu.conf`)
sorts before `pipewire-jack`'s own conf file, silently making every
JACK client link against the real library instead of the shim — with
no error, just processes that fail to reach PipeWire at all. Fix:

```bash
sudo mv /etc/ld.so.conf.d/pipewire-jack-$(uname -m)-linux-gnu.conf /etc/ld.so.conf.d/00-pipewire-jack-$(uname -m)-linux-gnu.conf
```
```bash
sudo ldconfig
```

Verify the shim now wins:

```bash
ldconfig -p | grep "libjack.so.0 "
```

The `pipewire-0.3/jack/libjack.so.0` line must appear **first**.

#### Stereo Tool's ALSA-JACK bridge

Stereo Tool reaches JACK only through an ALSA plugin (its "jack
(ALSA)" I/O option), not as a native JACK client, and the stock plugin
config hardcodes real hardware port names that don't exist under
system-scope PipeWire. Install the override:

```bash
cp conf/alsa/rd.asoundrc ~/.asoundrc
```

This is a temporary, hardcoded routing fix — see `conf/alsa/rd.asoundrc`'s
own header comment and `BACKLOG.md` for why, and the dashboard's
`/patchbay` page for the actual persistent routing mechanism now in
use instead of hand-editing this file further.

#### `rivapi` and `conf/` deployment

Build and install `rivapi` as a systemd service, and deploy the
`conf/` files (systemd units, sudoers rule, ALSA override above). See
[`docs/specs/0010-systemd-stack-orchestration.md`](https://github.com/anjeleno/rivolution/blob/main/docs/specs/0010-systemd-stack-orchestration.md)'s
Deployment section for the full, current file-by-file list — not
duplicated here since it changes as new units are added. In short:

```bash
cd rivapi && go build -o rivapi .
```
```bash
sudo install -m 755 rivapi/rivapi /usr/local/bin/rivapi
```
```bash
sudo cp conf/systemd/rivapi.service /etc/systemd/system/
```
```bash
sudo cp conf/sudoers.d/rivapi /etc/sudoers.d/rivapi && sudo chmod 440 /etc/sudoers.d/rivapi
```
```bash
sudo systemctl daemon-reload && sudo systemctl enable --now rivapi.service
```

`scripts/rivapi-rebuild.sh` automates the build+install+restart
sequence for subsequent source changes.

#### VLC

Used for remote broadcast via an Icecast relay: a remote encoder
streams to an Icecast source, and VLC at the studio end plays that
stream with its output patched into Rivendell's input (see spec 0010's
Background section) — not a systemd-managed service, launched manually
as needed, but required to be present:

```bash
sudo apt install vlc vlc-plugin-jack
```