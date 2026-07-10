package store

import (
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

// StackUnit describes one managed service in the broadcast stack.
type StackUnit struct {
	Unit        string // systemd unit name, e.g. "rivendell.service"
	DisplayName string // label shown in the dashboard
	AudioPath   bool   // true if restarting this unit interrupts live audio
}

// ServiceStatus is the runtime state of a single stack unit.
type ServiceStatus struct {
	StackUnit
	State  string // "active", "inactive", "failed", "activating", "unknown"
	Detail string // sub-state detail shown parenthetically, e.g. "auto-restart", "start-post"
	Warn   string // non-empty when the unit is "active" but a health check found a problem
}

// StackTarget is the systemd target that groups the full broadcast stack.
const StackTarget = "rivolution-stack.target"

// ManagedUnits is the ordered list of individual services the dashboard controls.
// Units not yet installed return state "unknown" and are displayed accordingly.
// The broadcast streams themselves aren't listed here -- there's no fixed
// count (it depends on how many mounts are configured on /broadcast), so
// QueryStackStatus appends one row per currently-deployed stream unit
// dynamically instead. See streamUnitStatuses.
var ManagedUnits = []StackUnit{
	{"pipewire-system.service", "PipeWire", false},
	{"wireplumber-system.service", "WirePlumber", false},
	{"rivendell.service", "Rivolution", true},
	{"icecast2.service", "Icecast", false},
	{"stereo-tool.service", "Stereo Tool", false},
	{"tailscaled.service", "Tailscale", false},
}

// QueryStackStatus returns the current state of every managed unit, plus
// one row per currently-deployed broadcast stream (see streamUnitStatuses).
// systemctl show does not require privilege; called directly.
func QueryStackStatus() ([]ServiceStatus, error) {
	result := make([]ServiceStatus, len(ManagedUnits))
	for i, u := range ManagedUnits {
		state, detail := unitInfo(u.Unit)
		result[i] = ServiceStatus{u, state, detail, ""}
	}
	streams, err := streamUnitStatuses()
	if err != nil {
		return nil, err
	}
	return append(result, streams...), nil
}

// streamUnitStatuses returns one ServiceStatus per stream unit recorded in
// FfmpegStreamsManifestPath (the mounts currently configured on
// /broadcast), sorted by mount for stable display order. Each is
// AudioPath: true -- restarting one interrupts that stream's live audio,
// same reasoning as rivendell.service above. Returns an empty slice, not
// an error, if no streams have been deployed yet.
func streamUnitStatuses() ([]ServiceStatus, error) {
	manifest, err := loadFfmpegStreamsManifest(FfmpegStreamsManifestPath)
	if err != nil {
		return nil, err
	}
	mounts := make([]string, 0, len(manifest))
	for mount := range manifest {
		mounts = append(mounts, mount)
	}
	sort.Strings(mounts)

	result := make([]ServiceStatus, 0, len(mounts))
	for _, mount := range mounts {
		unit := TaskUnitName(manifest[mount]) + ".service"
		state, detail := unitInfo(unit)
		result = append(result, ServiceStatus{
			StackUnit: StackUnit{Unit: unit, DisplayName: "Stream " + mount, AudioPath: true},
			State:     state,
			Detail:    detail,
		})
	}
	return result, nil
}

// unitInfo queries ActiveState and SubState via systemctl show.
// Returns (state, detail) where detail is the informative sub-state when
// it carries meaning beyond the top-level state (e.g. "auto-restart",
// "start-post"). Trivial sub-states ("running", "dead") are suppressed.
func unitInfo(unit string) (state, detail string) {
	out, _ := exec.Command("systemctl", "show", "--property=ActiveState,SubState", unit).Output()
	var active, sub string
	for _, line := range strings.Split(string(out), "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch k {
		case "ActiveState":
			active = strings.TrimSpace(v)
		case "SubState":
			sub = strings.TrimSpace(v)
		}
	}
	if active == "" {
		return "unknown", ""
	}
	state = active
	switch active {
	case "active":
		// "running" and "exited" are unremarkable; omit them.
	case "activating", "deactivating":
		// sub-states here ("start-post", "auto-restart", "stop") tell
		// the operator exactly what systemd is waiting on.
		if sub != "" && sub != active {
			detail = sub
		}
	case "failed":
		// sub-state is always "dead" for failed units; not informative.
	}
	return state, detail
}

// ControlUnit runs `sudo systemctl <action> <unit>`.
// action must be "start", "stop", or "restart".
// unit must be StackTarget or one of ManagedUnits — all other values are rejected
// to prevent command injection through the HTTP API.
func ControlUnit(unit, action string) error {
	switch action {
	case "start", "stop", "restart":
	default:
		return fmt.Errorf("unsupported action %q", action)
	}
	if !isAllowedUnit(unit) {
		return fmt.Errorf("unit %q is not in the managed list", unit)
	}
	out, err := exec.Command("sudo", "systemctl", action, unit).CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 5 {
			return fmt.Errorf(
				"%s is not loaded — install conf/systemd/%s to /etc/systemd/system/ then run: sudo systemctl daemon-reload",
				unit, unit,
			)
		}
		return fmt.Errorf("systemctl %s %s: %w: %s", action, unit, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// isAllowedUnit checks unit against ManagedUnits plus every currently
// deployed stream unit (read fresh from the manifest, since the set of
// stream units isn't fixed at compile time -- see streamUnitStatuses)
// before ControlUnit will act on it, so the HTTP API can't be used to
// run systemctl against an arbitrary unit name.
func isAllowedUnit(unit string) bool {
	if unit == StackTarget {
		return true
	}
	for _, u := range ManagedUnits {
		if u.Unit == unit {
			return true
		}
	}
	streams, err := streamUnitStatuses()
	if err != nil {
		return false
	}
	for _, s := range streams {
		if s.Unit == unit {
			return true
		}
	}
	return false
}
