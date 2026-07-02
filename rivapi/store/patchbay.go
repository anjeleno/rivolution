package store

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// PatchLink is one active connection between an output port and an input port.
type PatchLink struct {
	Output string
	Input  string
}

// DesiredLinksPath is where the patchbay's saved (persistent) link set is
// stored. Reconciled against the live PipeWire graph on a timer — see
// ReconcileLinks — since links themselves don't survive either endpoint's
// process restart (a PipeWire limitation, not something we can fix by
// storing the list correctly; verified 2026-07-01 that WirePlumber's own
// declarative target-metadata mechanism does not apply to JACK-bridged
// ports, so this poll-and-reapply approach is deliberate, not a stopgap
// for something not yet wired up — see docs/specs/0007-pipewire-audio-engine.md).
const DesiredLinksPath = "/home/rd/etc/rivolution/patchbay.json"

// LoadDesiredLinks reads the saved link set. Returns an empty slice, not an
// error, if the file doesn't exist yet (nothing saved yet).
func LoadDesiredLinks(path string) ([]PatchLink, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var links []PatchLink
	if err := json.Unmarshal(data, &links); err != nil {
		return nil, err
	}
	return links, nil
}

// SaveDesiredLinks writes links as the persistent set, creating parent
// directories as needed. The write is atomic (temp file + rename).
func SaveDesiredLinks(links []PatchLink, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(links, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ReconcileLinks re-applies any saved link that isn't currently present in
// the live graph. Ports that don't exist yet (e.g. a client hasn't started)
// fail individually via Link() and are skipped, not treated as fatal —
// reconciliation just tries again on the next call.
func ReconcileLinks(path string) error {
	desired, err := LoadDesiredLinks(path)
	if err != nil {
		return fmt.Errorf("load desired links: %w", err)
	}
	if len(desired) == 0 {
		return nil
	}
	current, err := ListPatchLinks()
	if err != nil {
		return fmt.Errorf("list current links: %w", err)
	}
	have := make(map[string]bool, len(current))
	for _, l := range current {
		have[linkPairKey(l.Output, l.Input)] = true
	}
	for _, l := range desired {
		if !have[linkPairKey(l.Output, l.Input)] {
			_ = Link(l.Output, l.Input) // best-effort; port may not exist yet
		}
	}
	return nil
}

func linkPairKey(output, input string) string {
	return output + "|" + input
}

// pwEnv sets XDG_RUNTIME_DIR for a pw-link invocation. rivapi.service also
// sets this at the unit level, but setting it explicitly here means these
// functions behave correctly even if called from a context that doesn't
// (e.g. a future CLI tool, or local testing).
func pwEnv(cmd *exec.Cmd) {
	cmd.Env = append(cmd.Environ(), "XDG_RUNTIME_DIR=/run/pipewire-system")
}

// ListOutputPorts returns every JACK/PipeWire output (source) port, one
// "client:port" name per entry, in pw-link's own order.
func ListOutputPorts() ([]string, error) {
	return pwLinkPorts("-o")
}

// ListInputPorts returns every JACK/PipeWire input (sink) port.
func ListInputPorts() ([]string, error) {
	return pwLinkPorts("-i")
}

func pwLinkPorts(flag string) ([]string, error) {
	cmd := exec.Command("pw-link", flag)
	pwEnv(cmd)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("pw-link %s: %w", flag, err)
	}
	var ports []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			ports = append(ports, line)
		}
	}
	return ports, nil
}

// ListPatchLinks returns every currently active output->input link.
//
// Parses "pw-link -l", which lists every port with its own connections
// from that port's perspective (an "|->" line under an output port, an
// "|<-" line under an input port — each real link therefore appears
// twice, once from each side). Only "|->" lines are collected, so each
// link is returned exactly once.
func ListPatchLinks() ([]PatchLink, error) {
	cmd := exec.Command("pw-link", "-l")
	pwEnv(cmd)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("pw-link -l: %w", err)
	}
	var links []PatchLink
	var current string
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			current = strings.TrimSpace(line)
			continue
		}
		trimmed := strings.TrimSpace(line)
		if target, ok := strings.CutPrefix(trimmed, "|->"); ok {
			links = append(links, PatchLink{Output: current, Input: strings.TrimSpace(target)})
		}
	}
	return links, nil
}

// Link connects an output port to an input port. No privilege required —
// pw-link operates as the calling user against their own PipeWire instance.
func Link(output, input string) error {
	cmd := exec.Command("pw-link", output, input)
	pwEnv(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("pw-link %s %s: %w: %s", output, input, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Unlink disconnects an output port from an input port.
func Unlink(output, input string) error {
	cmd := exec.Command("pw-link", "-d", output, input)
	pwEnv(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("pw-link -d %s %s: %w: %s", output, input, err, strings.TrimSpace(string(out)))
	}
	return nil
}
