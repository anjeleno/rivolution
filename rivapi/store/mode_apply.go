package store

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// ApplyMode switches this station between standalone/server/client, per
// cfg. Returns a step-by-step log of what actually happened (mirroring
// BroadcastSave's result.Steps pattern) even on partial failure, so an
// operator can see exactly how far it got before something went wrong.
//
// Deliberately conservative about what it will and won't touch:
//   - Never installs or removes mariadb-server based on mode. The local
//     Rivendell database and rduser (both @'localhost' and @'%') are
//     already created unconditionally by scripts/rivolution-first-run.sh
//     at package-install time, regardless of mode -- there's no
//     per-mode branch in that script today. So standalone/server only
//     ever need to ensure the already-installed package is running with
//     the right bind-address; client mode leaves mariadb-server entirely
//     alone (installed or not) and just stops pointing rd.conf at it.
//   - Never uninstalls anything, never drops a database, never deletes
//     an existing local audio store. Switching away from a mode just
//     stops actively using what that mode set up; nothing destructive
//     happens automatically.
//
// UNTESTED against a real box as of 2026-07-04 -- built following the
// same patterns as BroadcastSave/PatchbaySave, but this project's own
// hard-won lesson tonight is that real install/upgrade testing finds
// what static reading can't. Treat this as needing the same real-box
// verification pass everything else got before trusting it in production.
func ApplyMode(cfg ModeConfig) ([]string, error) {
	var steps []string
	step := func(s string) { steps = append(steps, s) }

	switch cfg.Mode {
	case ModeStandalone, ModeServer:
		if err := aptInstallMariaDB(); err != nil {
			return steps, fmt.Errorf("installing mariadb-server: %w", err)
		}
		step("mariadb-server present.")

		if err := sudoSystemctl("enable", "--now", "mariadb.service"); err != nil {
			return steps, fmt.Errorf("starting mariadb: %w", err)
		}
		step("mariadb started and enabled.")

		bindTarget := "127.0.0.1"
		if cfg.Mode == ModeServer {
			bindTarget = "0.0.0.0"
		}
		if err := setMariaDBBindAddress(bindTarget); err != nil {
			return steps, fmt.Errorf("setting mariadb bind-address: %w", err)
		}
		step(fmt.Sprintf("mariadb bind-address set to %s.", bindTarget))

		if err := sudoSystemctl("restart", "mariadb.service"); err != nil {
			return steps, fmt.Errorf("restarting mariadb: %w", err)
		}
		step("mariadb restarted.")

		// rduser/Rivendell are the fixed names rivolution-first-run.sh
		// always creates locally -- not user-editable here, since this
		// branch is specifically "use the local database," not "use some
		// arbitrary database."
		localPassword := rdConfValue(rdConfPath, "mySQL", "Password")
		if err := writeRdConfMySQL("localhost", "rduser", localPassword, "Rivendell"); err != nil {
			return steps, fmt.Errorf("updating rd.conf: %w", err)
		}
		step("rd.conf [mySQL] pointed at localhost.")

		if err := ensureVarSndLocal(); err != nil {
			return steps, fmt.Errorf("detaching /var/snd from any remote mount: %w", err)
		}
		step("/var/snd confirmed as a local directory (not NFS-mounted).")

		if err := writeRdConfAudioStore("", ""); err != nil {
			return steps, fmt.Errorf("updating rd.conf: %w", err)
		}
		step("rd.conf [AudioStore] cleared (local audio store).")

		if cfg.Mode == ModeServer {
			if err := aptInstallNFSServer(); err != nil {
				return steps, fmt.Errorf("installing nfs-kernel-server: %w", err)
			}
			step("nfs-kernel-server present.")

			if err := createNFSExportDirs(); err != nil {
				return steps, fmt.Errorf("creating NFS export directories: %w", err)
			}
			step("NFS export directories created under /srv/nfs4.")

			if err := writeExports(); err != nil {
				return steps, fmt.Errorf("writing /etc/exports: %w", err)
			}
			step("/etc/exports updated.")

			if err := sudoRun("exportfs", "-ra"); err != nil {
				return steps, fmt.Errorf("re-exporting NFS shares: %w", err)
			}
			step("NFS shares re-exported.")
		}

	case ModeClient:
		if strings.TrimSpace(cfg.RemoteNFSHost) == "" {
			return steps, fmt.Errorf("client mode requires a remote NFS host")
		}
		if !validHostOrIP(cfg.RemoteNFSHost) {
			return steps, fmt.Errorf("remote NFS host %q doesn't look like a valid hostname or IP", cfg.RemoteNFSHost)
		}
		if strings.TrimSpace(cfg.RemoteMySQLHost) == "" {
			return steps, fmt.Errorf("client mode requires a remote MySQL host")
		}

		if err := aptInstallNFSClient(); err != nil {
			return steps, fmt.Errorf("installing nfs-common/autofs: %w", err)
		}
		step("nfs-common and autofs present.")

		if err := mountRemoteVarSnd(cfg.RemoteNFSHost); err != nil {
			return steps, fmt.Errorf("mounting remote audio store: %w", err)
		}
		step(fmt.Sprintf("/var/snd mounted from %s:/srv/nfs4/var/snd.", cfg.RemoteNFSHost))

		if err := writeFstab(cfg.RemoteNFSHost); err != nil {
			return steps, fmt.Errorf("writing /etc/fstab: %w", err)
		}
		step("/etc/fstab entry written so the mount survives a reboot.")

		if err := writeAutofsMap(cfg.RemoteNFSHost); err != nil {
			return steps, fmt.Errorf("writing autofs map: %w", err)
		}
		step("autofs audio-store map written.")

		if err := writeAutofsMaster(); err != nil {
			return steps, fmt.Errorf("writing autofs master map: %w", err)
		}
		step("autofs master map updated.")

		if err := sudoSystemctl("restart", "autofs.service"); err != nil {
			return steps, fmt.Errorf("restarting autofs: %w", err)
		}
		step("autofs restarted.")

		if err := symlinkAutofsDirs(); err != nil {
			return steps, fmt.Errorf("symlinking staging directories: %w", err)
		}
		step("rd_xfer/music_export/music_import/traffic_export/traffic_import symlinked to the autofs mount.")

		if err := writeRdConfMySQL(cfg.RemoteMySQLHost, cfg.RemoteMySQLUser, cfg.RemoteMySQLPassword, cfg.RemoteMySQLDatabase); err != nil {
			return steps, fmt.Errorf("updating rd.conf: %w", err)
		}
		step("rd.conf [mySQL] pointed at the remote host.")

		if err := writeRdConfAudioStore(cfg.RemoteNFSHost+":/srv/nfs4/var/snd", "nfs4"); err != nil {
			return steps, fmt.Errorf("updating rd.conf: %w", err)
		}
		step("rd.conf [AudioStore] pointed at the remote mount.")

	default:
		return steps, fmt.Errorf("unknown mode %q", cfg.Mode)
	}

	if err := ControlUnit("rivendell.service", "restart"); err != nil {
		return steps, fmt.Errorf("restarting rivendell: %w", err)
	}
	step("rivendell restarted to pick up the new configuration.")

	return steps, nil
}

// validHostOrIP is a deliberately strict allow-list check on a value that
// flows into a sudo-whitelisted mount command: hostname/IPv4 characters
// only, so it can never be interpreted as a second argument or shell
// metacharacter no matter how the wildcard half of the sudoers entry
// matches it.
var validHostOrIPRe = regexp.MustCompile(`^[A-Za-z0-9.-]+$`)

func validHostOrIP(s string) bool {
	return s != "" && len(s) < 256 && validHostOrIPRe.MatchString(s)
}

func sudoRun(args ...string) error {
	out, err := exec.Command("sudo", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func sudoSystemctl(args ...string) error {
	return sudoRun(append([]string{"systemctl"}, args...)...)
}

// isExitCode reports whether err (possibly wrapped, e.g. by sudoRun's
// fmt.Errorf("...: %w: ...")) is an *exec.ExitError with the given code.
// Same check dashboard.isExitCode makes for BroadcastSave's liquidsoap
// tolerance, duplicated here rather than exported across packages for one
// two-line helper.
func isExitCode(err error, code int) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr) && exitErr.ExitCode() == code
}

func aptInstallMariaDB() error {
	if packageInstalled("mariadb-server") {
		return nil
	}
	return aptGetInstall("mariadb-server", "mariadb-client")
}

func aptInstallNFSServer() error {
	if packageInstalled("nfs-kernel-server") {
		return nil
	}
	return aptGetInstall("nfs-kernel-server")
}

func aptInstallNFSClient() error {
	if packageInstalled("nfs-common") && packageInstalled("autofs") {
		return nil
	}
	return aptGetInstall("nfs-common", "autofs")
}

// dpkgLockWait bounds how long aptGetInstall will keep retrying against a
// held dpkg lock before giving up and reporting the failure for real.
const dpkgLockWait = 3 * time.Minute

// aptGetInstall runs `apt-get install -y <pkgs>`, retrying on dpkg lock
// contention instead of failing on the first attempt. Confirmed live
// 2026-07-05: a client-mode switch failed outright because
// unattended-upgrades (which runs on its own schedule, independent of
// anything this dashboard does) held /var/lib/dpkg/lock-frontend at that
// exact moment -- a routine, transient condition on any Ubuntu box, not a
// real failure to surface immediately. Any other failure (bad package name,
// no network, etc.) still returns right away.
func aptGetInstall(pkgs ...string) error {
	args := append([]string{"apt-get", "install", "-y"}, pkgs...)
	deadline := time.Now().Add(dpkgLockWait)
	for {
		err := sudoRun(args...)
		if err == nil || !isDpkgLockContention(err) || time.Now().After(deadline) {
			return err
		}
		time.Sleep(5 * time.Second)
	}
}

// isDpkgLockContention recognizes apt-get's own message for "another process
// (most commonly unattended-upgrades) currently holds a dpkg lock file" --
// apt uses this same wording across lock-frontend, the archives lock, and
// the main dpkg lock, so matching on it covers all three rather than one
// specific lock path.
func isDpkgLockContention(err error) bool {
	return err != nil && strings.Contains(err.Error(), "Could not get lock")
}

// setMariaDBBindAddress flips MariaDB's bind-address between loopback
// (standalone/client -- though client never calls this) and all-interfaces
// (server). Same regenerate-then-install pattern as writeRdConfMySQL/
// writeExports/writeFstab, rather than a sudo-whitelisted `sed` — a `sed`
// expression containing spaces is one execve argument from Go's side but
// fragile to get sudoers to match correctly as a single token (sudoers
// splits command-spec arguments on whitespace unless every embedded space
// is individually backslash-escaped); reading the world-readable config
// file directly and shipping the whole edited file through a fixed
// `install` command sidesteps that entirely.
func setMariaDBBindAddress(target string) error {
	if target != "0.0.0.0" && target != "127.0.0.1" {
		return fmt.Errorf("internal error: unexpected bind-address target %q", target)
	}
	data, err := os.ReadFile(mariadbBindConfPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", mariadbBindConfPath, err)
	}
	lines := strings.Split(string(data), "\n")
	found := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			continue
		}
		if k, _, ok := strings.Cut(trimmed, "="); ok && strings.TrimSpace(k) == "bind-address" {
			lines[i] = "bind-address = " + target
			found = true
		}
	}
	if !found {
		lines = append(lines, "bind-address = "+target)
	}
	staged, err := writeStaged("50-server.cnf.staged", strings.Join(lines, "\n"))
	if err != nil {
		return err
	}
	return sudoRun("install", "-o", "root", "-g", "root", "-m", "644", staged, mariadbBindConfPath)
}

// stagingDir holds files this process generates locally (owned by rd) before
// a fixed `sudo install` command atomically deploys them to their real
// system location -- the same "regenerate whole file, then one whitelisted
// install" pattern GenerateIcecastXML/GenerateLiquidsoapScript already use,
// applied here to rd.conf/exports/fstab/autofs maps.
const stagingDir = "/home/rd/etc/rivolution/staging"

func stagePath(name string) string {
	return stagingDir + "/" + name
}

func writeStaged(name, content string) (string, error) {
	if err := os.MkdirAll(stagingDir, 0700); err != nil {
		return "", err
	}
	path := stagePath(name)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return "", err
	}
	return path, nil
}

// writeRdConfMySQL rewrites only the [mySQL] section's four connection
// fields in /etc/rivendell.d/rd-default.conf (the real file /etc/rd.conf
// symlinks to, matching debian/postinst's own convention), preserving
// everything else in the file untouched.
func writeRdConfMySQL(host, user, password, database string) error {
	const realPath = "/etc/rivendell.d/rd-default.conf"
	data, err := os.ReadFile(realPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", realPath, err)
	}

	fields := map[string]string{
		"Hostname":  host,
		"Loginname": user,
		"Password":  password,
		"Database":  database,
	}
	lines := strings.Split(string(data), "\n")
	inSection := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inSection = trimmed == "[mySQL]"
			continue
		}
		if !inSection {
			continue
		}
		if k, _, ok := strings.Cut(trimmed, "="); ok {
			if v, known := fields[strings.TrimSpace(k)]; known {
				lines[i] = k + "=" + v
			}
		}
	}

	staged, err := writeStaged("rd.conf.staged", strings.Join(lines, "\n"))
	if err != nil {
		return err
	}
	return sudoRun("install", "-o", "root", "-g", "root", "-m", "644", staged, realPath)
}

// writeRdConfAudioStore rewrites only [AudioStore]'s MountSource/MountType
// fields, same pattern as writeRdConfMySQL. Rivendell's own health checks
// (RDAudioStoreValid, lib/rdstatus.cpp -- used by rdmonitor(1)/rdselect(1))
// decide whether the audio store is "local" or "remote" purely from whether
// MountSource is empty, and for the remote case, confirm the mount by
// matching MountSource against /etc/mtab's source field. Confirmed live
// 2026-07-06: leaving this section blank while /var/snd is actually a real
// NFS mount (which our own fstab/autofs plumbing sets up independently of
// rd.conf) makes Rivendell think a local audio store somehow ended up on a
// separate mount, and RDAudioStoreValid reports the station unhealthy even
// though the mount itself is completely fine. MountOptions/CaeHostname/
// XportHostname are left untouched -- MountOptions ships a sane "defaults"
// in the base rd.conf template, and Cae/XportHostname only matter for
// split-host topologies this project's client/server modes don't create.
func writeRdConfAudioStore(mountSource, mountType string) error {
	const realPath = "/etc/rivendell.d/rd-default.conf"
	data, err := os.ReadFile(realPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", realPath, err)
	}

	fields := map[string]string{
		"MountSource": mountSource,
		"MountType":   mountType,
	}
	lines := strings.Split(string(data), "\n")
	inSection := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inSection = trimmed == "[AudioStore]"
			continue
		}
		if !inSection {
			continue
		}
		if k, _, ok := strings.Cut(trimmed, "="); ok {
			if v, known := fields[strings.TrimSpace(k)]; known {
				lines[i] = k + "=" + v
			}
		}
	}

	staged, err := writeStaged("rd.conf.staged", strings.Join(lines, "\n"))
	if err != nil {
		return err
	}
	return sudoRun("install", "-o", "root", "-g", "root", "-m", "644", staged, realPath)
}

// ensureVarSndLocal unmounts /var/snd if it's currently an NFS mount and
// removes any fstab entry for it, leaving a plain local directory behind.
// Never deletes the directory's contents -- unmounting an NFS mount just
// re-exposes whatever's underneath (normally nothing, an empty mountpoint),
// it doesn't destroy the remote data.
func ensureVarSndLocal() error {
	if mountSourceOf(varSndPath) == "" {
		return removeFstabEntryFor(varSndPath) // no-op if there was never one
	}
	if err := sudoRun("umount", varSndPath); err != nil {
		return err
	}
	return removeFstabEntryFor(varSndPath)
}

func createNFSExportDirs() error {
	dirs := []string{
		"/srv/nfs4/var/snd",
		"/srv/nfs4/home/rd/music_export",
		"/srv/nfs4/home/rd/music_import",
		"/srv/nfs4/home/rd/traffic_export",
		"/srv/nfs4/home/rd/traffic_import",
		"/srv/nfs4/home/rd/rd_xfer",
	}
	for _, d := range dirs {
		if err := sudoRun("mkdir", "-p", d); err != nil {
			return err
		}
	}
	return nil
}

// writeExports regenerates /etc/exports in full: keeps any lines already
// there that this project didn't add (an operator's own additions), then
// ensures this project's six fixed export lines are present, adding any
// that are missing rather than duplicating ones already there.
func writeExports() error {
	wanted := []string{
		"/srv/nfs4/var/snd *(rw,no_root_squash)",
		"/srv/nfs4/home/rd/music_export *(rw,no_root_squash)",
		"/srv/nfs4/home/rd/music_import *(rw,no_root_squash)",
		"/srv/nfs4/home/rd/traffic_export *(rw,no_root_squash)",
		"/srv/nfs4/home/rd/traffic_import *(rw,no_root_squash)",
		"/srv/nfs4/home/rd/rd_xfer *(rw,no_root_squash)",
	}
	existing, _ := os.ReadFile(exportsPath) // missing file is fine, start from empty
	lines := strings.Split(string(existing), "\n")
	have := make(map[string]bool, len(lines))
	for _, l := range lines {
		have[strings.TrimSpace(l)] = true
	}
	for _, w := range wanted {
		if !have[w] {
			lines = append(lines, w)
		}
	}
	staged, err := writeStaged("exports.staged", strings.Join(lines, "\n")+"\n")
	if err != nil {
		return err
	}
	return sudoRun("install", "-m", "644", staged, exportsPath)
}

// mountVarSndScript takes the remote host as its one argument rather than
// baking a wildcard into a sudoers Cmnd_Alias -- see tasks_deploy.go's
// controlScriptsDir doc comment for why: sudo-rs (Ubuntu 26.04's sudo)
// rejects wildcards in command arguments outright, so the host has to be
// validated inside a fixed, sudo-whitelisted-by-bare-path script instead.
// Deployed alongside the task control scripts, into the same directory.
var mountVarSndScript = `#!/bin/sh
# mount-var-snd.sh <remote-host>
# Generated by rivapi -- do not hand-edit, overwritten on next deploy.
set -e
HOST="$1"
case "$HOST" in
    ''|*[!A-Za-z0-9.-]*) echo "mount-var-snd.sh: invalid host" >&2; exit 1 ;;
esac
mkdir -p /var/snd
mount -t nfs4 "$HOST:/srv/nfs4/var/snd" /var/snd
`

func ensureMountScript() error {
	path := controlScriptsDir + "/mount-var-snd.sh"
	if existing, err := os.ReadFile(path); err == nil && string(existing) == mountVarSndScript {
		return nil
	}
	if err := sudoRun("mkdir", "-p", controlScriptsDir); err != nil {
		return err
	}
	staged, err := writeStaged("control-mount-var-snd.sh", mountVarSndScript)
	if err != nil {
		return err
	}
	return sudoRun("install", "-m", "755", staged, path)
}

// mountRemoteVarSnd mounts now (does not by itself persist across reboot --
// see writeFstab for that). host has already been validated by
// validHostOrIP in ApplyMode before this is ever called (defense in depth:
// mountVarSndScript validates it again independently, shell-side).
func mountRemoteVarSnd(host string) error {
	src := host + ":/srv/nfs4/var/snd"
	if cur := mountSourceOf(varSndPath); cur == src {
		return nil // already mounted from the right place
	} else if cur != "" {
		if err := sudoRun("umount", varSndPath); err != nil {
			return err
		}
	}
	if err := ensureMountScript(); err != nil {
		return fmt.Errorf("deploying mount helper script: %w", err)
	}
	return sudoRun(controlScriptsDir+"/mount-var-snd.sh", host)
}

// writeFstab replaces any existing /var/snd line with one reflecting host,
// preserving every other line untouched. host has already been validated
// by validHostOrIP in ApplyMode.
func writeFstab(host string) error {
	data, err := os.ReadFile(fstabPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", fstabPath, err)
	}
	wanted := fmt.Sprintf("%s:/srv/nfs4/var/snd %s nfs4 rw 0 0", host, varSndPath)
	lines := strings.Split(string(data), "\n")
	replaced := false
	for i, l := range lines {
		fields := strings.Fields(l)
		if len(fields) >= 2 && fields[1] == varSndPath {
			lines[i] = wanted
			replaced = true
		}
	}
	if !replaced {
		lines = append(lines, wanted)
	}
	staged, err := writeStaged("fstab.staged", strings.Join(lines, "\n")+"\n")
	if err != nil {
		return err
	}
	if err := sudoRun("install", "-m", "644", staged, fstabPath); err != nil {
		return err
	}
	return sudoSystemctl("daemon-reload")
}

// removeFstabEntryFor drops any line whose second field (mount point) is
// target, leaving everything else untouched. No-op (not an error) if there
// was never a matching line.
func removeFstabEntryFor(target string) error {
	data, err := os.ReadFile(fstabPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", fstabPath, err)
	}
	var kept []string
	changed := false
	for _, l := range strings.Split(string(data), "\n") {
		fields := strings.Fields(l)
		if len(fields) >= 2 && fields[1] == target {
			changed = true
			continue
		}
		kept = append(kept, l)
	}
	if !changed {
		return nil
	}
	staged, err := writeStaged("fstab.staged", strings.Join(kept, "\n"))
	if err != nil {
		return err
	}
	if err := sudoRun("install", "-m", "644", staged, fstabPath); err != nil {
		return err
	}
	return sudoSystemctl("daemon-reload")
}

const autofsMapPath = "/etc/auto.rd.audiostore"
const autofsMasterPath = "/etc/auto.master"

// writeAutofsMap templates /etc/auto.rd.audiostore, one line per
// staging directory this station needs from the remote NFS host.
func writeAutofsMap(host string) error {
	dirs := []string{
		"rd_xfer", "music_export", "music_import", "traffic_export", "traffic_import",
	}
	var b strings.Builder
	for _, d := range dirs {
		fmt.Fprintf(&b, "%s -fstype=nfs4,rw %s:/srv/nfs4/home/rd/%s\n", d, host, d)
	}
	staged, err := writeStaged("auto.rd.audiostore.staged", b.String())
	if err != nil {
		return err
	}
	return sudoRun("install", "-m", "644", staged, autofsMapPath)
}

// writeAutofsMaster ensures /etc/auto.master references the map above,
// adding the reference if it's missing rather than duplicating it.
func writeAutofsMaster() error {
	const wanted = "/misc /etc/auto.rd.audiostore"
	data, _ := os.ReadFile(autofsMasterPath)
	lines := strings.Split(string(data), "\n")
	for _, l := range lines {
		if strings.TrimSpace(l) == wanted {
			return nil // already referenced
		}
	}
	lines = append(lines, wanted)
	staged, err := writeStaged("auto.master.staged", strings.Join(lines, "\n")+"\n")
	if err != nil {
		return err
	}
	return sudoRun("install", "-m", "644", staged, autofsMasterPath)
}

// symlinkAutofsDirs points rd's own staging directories at the
// autofs-managed /misc/* mounts. Runs unprivileged -- /home/rd is rd's own
// home directory, no sudo needed to create a symlink inside it.
func symlinkAutofsDirs() error {
	dirs := []string{
		"rd_xfer", "music_export", "music_import", "traffic_export", "traffic_import",
	}
	for _, d := range dirs {
		dest := "/home/rd/" + d
		src := "/misc/" + d
		if target, err := os.Readlink(dest); err == nil && target == src {
			continue // already the right symlink
		}
		_ = os.Remove(dest) // may not exist, or may be a real (non-symlink) directory to replace
		if err := os.Symlink(src, dest); err != nil {
			return fmt.Errorf("symlinking %s -> %s: %w", dest, src, err)
		}
	}
	return nil
}
