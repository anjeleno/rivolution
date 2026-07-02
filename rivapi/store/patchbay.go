package store

import (
	"fmt"
	"os/exec"
	"strings"
)

// PatchLink is one active connection between an output port and an input port.
type PatchLink struct {
	Output string
	Input  string
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
