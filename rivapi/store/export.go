package store

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ExportBundle is everything the dashboard itself configures, plus Stereo
// Tool's own state (which lives outside this project's own JSON stores),
// bundled into one importable/exportable artifact for migration or
// disaster recovery. Contains real secrets (Icecast source/relay/admin
// passwords already in Broadcast, and a remote MySQL password if Mode is
// client) -- treat an exported file the same as any other credentials
// file, not something to share casually.
type ExportBundle struct {
	ExportedAt time.Time       `json:"exported_at"`
	Broadcast  BroadcastConfig `json:"broadcast"`
	Patchbay   []PatchLink     `json:"patchbay"`
	Mode       ModeConfig      `json:"mode"`
	Tasks      []ScheduledTask `json:"tasks"`

	// StereoToolRC is ~/.stereo_tool.rc's raw content, base64-encoded --
	// Stereo Tool's own device/routing settings and active-preset
	// reference (see docs/specs/0010-systemd-stack-orchestration.md).
	// Omitted entirely if the file doesn't exist on this box.
	StereoToolRC string `json:"stereo_tool_rc,omitempty"`

	// StereoToolPresets maps each file under ~/.stereo_tool.presets/ to
	// its own base64-encoded content -- these are Stereo Tool's own
	// binary/text .sts preset format, not JSON-native, so they travel as
	// opaque blobs inside this JSON envelope rather than being parsed.
	StereoToolPresets map[string]string `json:"stereo_tool_presets,omitempty"`
}

const stereoToolRCPath = "/home/rd/.stereo_tool.rc"
const stereoToolPresetsDir = "/home/rd/.stereo_tool.presets"

// BuildExportBundle gathers the current state of everything this project's
// dashboard configures, plus whatever Stereo Tool state exists on this box.
// broadcastConfigPath is passed in (rather than a package constant, unlike
// DesiredLinksPath/ModeConfigPath/TasksConfigPath) since it's the one path
// here that's actually configurable via config.Config.BroadcastConfigPath,
// which this package can't import without a cycle. Every piece is
// best-effort/optional except Broadcast/Patchbay/Mode/Tasks' own
// zero-value-if-missing loaders -- a box that's never touched Stereo Tool
// at all still produces a valid (partial) bundle.
func BuildExportBundle(broadcastConfigPath string) (ExportBundle, error) {
	b := ExportBundle{ExportedAt: time.Now()}

	broadcast, err := LoadBroadcastConfig(broadcastConfigPath)
	if err != nil {
		return b, fmt.Errorf("loading broadcast config: %w", err)
	}
	b.Broadcast = broadcast

	patchbay, err := LoadDesiredLinks(DesiredLinksPath)
	if err != nil {
		return b, fmt.Errorf("loading patchbay config: %w", err)
	}
	b.Patchbay = patchbay

	mode, err := LoadModeConfig(ModeConfigPath)
	if err != nil {
		return b, fmt.Errorf("loading mode config: %w", err)
	}
	b.Mode = mode

	tasks, err := LoadTasks(TasksConfigPath)
	if err != nil {
		return b, fmt.Errorf("loading tasks config: %w", err)
	}
	b.Tasks = tasks

	if data, err := os.ReadFile(stereoToolRCPath); err == nil {
		b.StereoToolRC = base64.StdEncoding.EncodeToString(data)
	}

	if entries, err := os.ReadDir(stereoToolPresetsDir); err == nil {
		presets := make(map[string]string, len(entries))
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			data, err := os.ReadFile(filepath.Join(stereoToolPresetsDir, e.Name()))
			if err != nil {
				continue // best-effort: skip a preset we can't read rather than failing the whole export
			}
			presets[e.Name()] = base64.StdEncoding.EncodeToString(data)
		}
		if len(presets) > 0 {
			b.StereoToolPresets = presets
		}
	}

	return b, nil
}

// ApplyImportBundle restores b's contents. Returns a step-by-step log, same
// pattern as ApplyMode/BroadcastSave.
//
// Deliberately does NOT call ApplyMode automatically, even though b.Mode is
// restored to mode.json -- re-running mode-switching (mounting NFS,
// restarting mariadb, changing bind-address) is a much bigger, more
// surprising side effect than restoring a config file, especially on a
// disaster-recovery box where the operator may want to review the restored
// values before anything privileged actually runs. The Mode page picks up
// the restored file the next time it's loaded; the operator clicks Apply
// there deliberately.
func ApplyImportBundle(b ExportBundle, broadcastConfigPath string) ([]string, error) {
	var steps []string
	step := func(s string) { steps = append(steps, s) }

	if err := SaveBroadcastConfig(b.Broadcast, broadcastConfigPath); err != nil {
		return steps, fmt.Errorf("restoring broadcast config: %w", err)
	}
	step("Broadcast config restored.")

	if err := GenerateIcecastXML(b.Broadcast); err != nil {
		return steps, fmt.Errorf("generating icecast.xml: %w", err)
	}
	step("icecast.xml regenerated.")

	if err := GenerateLiquidsoapScript(b.Broadcast); err != nil {
		return steps, fmt.Errorf("generating radio.liq: %w", err)
	}
	step("radio.liq regenerated.")

	if err := sudoSystemctl("restart", "icecast2.service"); err != nil {
		return steps, fmt.Errorf("restarting icecast2: %w", err)
	}
	step("icecast2 restarted.")

	if err := sudoSystemctl("restart", "liquidsoap.service"); err != nil {
		// Same tolerance as BroadcastSave: liquidsoap may not be installed
		// yet on a box mid-restore, that's not a fatal error here either.
		if !isExitCode(err, 5) {
			return steps, fmt.Errorf("restarting liquidsoap: %w", err)
		}
		step("liquidsoap not installed yet — radio.liq is ready for when it is.")
	} else {
		step("liquidsoap restarted.")
	}

	if err := SaveDesiredLinks(b.Patchbay, DesiredLinksPath); err != nil {
		return steps, fmt.Errorf("restoring patchbay config: %w", err)
	}
	step("Patchbay config restored (applied automatically within 30s by the background reconciler).")
	if err := ReconcileLinks(DesiredLinksPath); err != nil {
		// Best-effort immediate reconcile for faster feedback; the
		// background ticker will retry regardless if this fails (e.g. a
		// port that doesn't exist yet because its client hasn't started).
		step(fmt.Sprintf("Immediate patchbay reconcile reported: %v (will retry automatically).", err))
	} else {
		step("Patchbay reconciled immediately.")
	}

	if err := SaveModeConfig(b.Mode, ModeConfigPath); err != nil {
		return steps, fmt.Errorf("restoring mode config: %w", err)
	}
	step(fmt.Sprintf("Mode config restored as %q — visit Mode and click Apply to actually switch the box over to it.", b.Mode.Mode))

	if err := SaveTasks(b.Tasks, TasksConfigPath); err != nil {
		return steps, fmt.Errorf("restoring tasks config: %w", err)
	}
	step(fmt.Sprintf("%d scheduled task(s) restored.", len(b.Tasks)))
	for _, t := range b.Tasks {
		if err := DeployTask(t); err != nil {
			step(fmt.Sprintf("Task %q failed to deploy: %v", t.Name, err))
			continue
		}
		step(fmt.Sprintf("Task %q redeployed.", t.Name))
	}

	if b.StereoToolRC != "" {
		data, err := base64.StdEncoding.DecodeString(b.StereoToolRC)
		if err != nil {
			return steps, fmt.Errorf("decoding stereo_tool.rc: %w", err)
		}
		if err := os.WriteFile(stereoToolRCPath, data, 0644); err != nil {
			return steps, fmt.Errorf("restoring stereo_tool.rc: %w", err)
		}
		step("Stereo Tool's .stereo_tool.rc restored.")
	}

	if len(b.StereoToolPresets) > 0 {
		if err := os.MkdirAll(stereoToolPresetsDir, 0700); err != nil {
			return steps, fmt.Errorf("creating stereo tool presets directory: %w", err)
		}
		restored := 0
		for name, encoded := range b.StereoToolPresets {
			data, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				step(fmt.Sprintf("Preset %q skipped: %v", name, err))
				continue
			}
			if err := os.WriteFile(filepath.Join(stereoToolPresetsDir, name), data, 0644); err != nil {
				step(fmt.Sprintf("Preset %q failed to write: %v", name, err))
				continue
			}
			restored++
		}
		step(fmt.Sprintf("%d Stereo Tool preset(s) restored.", restored))
	}

	return steps, nil
}
