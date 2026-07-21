package store

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	stDownloadBaseAMD64 = "https://download.thimeo.com/stereo_tool_gui_jack_64"
	stDownloadBaseARM64 = "https://download.thimeo.com/stereo_tool_gui_jack_pi2_64"

	// stDownloadTimeout caps the download; Stereo Tool binaries are ~10 MB.
	stDownloadTimeout = 5 * time.Minute
)

// StereoToolConfigPath is where Stereo Tool persists its own settings --
// a fixed location of its own choosing ($HOME/.stereo_tool.rc), not
// something rivapi controls, hardcoded here the same way every other
// single-user rd path in this project is.
const StereoToolConfigPath = "/home/rd/.stereo_tool.rc"

// AsoundrcPath is the user-level ALSA config override
// (conf/alsa/rd.asoundrc's install target) that Stereo Tool's "jack
// (ALSA)" device option actually reads its JACK target from -- confirmed
// 2026-07-10 live: the installed Stereo Tool build resolves this device
// through ALSA's own `type jack` PCM plugin (`libasound_module_pcm_jack.so`,
// package libasound2-plugins), which makes the real libjack calls
// internally using the port names configured here, not via
// ~/.stereo_tool.rc's "Jack ID" fields (which patchStereoToolJackIDs
// below still keeps set as a harmless secondary measure, in case a
// future build or device mode does read them, but they are not what
// actually mattered).
const AsoundrcPath = "/home/rd/.asoundrc"

// StereoToolArch returns "arm64" or "amd64" — the server's architecture,
// which determines which Thimeo binary URL to use.
func StereoToolArch() string {
	return runtime.GOARCH
}

// StereoToolDownloadURL returns the Thimeo download URL for the current
// architecture. version may be "" (latest) or a version string like "1030".
func StereoToolDownloadURL(version string) string {
	base := stDownloadBaseAMD64
	if runtime.GOARCH == "arm64" {
		base = stDownloadBaseARM64
	}
	if version == "" {
		return base
	}
	return fmt.Sprintf("%s_%s", base, version)
}

// StereoToolInstalled reports whether a file exists at installPath.
func StereoToolInstalled(installPath string) bool {
	_, err := os.Stat(installPath)
	return err == nil
}

// InstallStereoTool downloads the Stereo Tool binary from Thimeo and
// installs it to installPath. version may be "" (latest) or a version
// string like "1030". The existing binary is replaced atomically via a
// temp file + rename in the same directory, so a partial download never
// leaves a broken binary in place.
func InstallStereoTool(installPath, version string) error {
	url := StereoToolDownloadURL(version)

	client := &http.Client{Timeout: stDownloadTimeout}
	resp, err := client.Get(url) //nolint:noctx
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned HTTP %d from %s", resp.StatusCode, url)
	}

	dir := filepath.Dir(installPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("cannot create install directory %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".stereo_tool_download_*")
	if err != nil {
		return fmt.Errorf("cannot create temp file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	// Always remove the temp file; os.Remove on a path that no longer exists
	// (because Rename succeeded) is a no-op on Linux.
	defer os.Remove(tmpPath)

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		return fmt.Errorf("write failed: %w", err)
	}
	tmp.Close()

	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return fmt.Errorf("chmod failed: %w", err)
	}

	if err := os.Rename(tmpPath, installPath); err != nil {
		return fmt.Errorf("install to %s failed: %w", installPath, err)
	}
	return nil
}

// stSoundcardOutputSection is the INI section in ~/.stereo_tool.rc that
// holds the JACK auto-connect target for Stereo Tool's processed
// output. Confirmed via live behavioral testing (2026-07-10, not just
// static analysis -- see ARCHITECTURE.md's "device label" recurring-
// mistake writeup): Stereo Tool really does reach JACK through ALSA's
// own `type jack` PCM plugin, exactly as its config UI's "jack (ALSA)"
// label says. Left blank, the Jack ID fields fall through to a
// Thimeo-hardcoded default ("liquidsoap:in_0/in_1") that no longer
// exists now that Liquidsoap has been replaced by per-stream ffmpeg
// processes.
const stSoundcardOutputSection = "[Soundcard - Normal output]"

// stSoundcardInputSection is the matching INI section for Stereo
// Tool's own input device -- asymmetrically named in Stereo Tool's own
// file format ("Input", not "Normal input") to stSoundcardOutputSection's
// "Normal output".
const stSoundcardInputSection = "[Soundcard - Input]"

// stJackDeviceID is the exact "Device ID=" value Stereo Tool's own INI
// format uses to mean "route through ALSA's jack (ALSA) plugin" --
// confirmed against a real installed config's own dropdown-populated
// value, not guessed.
const stJackDeviceID = "jack (ALSA)"

const (
	stBootstrapTimeout      = 15 * time.Second
	stBootstrapPollInterval = 200 * time.Millisecond
	stBootstrapKillGrace    = 3 * time.Second
)

// ConfigureStereoToolJack ensures configPath's [Soundcard - Normal output]
// Jack ID 1/Jack ID 2 point at sinkPortL/sinkPortR instead of Stereo
// Tool's own dead default, and that both the input and output sections'
// own "Device ID=" actually selects the jack (ALSA) device -- not just
// the Jack ID fields, which have no effect unless the device itself is
// set to route through JACK in the first place (see
// patchStereoToolDeviceIDs). If configPath doesn't exist yet or has no
// [Soundcard - Normal output] section (a genuinely fresh install --
// Stereo Tool has never run), it bootstraps one via a brief --no-gui
// launch first.
func ConfigureStereoToolJack(installPath, configPath string, webPort int, sinkPortL, sinkPortR string) error {
	if !hasStereoToolOutputSection(configPath) {
		if err := bootstrapStereoToolConfig(installPath, configPath, webPort); err != nil {
			return fmt.Errorf("bootstrapping stereo tool config: %w", err)
		}
	}
	if _, err := patchStereoToolDeviceIDs(configPath); err != nil {
		return fmt.Errorf("patching device IDs: %w", err)
	}
	return patchStereoToolJackIDs(configPath, sinkPortL, sinkPortR)
}

// ReconcileStereoToolDeviceIDs re-checks StereoToolConfigPath's Device
// ID fields and restarts stereo-tool.service if either one needed
// correcting. Called periodically (see rivapi/main.go's reconcile
// loop), not just from ConfigureStereoToolJack's one-time install/
// configure flow -- confirmed live 2026-07-21 that the one-time flow
// alone leaves a real gap: rebuilding/upgrading the .deb never
// re-triggers ConfigureStereoToolJack on its own (Stereo Tool isn't
// part of the package at all; it's only ever installed/configured via
// the dashboard's "Install Stereo Tool" button), so a box that already
// has Stereo Tool configured before this fix existed would otherwise
// need someone to notice and manually re-run that install step. No-ops
// entirely if Stereo Tool has never been installed at all (no config
// file yet to patch).
func ReconcileStereoToolDeviceIDs() error {
	if _, err := os.Stat(StereoToolConfigPath); os.IsNotExist(err) {
		return nil
	}
	changed, err := patchStereoToolDeviceIDs(StereoToolConfigPath)
	if err != nil {
		return fmt.Errorf("patching stereo tool device IDs: %w", err)
	}
	if !changed {
		return nil
	}
	if err := sudoSystemctl("restart", "stereo-tool.service"); err != nil {
		return fmt.Errorf("restarting stereo-tool.service: %w", err)
	}
	return nil
}

func hasStereoToolOutputSection(configPath string) bool {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), stSoundcardOutputSection)
}

// bootstrapStereoToolConfig launches installPath once, headless, just
// long enough for Stereo Tool to write its own default ~/.stereo_tool.rc
// (including [Soundcard - Normal output], with blank Jack IDs), then
// terminates it. This specific failure mode never exits on its own
// (confirmed live: a manual run loops its own internal JACK
// connect-retry forever without ever returning), so termination is
// unconditional once the config file is ready or the timeout is hit --
// not conditional on the process exiting cleanly.
func bootstrapStereoToolConfig(installPath, configPath string, webPort int) error {
	cmd := exec.Command(installPath, "--no-gui", "-p", strconv.Itoa(webPort))
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launching %s: %w", installPath, err)
	}

	deadline := time.Now().Add(stBootstrapTimeout)
	ready := false
	for time.Now().Before(deadline) {
		if hasStereoToolOutputSection(configPath) {
			ready = true
			break
		}
		time.Sleep(stBootstrapPollInterval)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	_ = cmd.Process.Signal(os.Interrupt)
	select {
	case <-done:
	case <-time.After(stBootstrapKillGrace):
		_ = cmd.Process.Kill()
		<-done
	}

	if !ready {
		return fmt.Errorf("stereo tool did not generate a config file with %s within %s",
			stSoundcardOutputSection, stBootstrapTimeout)
	}
	return nil
}

// patchStereoToolDeviceIDs forces both [Soundcard - Input] and
// [Soundcard - Normal output]'s own "Device ID=" to stJackDeviceID.
// Confirmed live 2026-07-21: unlike the Jack ID fields below (always
// blank until something sets them), "Device ID=" is never blank after
// Stereo Tool's own bootstrap -- it defaults to whatever real ALSA
// hardware it happens to detect (e.g. "HDA Intel: Generic Analog
// (hw:0,0) (ALSA)"), which nothing in this codebase ever overwrote
// before now. The input section's own default happened to already be
// correct on the box this was found on, which is exactly what let the
// gap go unnoticed: the output section's own separate, always-wrong
// default was masked by the input section coincidentally being right.
// Both are set unconditionally (not just when blank, the pattern
// patchStereoToolJackIDs uses) since a real hardware device name is
// never something this fork's own architecture wants preserved as an
// operator's deliberate choice -- routing through jack (ALSA) is a
// hard requirement here, not a default that's merely convenient.
// Returns whether anything actually changed, so the caller only logs a
// restart-worthy change when one genuinely happened.
func patchStereoToolDeviceIDs(configPath string) (bool, error) {
	info, err := os.Stat(configPath)
	if err != nil {
		return false, fmt.Errorf("stat %s: %w", configPath, err)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return false, fmt.Errorf("reading %s: %w", configPath, err)
	}

	lines := strings.Split(string(data), "\n")
	inTargetSection := false
	changed := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inTargetSection = trimmed == stSoundcardInputSection || trimmed == stSoundcardOutputSection
			continue
		}
		if !inTargetSection {
			continue
		}
		if strings.HasPrefix(trimmed, "Device ID=") && trimmed != "Device ID="+stJackDeviceID {
			lines[i] = "Device ID=" + stJackDeviceID
			changed = true
		}
	}
	if !changed {
		return false, nil
	}

	tmp := configPath + ".tmp"
	if err := os.WriteFile(tmp, []byte(strings.Join(lines, "\n")), info.Mode()); err != nil {
		return false, fmt.Errorf("writing %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, configPath); err != nil {
		return false, err
	}
	return true, nil
}

// patchStereoToolJackIDs sets Jack ID 1/Jack ID 2 within
// [Soundcard - Normal output] specifically -- the same two field names
// also appear, blank, in unrelated disabled sections elsewhere in this
// file (an AES67/MPX output, a "Low latency output" ALSA-hardware
// profile), so the edit is scoped by tracking the current INI section
// while scanning, not a blind find-and-replace. Only blank fields
// (exactly "Jack ID 1=" with nothing after) are set; anything already
// configured is an operator's own choice and is left alone.
func patchStereoToolJackIDs(configPath, sinkPortL, sinkPortR string) error {
	info, err := os.Stat(configPath)
	if err != nil {
		return fmt.Errorf("stat %s: %w", configPath, err)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", configPath, err)
	}

	lines := strings.Split(string(data), "\n")
	inSection := false
	changed := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inSection = trimmed == stSoundcardOutputSection
			continue
		}
		if !inSection {
			continue
		}
		switch trimmed {
		case "Jack ID 1=":
			lines[i] = "Jack ID 1=" + sinkPortL
			changed = true
		case "Jack ID 2=":
			lines[i] = "Jack ID 2=" + sinkPortR
			changed = true
		}
	}
	if !changed {
		return nil
	}

	tmp := configPath + ".tmp"
	if err := os.WriteFile(tmp, []byte(strings.Join(lines, "\n")), info.Mode()); err != nil {
		return fmt.Errorf("writing %s: %w", tmp, err)
	}
	return os.Rename(tmp, configPath)
}

// asoundrcPlaybackPortRe matches one "playback_ports" entry line inside
// asoundrc's pcm.jack block, e.g. "		0 rivendell_0:playout_0L" -- index
// (0 or 1) captured separately from whatever target port name currently
// follows it.
var asoundrcPlaybackPortRe = regexp.MustCompile(`^(\s*)([01])\s+\S+\s*$`)

// PatchAsoundrcTarget points AsoundrcPath's pcm.jack playback_ports at
// leftPort/rightPort (full "client:port" names), leaving every other
// line -- including capture_ports -- untouched. Returns whether the
// file actually changed, so the caller only needs to restart Stereo
// Tool when the target genuinely moved (e.g. a new station's first
// stream became the deploy's anchor), not on every deploy.
//
// Scoped to the "playback_ports {" block specifically: asoundrc has
// exactly one pcm.jack definition (see conf/alsa/rd.asoundrc), so unlike
// patchStereoToolJackIDs above there's no risk of matching an unrelated
// section, but the block boundary is still tracked explicitly rather
// than assuming the two port lines always immediately follow the
// opening brace, in case the file gains more fields later.
func PatchAsoundrcTarget(asoundrcPath, leftPort, rightPort string) (bool, error) {
	info, err := os.Stat(asoundrcPath)
	if err != nil {
		return false, fmt.Errorf("stat %s: %w", asoundrcPath, err)
	}
	data, err := os.ReadFile(asoundrcPath)
	if err != nil {
		return false, fmt.Errorf("reading %s: %w", asoundrcPath, err)
	}

	targets := map[string]string{"0": leftPort, "1": rightPort}
	lines := strings.Split(string(data), "\n")
	inBlock := false
	changed := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "playback_ports {" {
			inBlock = true
			continue
		}
		if inBlock && trimmed == "}" {
			inBlock = false
			continue
		}
		if !inBlock {
			continue
		}
		m := asoundrcPlaybackPortRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		indent, index := m[1], m[2]
		newLine := indent + index + " " + targets[index]
		if newLine != line {
			lines[i] = newLine
			changed = true
		}
	}
	if !changed {
		return false, nil
	}

	tmp := asoundrcPath + ".tmp"
	if err := os.WriteFile(tmp, []byte(strings.Join(lines, "\n")), info.Mode()); err != nil {
		return false, fmt.Errorf("writing %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, asoundrcPath); err != nil {
		return false, err
	}
	return true, nil
}
