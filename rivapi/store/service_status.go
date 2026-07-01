package store

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
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
var ManagedUnits = []StackUnit{
	{"rivendell.service", "Rivolution", true},
	{"icecast2.service", "Icecast", false},
	{"liquidsoap.service", "Liquidsoap", false},
	{"stereo-tool.service", "Stereo Tool", false},
	{"tailscaled.service", "Tailscale", false},
}

// QueryStackStatus returns the current state of every managed unit.
// systemctl show does not require privilege; called directly.
func QueryStackStatus() ([]ServiceStatus, error) {
	result := make([]ServiceStatus, len(ManagedUnits))
	for i, u := range ManagedUnits {
		state, detail := unitInfo(u.Unit)
		var warn string
		if u.Unit == "liquidsoap.service" {
			warn = liquidsoapWarn()
		}
		result[i] = ServiceStatus{u, state, detail, warn}
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

// liquidsoapWarn checks the Liquidsoap log for a JACK connection failure
// within the last 10 minutes. When JACK / PipeWire is not yet running,
// Liquidsoap logs this error on every start but systemd still marks the
// unit active (the process stays alive with a dead clock thread).
// The warning clears automatically once PipeWire is running and
// Liquidsoap stops logging the error.
func liquidsoapWarn() string {
	const logPath = "/home/rd/Log/liquidsoap.log"
	const jackErr = "Could not open JACK device"

	f, err := os.Open(logPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return ""
	}
	// Read at most the last 8 KB — enough for several minutes of log lines.
	offset := info.Size() - 8192
	if offset < 0 {
		offset = 0
	}
	if _, err = f.Seek(offset, io.SeekStart); err != nil {
		return ""
	}
	data, _ := io.ReadAll(f)

	cutoff := time.Now().Add(-10 * time.Minute)
	for _, line := range strings.Split(string(data), "\n") {
		if len(line) < 19 {
			continue
		}
		t, err := time.ParseInLocation("2006/01/02 15:04:05", line[:19], time.Local)
		if err != nil {
			continue
		}
		if t.After(cutoff) && strings.Contains(line, jackErr) {
			return "JACK device not available — Liquidsoap is running but not streaming (expected until PipeWire bridge is set up)"
		}
	}
	return ""
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

func isAllowedUnit(unit string) bool {
	if unit == StackTarget {
		return true
	}
	for _, u := range ManagedUnits {
		if u.Unit == unit {
			return true
		}
	}
	return false
}
