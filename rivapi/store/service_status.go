package store

import (
	"errors"
	"fmt"
	"os/exec"
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
	State string // "active", "inactive", "failed", "activating", "unknown"
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

// QueryStackStatus returns the current active state of every managed unit.
// systemctl is-active does not require privilege; called directly.
func QueryStackStatus() ([]ServiceStatus, error) {
	result := make([]ServiceStatus, len(ManagedUnits))
	for i, u := range ManagedUnits {
		result[i] = ServiceStatus{u, unitState(u.Unit)}
	}
	return result, nil
}

// unitState calls systemctl is-active and returns the trimmed output.
// systemctl is-active exits non-zero for inactive/failed units; the exit
// code is ignored because all non-"active" states are valid values we
// want to surface. An empty output (unit not installed) maps to "unknown".
func unitState(unit string) string {
	out, _ := exec.Command("systemctl", "is-active", unit).Output()
	if s := strings.TrimSpace(string(out)); s != "" {
		return s
	}
	return "unknown"
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
