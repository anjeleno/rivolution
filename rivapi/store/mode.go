package store

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// InstallMode is which of the three network topologies this station runs.
// See docs/specs/0001-install-modes.md (rivolution-unified-installer) for
// the original Ansible-side definition this mirrors: standalone has
// everything local; server additionally exposes the database and audio
// store to other Rivendell hosts; client has neither locally and points at
// a remote host for both.
type InstallMode string

const (
	ModeStandalone InstallMode = "standalone"
	ModeServer     InstallMode = "server"
	ModeClient     InstallMode = "client"
)

// ModeConfig is the persisted install-mode intent — what the operator asked
// for, applied by ApplyMode. Only the Remote* fields are consulted when
// Mode is ModeClient; they're kept (not cleared) when switching away from
// client so switching back doesn't require re-entering them.
type ModeConfig struct {
	Mode InstallMode `json:"mode"`

	RemoteMySQLHost     string `json:"remote_mysql_host"`
	RemoteMySQLUser     string `json:"remote_mysql_user"`
	RemoteMySQLPassword string `json:"remote_mysql_password"`
	RemoteMySQLDatabase string `json:"remote_mysql_database"`
	RemoteNFSHost       string `json:"remote_nfs_host"`
}

// ModeConfigPath is where the mode dashboard persists its JSON config,
// same pattern as BroadcastConfigPath/DesiredLinksPath.
const ModeConfigPath = "/home/rd/etc/rivolution/mode.json"

// LoadModeConfig reads the saved mode config. Returns ModeStandalone with
// otherwise-empty fields, not an error, if the file doesn't exist yet —
// standalone (everything local) is this project's original, still-default
// behavior for any install that predates this page.
func LoadModeConfig(path string) (ModeConfig, error) {
	cfg := ModeConfig{Mode: ModeStandalone}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// SaveModeConfig writes cfg as the persisted mode intent. Atomic (temp file
// + rename), same pattern as SaveBroadcastConfig/SaveDesiredLinks.
func SaveModeConfig(cfg ModeConfig, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil { // 0600: may contain a remote DB password
		return err
	}
	return os.Rename(tmp, path)
}

// ModeStatus is the live, actually-observed state of everything ApplyMode
// touches — deliberately queried fresh every time rather than trusted from
// ModeConfig, since the whole point of surfacing this page is that the
// dashboard's stored intent and the box's real state can drift (see
// KNOWN_ISSUES.md's NFS mount/fstab entry, found 2026-07-04).
type ModeStatus struct {
	Config ModeConfig

	MariaDBInstalled     bool
	MariaDBActive        bool
	MariaDBListensRemote bool // bind-address is not 127.0.0.1/localhost

	NFSServerInstalled bool
	ExportsConfigured  bool // /etc/exports has this project's entries

	NFSClientInstalled bool
	AutofsInstalled    bool
	VarSndMounted      bool   // /var/snd is currently mounted from somewhere
	VarSndMountSource  string // e.g. "203.0.113.5:/srv/nfs4/var/snd", empty if not mounted
	FstabEntryPresent  bool   // /var/snd has a real /etc/fstab line, not just a live mount

	RdConfMySQLHost string // current [mySQL] Hostname in /etc/rd.conf, whatever it actually says
}

const mariadbBindConfPath = "/etc/mysql/mariadb.conf.d/50-server.cnf"
const exportsPath = "/etc/exports"
const fstabPath = "/etc/fstab"
const varSndPath = "/var/snd"
const rdConfPath = "/etc/rd.conf"

// QueryModeStatus gathers the live state of every mode-relevant subsystem.
// Best-effort throughout: a query that fails (package not installed, file
// not present) reports the corresponding false/empty value rather than an
// error, since "not set up yet" is an entirely expected, common state here,
// not a fault condition.
func QueryModeStatus(cfg ModeConfig) ModeStatus {
	s := ModeStatus{Config: cfg}

	s.MariaDBInstalled = packageInstalled("mariadb-server")
	s.MariaDBActive = systemctlIsActive("mariadb.service")
	s.MariaDBListensRemote = mariadbListensRemote()

	s.NFSServerInstalled = packageInstalled("nfs-kernel-server")
	s.ExportsConfigured = fileContains(exportsPath, "/srv/nfs4/var/snd")

	s.NFSClientInstalled = packageInstalled("nfs-common")
	s.AutofsInstalled = packageInstalled("autofs")
	s.VarSndMountSource = mountSourceOf(varSndPath)
	s.VarSndMounted = s.VarSndMountSource != ""
	s.FstabEntryPresent = fileContains(fstabPath, " "+varSndPath+" ")

	s.RdConfMySQLHost = rdConfValue(rdConfPath, "mySQL", "Hostname")

	return s
}

func packageInstalled(pkg string) bool {
	// dpkg-query exits 0 only when the package is actually installed
	// (not just known-but-removed), matching `dpkg -l`'s "ii" state.
	out, err := exec.Command("dpkg-query", "-W", "-f=${Status}", pkg).Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "install ok installed")
}

func systemctlIsActive(unit string) bool {
	out, _ := exec.Command("systemctl", "is-active", unit).Output()
	return strings.TrimSpace(string(out)) == "active"
}

// mariadbListensRemote reports whether MariaDB's bind-address is set to
// something other than loopback -- the actual network-level gate on remote
// connections. The SQL-level grants (rduser@'%') are already created
// unconditionally for every install by rivolution-first-run.sh, so this,
// not the grant, is what actually distinguishes "server" from "standalone"
// for MySQL reachability.
func mariadbListensRemote() bool {
	data, err := os.ReadFile(mariadbBindConfPath)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok && strings.TrimSpace(k) == "bind-address" {
			v = strings.TrimSpace(v)
			return v != "127.0.0.1" && v != "localhost" && v != "::1"
		}
	}
	// No bind-address line at all: package default ships one commented-out
	// at 127.0.0.1, so absence of an active line is ambiguous. Treat as
	// "not confirmed remote" -- the conservative reading for a status page.
	return false
}

// mountSourceOf returns what path is mounted at target right now (the
// "source" field pw-link-style tools would call the device), or "" if
// target isn't a separate mount point at all. Reads /proc/mounts directly
// rather than shelling out to findmnt/mount, since the format is fixed and
// this avoids depending on findmnt's own output-format stability.
func mountSourceOf(target string) string {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if fields[1] == target {
			return fields[0]
		}
	}
	return ""
}

func fileContains(path, substr string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), substr)
}

// rdConfValue reads one key from one section of /etc/rd.conf. Same
// INI-parsing shape as config.go's parseRdConf, kept separate rather than
// exported/shared since that one is a one-shot startup read and this one
// is called on every status-page load — not worth coupling the two just to
// save a dozen lines.
func rdConfValue(path, section, key string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	current := ""
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			current = line[1 : len(line)-1]
			continue
		}
		if current != section {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok && strings.TrimSpace(k) == key {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
