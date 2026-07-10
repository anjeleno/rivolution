package store

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
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

// ReconcileLinks forces the live graph to exactly match the saved link set:
// applies any saved link that isn't currently present, and removes any live
// link that isn't in the saved set. The removal half matters as much as the
// addition half — WirePlumber's own default auto-linking policy connects
// newly-appeared JACK-bridged ports to default sinks/sources on its own
// initiative (this project ships no override disabling that), and previous
// additive-only reconciliation left those extra connections in place
// indefinitely, on top of the saved patches, until an operator noticed and
// manually restarted the stack. Ports that don't exist yet (e.g. a client
// hasn't started) fail individually via Link()/Unlink() and are skipped, not
// treated as fatal — reconciliation just tries again on the next call.
func ReconcileLinks(path string) error {
	desired, err := LoadDesiredLinks(path)
	if err != nil {
		return fmt.Errorf("load desired links: %w", err)
	}
	if len(desired) == 0 {
		// Nothing has ever been saved (or the saved set is deliberately
		// empty) -- treat this as "no opinion yet" rather than "tear down
		// every connection." Once at least one link has been saved, this
		// function becomes authoritative over the whole graph; until then,
		// forcing an empty graph on every tick would fight a fresh install's
		// initial routing (or WirePlumber's own defaults) with nothing to
		// replace it with.
		return nil
	}
	current, err := ListPatchLinks()
	if err != nil {
		return fmt.Errorf("list current links: %w", err)
	}
	want := make(map[string]bool, len(desired))
	for _, l := range desired {
		want[normalizedLinkKey(l.Output, l.Input)] = true
	}
	have := make(map[string]bool, len(current))
	for _, l := range current {
		have[normalizedLinkKey(l.Output, l.Input)] = true
	}
	// Needed to resolve a saved link's port name to whatever the live
	// equivalent actually is right now, when they differ only by a PID
	// segment (see resolvePortName) -- e.g. Stereo Tool restarting gives
	// it a new client name, and a saved link naming its old one would
	// otherwise fail Link() forever, not just until the PID changes back
	// (it never does), since normalizedLinkKey only affects the
	// have-we-already-got-this-connected comparison above, not what
	// Link() itself is actually called with.
	outputs, err := ListOutputPorts()
	if err != nil {
		return fmt.Errorf("list output ports: %w", err)
	}
	inputs, err := ListInputPorts()
	if err != nil {
		return fmt.Errorf("list input ports: %w", err)
	}
	for _, l := range desired {
		if !have[normalizedLinkKey(l.Output, l.Input)] {
			out := resolvePortName(l.Output, outputs)
			in := resolvePortName(l.Input, inputs)
			_ = Link(out, in) // best-effort; port may not exist yet
		}
	}
	for _, l := range current {
		if !want[normalizedLinkKey(l.Output, l.Input)] {
			_ = Unlink(l.Output, l.Input) // best-effort; may already be gone
		}
	}
	return nil
}

// DisconnectUnsaved removes every live connection that isn't in the saved
// set, without changing the saved set itself, and returns how many it
// removed. A one-click cleanup for the common real-world case ReconcileLinks
// deliberately doesn't handle on its own: a fresh box where nothing has
// been saved yet, but something else (WirePlumber's default auto-linking,
// or -- as found 2026-07-04 -- a device like Stereo Tool's ALSA/JACK driver
// probing multiple device instances while its I/O is being configured)
// has already produced a pile of unwanted connections. Manually clicking
// "Remove" on each one doesn't scale; this clears all of them in one call
// so the operator can then connect and Save just the ones actually wanted.
func DisconnectUnsaved(path string) (int, error) {
	desired, err := LoadDesiredLinks(path)
	if err != nil {
		return 0, fmt.Errorf("load desired links: %w", err)
	}
	want := make(map[string]bool, len(desired))
	for _, l := range desired {
		want[normalizedLinkKey(l.Output, l.Input)] = true
	}
	current, err := ListPatchLinks()
	if err != nil {
		return 0, fmt.Errorf("list current links: %w", err)
	}
	removed := 0
	for _, l := range current {
		if want[normalizedLinkKey(l.Output, l.Input)] {
			continue
		}
		if err := Unlink(l.Output, l.Input); err == nil {
			removed++
		}
	}
	return removed, nil
}

// stereoToolPortPattern matches a port name from Stereo Tool's ALSA-JACK
// bridge, which bakes its own process ID into its client name (e.g.
// "stereo_tool.C.4853.2:in_000") -- a fresh PID every time the service
// restarts, so the exact name never repeats across restarts. Confirmed live
// 2026-07-05: comparing saved links against live links by exact name meant
// ReconcileLinks tore down the correct, .asoundrc-formed connection every
// cycle, since its PID never matched whatever was saved in patchbay.json.
var stereoToolPortPattern = regexp.MustCompile(`^(stereo_tool\.[CP])\.\d+\.(\d+:.*)$`)

// normalizePortName collapses Stereo Tool's PID segment out of a port name
// so two names that differ only by PID compare equal. Names that don't
// match the pattern (everything else) are returned unchanged.
func normalizePortName(name string) string {
	return stereoToolPortPattern.ReplaceAllString(name, "$1.*.$2")
}

// normalizedLinkKey is linkPairKey's identity for reconciliation purposes:
// it's what decides whether a saved link is "already satisfied" or a live
// link is "already saved," so it goes through normalizePortName first. The
// actual Link()/Unlink() calls still use the real, current port names --
// see resolvePortName, which is what makes that true for a saved name
// that's since gone stale (e.g. Stereo Tool restarting), not just for
// this comparison.
func normalizedLinkKey(output, input string) string {
	return normalizePortName(output) + "|" + normalizePortName(input)
}

// resolvePortName returns name unchanged if it's still live (an exact
// match in live), otherwise looks for a live port whose normalized form
// matches name's -- i.e. the same port on the same client, just under a
// new PID (Stereo Tool restarted since this name was saved). Returns
// name unchanged if nothing matches at all, so Link()/Unlink() still get
// a value and fail exactly as before for a genuinely-gone port, not a
// new error class.
func resolvePortName(name string, live []string) string {
	for _, p := range live {
		if p == name {
			return name
		}
	}
	normalized := normalizePortName(name)
	if normalized == name {
		return name // not a Stereo-Tool-shaped name; nothing to resolve
	}
	for _, p := range live {
		if normalizePortName(p) == normalized {
			return p
		}
	}
	return name
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

// ListOutputClients returns every distinct client name with at least one
// output port (the part of "client:port" before the last colon), sorted,
// deduplicated. Unlike ListOutputPorts (per-port granularity, what
// /patchbay's own point-to-point connection dropdowns need),
// BroadcastConfig.ProgramSource is a per-client concept -- the actual
// stereo pair of ports is discovered live at deploy time
// (ffmpeg_generator.go's clientPorts), whatever they happen to be named.
func ListOutputClients() ([]string, error) {
	ports, err := ListOutputPorts()
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(ports))
	var clients []string
	for _, p := range ports {
		client, _, ok := strings.Cut(p, ":")
		if !ok {
			continue
		}
		if !seen[client] {
			seen[client] = true
			clients = append(clients, client)
		}
	}
	sort.Strings(clients)
	return clients, nil
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
