# Requirements to install Rivolution

This walk-through assumes you have already installed virgin Ubuntu
26.04 or 24.04 server, either on physical hardware, a local VM, or on a cloud VPS.

> [!TIP]
> Verified on a DigitalOcean Basic droplet (8 vCPU / 16 GB RAM / 320 GB disk) running Ubuntu 24.04 — full build in ~15 minutes, no changes needed to the steps below.

> [!TIP]
> Also verified on Ubuntu 26.04 arm64 (UTM guest on Apple Silicon) — same ~15 minute build, no changes needed to the steps below. Rivolution builds natively on either x86_64 or arm64; nothing in the build needs cross-compilation or an architecture-specific step.

## 0. Before you start: OS and desktop setup

Start by installing updates, setting your hostname and timezone.

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

Then, you'll need to install a desktop. After **extensive testing** on physical hardware, local UTM and Cloud VPS installs, we recommend minimal MATE (no bloat). SSH into your machine and install as root with:

```bash
apt update && apt install -y --no-install-recommends ubuntu-mate-core mate-system-monitor 
```

If this is a Cloud install, add xRDP for easy remote desktop access:

```bash
apt install -y xrdp dbus-x11
```

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

> [!WARNING]
> A separate, unrelated xRDP problem on a fresh Ubuntu 26.04 install
> specifically: 26.04 defaults to a Wayland session, which xRDP can't
> drive at all — connecting over RDP gets a black screen or an
> immediately-crashing session, even with the accelerated-graphics
> workaround above already applied. Confirmed on a real fresh 26.04
> install.

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

> [!TIP]
> If this is a VM/Cloud install running xRDP remote desktop, run the following command to fix Qt/XCB errors for root-run Rivendell tools under xRDP — as of `v6.0.0-rc1-4`, `postinst` does this automatically for a `.deb` install, so this is only needed for a from-source build or an older install predating that fix.

```bash
sudo ln -s /home/rd/.Xauthority /root/.Xauthority
```
