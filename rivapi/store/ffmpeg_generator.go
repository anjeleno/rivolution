package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ffmpeg_generator.go replaces liquidsoap_generator.go: instead of one
// radio.liq script driving every output.icecast() from a single
// Liquidsoap process, each StreamConfig becomes its own long-running
// ffmpeg process (JACK capture -> encode -> Icecast push), one systemd
// service per stream. See patches/liquidsoap/README.md and
// docs/handoff/2026-07-09.md for why: a real, reproducible upstream bug
// in Liquidsoap's input.jack (src/core/optionals/bjack/bjack_in.ml)
// pegs a CPU core and crashes under sustained load specifically against
// pipewire-jack, unrelated to this project's own config. ffmpeg's JACK
// input is native (written directly against libjack, not a third-party
// binding) and far more widely used against pipewire-jack in practice.
//
// BroadcastConfig/StreamConfig themselves are untouched -- they were
// already 100% encoder-agnostic (station identity, per-stream codec/
// bitrate/mount). Only the generator changes.
//
// Reuses tasks_deploy.go's existing control scripts (install-unit.sh/
// remove-unit.sh/task-systemctl.sh) rather than inventing a parallel
// deployment mechanism -- they're already generic (install/remove/
// enable a systemd unit by validated 16-hex ID), just never previously
// used for a "service" with no paired ".timer": streams are always-on,
// not periodic, so no timer is generated for them.

// FfmpegStreamsManifestPath persists which stream unit IDs are
// currently deployed, so a later Save & Deploy can remove units for
// streams no longer in the config (e.g. a mount deleted from the
// dashboard) -- same problem tasks.go solves for scheduled tasks, but
// streams don't have an operator-visible ID to track by, so this
// tracks them internally instead, keyed by mount.
const FfmpegStreamsManifestPath = "/home/rd/etc/rivolution/ffmpeg_streams.json"

// ProgramSourceStereoTool is BroadcastConfig.ProgramSource's sentinel
// value for "a local Stereo Tool processes the signal" -- as opposed to
// an ordinary value naming a specific JACK client directly (e.g.
// "rivendell_0" on a station with no local processing at all). Stereo
// Tool needs special handling no other source does: its own JACK client
// name embeds its process ID (see stereoToolOutputPorts), and its own
// ALSA-JACK bridge can only ever be configured with one fixed target
// (see syncStereoToolTarget) -- there is no way to hand it N target
// names for N streams the way a native multi-port JACK client could
// simply be told to connect to. Neither limitation is specific to this
// station's setup; every install using Stereo Tool hits both.
const ProgramSourceStereoTool = "stereo_tool"

// stereoToolOutputPortRe matches Stereo Tool's own JACK client's output
// (playback) port names, e.g. "stereo_tool.P.12898.3:out_000" -- see
// patchbay.go's stereoToolPortPattern, which exists for the identical
// reason (matching this project's one confirmed real case of a JACK
// client whose own name is never stable across restarts).
var stereoToolOutputPortRe = regexp.MustCompile(`^stereo_tool\.P\.\d+\.\d+:`)

// stereoToolOutputPorts returns Stereo Tool's currently live output
// ports, however its current process/client-instance happens to be
// numbered right now -- a literal "client:port" prefix match (what
// clientPorts does for every other kind of source) can never work here,
// since that name changes on every restart and even within one restart
// while it settles. Sorted for a stable L/R pairing order. Polls briefly
// (same shape as pollClientPorts): syncStereoToolTarget may have just
// restarted stereo-tool.service moments earlier, so its ports may not
// have registered yet.
func stereoToolOutputPorts() ([]string, error) {
	deadline := time.Now().Add(streamPortsPollTimeout)
	for {
		all, err := ListOutputPorts()
		if err != nil {
			return nil, err
		}
		var ports []string
		for _, p := range all {
			if stereoToolOutputPortRe.MatchString(p) {
				ports = append(ports, p)
			}
		}
		if len(ports) > 0 || time.Now().After(deadline) {
			sort.Strings(ports)
			return ports, nil
		}
		time.Sleep(streamPortsPollInterval)
	}
}

// primaryStreamMount deterministically picks which configured stream is
// Stereo Tool's own fixed ALSA-JACK bridge target (see
// syncStereoToolTarget) -- sorted by mount, so it's derived purely from
// whatever an operator has actually configured on /broadcast, never a
// hardcoded mount name. Returns false if no streams are configured yet.
func primaryStreamMount(cfg BroadcastConfig) (string, bool) {
	if len(cfg.Streams) == 0 {
		return "", false
	}
	mounts := make([]string, len(cfg.Streams))
	for i, s := range cfg.Streams {
		mounts[i] = s.Mount
	}
	sort.Strings(mounts)
	return mounts[0], true
}

// syncStereoToolTarget points Stereo Tool's ALSA-JACK bridge
// (AsoundrcPath's pcm.jack playback_ports, confirmed 2026-07-10 to be
// what it actually reads its target from -- see AsoundrcPath's doc
// comment) at the current primaryStreamMount's ffmpeg JACK client,
// restarting stereo-tool.service only when the target actually changed
// (a fresh station, or the previous anchor stream was removed) -- not
// on every deploy, since Stereo Tool's own connection is otherwise left
// alone once established. No-ops entirely if ProgramSource isn't the
// Stereo Tool sentinel, or no streams are configured yet.
func syncStereoToolTarget(cfg BroadcastConfig) error {
	if cfg.ProgramSource != ProgramSourceStereoTool {
		return nil
	}
	mount, ok := primaryStreamMount(cfg)
	if !ok {
		return nil
	}
	clientID := streamJackClientID(cfg.Liquidsoap.JackInputID, mount)
	changed, err := PatchAsoundrcTarget(AsoundrcPath, clientID+":input_1", clientID+":input_2")
	if err != nil {
		return fmt.Errorf("patching %s: %w", AsoundrcPath, err)
	}
	if !changed {
		return nil
	}
	if err := sudoSystemctl("restart", "stereo-tool.service"); err != nil {
		return fmt.Errorf("restarting stereo-tool.service: %w", err)
	}
	return nil
}

// StreamUnitID deterministically derives a task-shaped 16-hex-char ID
// from a stream's mount, so redeploying the same set of streams reuses
// the same unit names instead of creating new ones every save (and so
// a removed mount's old unit can be found and torn down later).
func StreamUnitID(mount string) string {
	sum := sha256.Sum256([]byte("ffmpeg-stream:" + mount))
	return hex.EncodeToString(sum[:])[:16]
}

// jackClientNameRe matches characters JACK/PipeWire client names are
// safe with -- alphanumeric, hyphen, underscore. Anything else in a
// mount (e.g. "/192", "/stream one") gets replaced, since each stream
// needs its own distinct JACK client name to independently connect to
// Stereo Tool's output (JACK ports support multiple simultaneous
// readers, so N ffmpeg clients each connecting to the same source
// ports is normal, not a conflict).
var jackClientNameRe = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

// baseID is the dashboard's configured base JACK client identity (the
// "JACK input ID" field, cfg.Liquidsoap.JackInputID -- name kept from
// the Liquidsoap era to avoid a config migration, see this file's
// top-of-file comment). A single client name can't be shared across N
// independent ffmpeg processes, so each stream's mount is appended to
// keep every client name distinct while still honoring what the
// operator configured as the station's base identity.
func streamJackClientID(baseID, mount string) string {
	if baseID == "" {
		baseID = "rivolution"
	}
	sanitized := jackClientNameRe.ReplaceAllString(strings.Trim(mount, "/"), "-")
	if sanitized == "" {
		sanitized = "stream"
	}
	return baseID + "-" + sanitized
}

// ffmpegPipeline returns the full shell pipeline for one stream's
// systemd ExecStart=. MP3/Vorbis go straight from ffmpeg's JACK input
// to its own Icecast output in one process. AAC keeps the same
// external-fdkaac-via-pipe shape radio.liq's %external encoder already
// used (see liquidsoap_generator.go's liqEncoder comment) -- ffmpeg's
// own AAC encoder is worse than fdkaac (distro ffmpeg builds don't
// link libfdk-aac, a licensing restriction, not a bug), and this is
// the identical "pipe raw PCM to an external encoder" pattern already
// proven in production, just with ffmpeg producing and consuming the
// pipe instead of Liquidsoap.
func ffmpegPipeline(cfg BroadcastConfig, s StreamConfig) (string, error) {
	clientID := streamJackClientID(cfg.Liquidsoap.JackInputID, s.Mount)
	rate := cfg.Liquidsoap.SampleRate
	name := s.EffectiveName(cfg.Station)
	genre := s.EffectiveGenre(cfg.Station)
	description := s.EffectiveDescription(cfg.Station)

	switch s.Codec {
	case "mp3":
		// Trailing -f mp3: ffmpeg's icecast:// output protocol has no
		// filename extension to infer a container from, and without an
		// explicit -f it fails immediately with "Unable to choose an
		// output format" -- confirmed live 2026-07-10, every codec path
		// hit this identically until each got its own explicit -f.
		return fmt.Sprintf(
			"exec ffmpeg -nostdin -loglevel warning -f jack -i %s "+
				"-c:a libmp3lame -b:a %dk -compression_level %d -content_type audio/mpeg "+
				"-ice_name %s -ice_genre %s -ice_description %s "+
				"-f mp3 %s",
			shellQuote(clientID), s.Bitrate, s.Mp3Quality,
			shellQuote(name), shellQuote(genre), shellQuote(description),
			shellQuote(icecastURL(cfg, s)),
		), nil
	case "ogg":
		return fmt.Sprintf(
			"exec ffmpeg -nostdin -loglevel warning -f jack -i %s "+
				"-c:a libvorbis -q:a %s -content_type application/ogg "+
				"-ice_name %s -ice_genre %s -ice_description %s "+
				"-f ogg %s",
			shellQuote(clientID), ffmpegVorbisQuality(s.Quality),
			shellQuote(name), shellQuote(genre), shellQuote(description),
			shellQuote(icecastURL(cfg, s)),
		), nil
	case "he-aac-v1", "he-aac-v2":
		// -f 2 (ADTS) matches radio.liq's own comment: fdkaac's default
		// M4A container needs to seek back and write its moov box,
		// which a streaming pipe can't do -- ADTS is a continuous
		// streamable bitstream instead. --profile 2 is plain AAC-LC;
		// see liquidsoap_generator.go's liqEncoder comment for why
		// HE-AAC (profiles 5/29) isn't available on this distro's
		// libfdk-aac2 build. Final stage's trailing -f adts: same
		// explicit-format requirement as mp3/ogg above -- the input
		// side is already ADTS (fdkaac's -f 2), so the output side
		// names the same container.
		return fmt.Sprintf(
			"exec ffmpeg -nostdin -loglevel warning -f jack -i %s "+
				"-f s16le -ar %d -ac 2 - | "+
				"fdkaac --silent --bitrate %d000 --profile 2 --raw --raw-channels 2 --raw-rate %d -f 2 -o - - | "+
				"exec ffmpeg -nostdin -loglevel warning -f aac -i - -c:a copy "+
				"-content_type audio/aacp -ice_name %s -ice_genre %s -ice_description %s "+
				"-f adts %s",
			shellQuote(clientID), rate,
			s.Bitrate, rate,
			shellQuote(name), shellQuote(genre), shellQuote(description),
			shellQuote(icecastURL(cfg, s)),
		), nil
	default:
		return "", fmt.Errorf("unknown stream codec %q for mount %q", s.Codec, s.Mount)
	}
}

// icecastURL builds the icecast:// destination URL. "source" is
// Icecast's fixed source-client username; only the password is
// operator-configured.
func icecastURL(cfg BroadcastConfig, s StreamConfig) string {
	return fmt.Sprintf("icecast://source:%s@%s:%d%s",
		cfg.Icecast.SourcePassword, cfg.Liquidsoap.IcecastHost, cfg.Liquidsoap.IcecastPort, s.Mount)
}

// ffmpegVorbisQuality converts Liquidsoap's %vorbis quality scale
// (-0.2 lowest .. 1.0 highest, the range the dashboard's form already
// collects) to ffmpeg's libvorbis -q:a scale (-1 lowest .. 10 highest)
// via a straight linear rescale, rounded to 1 decimal place (-q:a's
// own granularity). Approximate -- the two encoders' quality curves
// aren't numerically identical -- but keeps the dashboard's existing
// field and its stored values meaningful without a migration.
func ffmpegVorbisQuality(liqQuality float64) string {
	const liqMin, liqMax = -0.2, 1.0
	const ffMin, ffMax = -1.0, 10.0
	q := ffMin + (liqQuality-liqMin)/(liqMax-liqMin)*(ffMax-ffMin)
	if q < ffMin {
		q = ffMin
	}
	if q > ffMax {
		q = ffMax
	}
	return fmt.Sprintf("%.1f", q)
}

// DeployFfmpegStreams (re)generates and installs one systemd service
// per Streams entry (always-on, no timer -- Type=simple with
// Restart=on-failure, matching how liquidsoap.service ran), and tears
// down units for any previously-deployed stream no longer present in
// cfg (tracked via FfmpegStreamsManifestPath, keyed by mount since
// streams have no other stable identity).
func DeployFfmpegStreams(cfg BroadcastConfig) error {
	if err := ensureControlScripts(); err != nil {
		return fmt.Errorf("deploying control scripts: %w", err)
	}

	previous, err := loadFfmpegStreamsManifest(FfmpegStreamsManifestPath)
	if err != nil {
		return fmt.Errorf("loading previous streams manifest: %w", err)
	}

	// systemd's StandardOutput=append:<path> creates the file itself if
	// missing, but not its parent directory -- ensure it exists up front
	// so a fresh box's first deploy doesn't fail to start every stream.
	if cfg.Liquidsoap.LogPath != "" {
		if err := os.MkdirAll(filepath.Dir(cfg.Liquidsoap.LogPath), 0755); err != nil {
			return fmt.Errorf("creating log directory: %w", err)
		}
	}

	current := make(map[string]string, len(cfg.Streams)) // mount -> unit ID
	for _, s := range cfg.Streams {
		id := StreamUnitID(s.Mount)
		current[s.Mount] = id

		pipeline, err := ffmpegPipeline(cfg, s)
		if err != nil {
			return err
		}

		// Every stream's own clean output is always in
		// `journalctl -u rivolution-task-<id>.service`. LogPath (the
		// dashboard's "Log path" field, held over from the single-process
		// Liquidsoap era) is additionally honored here as a shared
		// append-log across all streams, for operators used to tailing
		// one file -- systemd opens it O_APPEND, so concurrent writers
		// from different streams can't corrupt each other, just
		// interleave line-by-line.
		logDirectives := ""
		if cfg.Liquidsoap.LogPath != "" {
			logDirectives = fmt.Sprintf("StandardOutput=append:%s\nStandardError=append:%s\n",
				cfg.Liquidsoap.LogPath, cfg.Liquidsoap.LogPath)
		}

		serviceContent := fmt.Sprintf(`# Generated by rivapi's Broadcast page for stream %q -- do not hand-edit,
# it will be overwritten the next time the broadcast config is saved.
[Unit]
Description=Rivolution broadcast stream: %s
After=rivolution-stack.target
PartOf=rivolution-stack.target

[Service]
Type=simple
User=rd
Group=rd
Environment=XDG_RUNTIME_DIR=/run/pipewire-system
ExecStart=/bin/sh -c %s
%sRestart=on-failure
RestartSec=5

[Install]
WantedBy=rivolution-stack.target
`, s.Mount, s.Mount, shellQuote(pipeline), logDirectives)

		staged, err := writeStaged("stream-"+id+".service", serviceContent)
		if err != nil {
			return err
		}
		controlScript := controlScriptsDir + "/install-unit.sh"
		if err := sudoRun(controlScript, staged, id, "service"); err != nil {
			return fmt.Errorf("installing unit for stream %q: %w", s.Mount, err)
		}
	}

	if err := sudoSystemctl("daemon-reload"); err != nil {
		return err
	}

	taskCtl := controlScriptsDir + "/task-systemctl.sh"
	for _, s := range cfg.Streams {
		id := current[s.Mount]
		// "enable-service", not "enable" -- streams are always-on Type=simple
		// services with no paired .timer (see this file's top-of-file
		// comment), and task-systemctl.sh's plain "enable" action only ever
		// targets a task's .timer.
		if err := sudoRun(taskCtl, "enable-service", id); err != nil {
			return fmt.Errorf("enabling stream %q: %w", s.Mount, err)
		}
	}

	// Remove units for mounts that were deployed before but aren't in
	// the new config.
	for mount, id := range previous {
		if _, stillPresent := current[mount]; stillPresent {
			continue
		}
		if err := sudoRun(controlScriptsDir+"/remove-unit.sh", id); err != nil {
			return fmt.Errorf("removing stale stream %q: %w", mount, err)
		}
	}
	if len(previous) != len(current) {
		if err := sudoSystemctl("daemon-reload"); err != nil {
			return err
		}
	}

	if err := syncStereoToolTarget(cfg); err != nil {
		return fmt.Errorf("syncing stereo tool target: %w", err)
	}

	if err := syncStreamPatchLinks(cfg, previous, current); err != nil {
		return fmt.Errorf("syncing stream patch links: %w", err)
	}

	return saveFfmpegStreamsManifest(current, FfmpegStreamsManifestPath)
}

const (
	streamPortsPollTimeout  = 3 * time.Second
	streamPortsPollInterval = 200 * time.Millisecond
)

// syncStreamPatchLinks keeps the patchbay's saved connections from
// cfg.ProgramSource to each broadcast stream in sync with cfg.Streams --
// but only for a stream with no saved link at all yet. A stream that
// already has *any* saved link referencing its client (whether this
// function created it or an operator drew it by hand in /patchbay) is
// never touched again, so a manual repatch or a deliberate disconnect
// always sticks. Deliberately keyed on "does a link exist," not "is
// this mount new in the deploy manifest" -- a stream that was deployed
// before but never actually got a working connection (e.g. its process
// kept failing to start) still needs auto-connecting once it finally
// comes up, even though its mount already appears in `previous`. See
// BroadcastConfig.ProgramSource's doc comment for why no source is
// assumed anywhere in this package.
//
// Deliberately discovers real port names live (via ListOutputPorts/
// ListInputPorts) rather than assuming a fixed naming convention on
// either side -- cfg.ProgramSource might be a PipeWire virtual bus, a
// native JACK client, or anything else an operator picked, and
// different clients name their ports differently.
func syncStreamPatchLinks(cfg BroadcastConfig, previous, current map[string]string) error {
	desired, err := LoadDesiredLinks(DesiredLinksPath)
	if err != nil {
		return fmt.Errorf("loading patchbay links: %w", err)
	}

	// Removals: a mount that existed before but not now has a genuinely
	// gone stream client -- any saved link naming it is stale, whether
	// it was auto-created here or an operator's own change.
	removedClients := make(map[string]bool)
	for mount := range previous {
		if _, stillPresent := current[mount]; !stillPresent {
			removedClients[streamJackClientID(cfg.Liquidsoap.JackInputID, mount)] = true
		}
	}
	kept := make([]PatchLink, 0, len(desired))
	for _, l := range desired {
		stale := false
		for client := range removedClients {
			if strings.HasPrefix(l.Input, client+":") {
				stale = true
				break
			}
		}
		if !stale {
			kept = append(kept, l)
		}
	}
	desired = kept

	// Additions: only for a stream with no saved link at all yet, and
	// only if an operator has actually configured a program source.
	if cfg.ProgramSource != "" {
		var sourcePorts []string
		var err error
		if cfg.ProgramSource == ProgramSourceStereoTool {
			// Stereo Tool's own JACK client name embeds its PID, so it
			// can never be matched by a literal "client:port" prefix the
			// way any other source can -- discover whatever's currently
			// live by pattern instead (same PID-agnostic idea already
			// used for reconciling saved links, see patchbay.go's
			// normalizePortName).
			//
			// The anchor stream (see primaryStreamMount) is included
			// here too, not skipped -- confirmed live 2026-07-10 that
			// skipping it was a real bug, not a harmless redundancy:
			// ReconcileLinks (patchbay.go) treats *any* live connection
			// not in the saved set as unwanted and tears it down, so
			// omitting the anchor's own link from `desired` actively
			// undid syncStereoToolTarget's direct connection every
			// reconcile cycle instead of just duplicating it.
			sourcePorts, err = stereoToolOutputPorts()
		} else {
			sourcePorts, err = clientPorts(cfg.ProgramSource, ListOutputPorts)
		}
		if err != nil {
			return fmt.Errorf("listing program source ports: %w", err)
		}
		for _, s := range cfg.Streams {
			clientID := streamJackClientID(cfg.Liquidsoap.JackInputID, s.Mount)
			hasLink := false
			for _, l := range desired {
				if strings.HasPrefix(l.Input, clientID+":") {
					hasLink = true
					break
				}
			}
			if hasLink {
				continue
			}
			// Poll briefly: the stream's systemd unit was only just
			// enabled --now moments ago (see sudoRun(taskCtl,
			// "enable-service", id) above), so its JACK ports may not
			// have registered yet.
			inputPorts, err := pollClientPorts(clientID, ListInputPorts)
			if err != nil {
				return fmt.Errorf("listing stream %q ports: %w", s.Mount, err)
			}
			n := len(sourcePorts)
			if len(inputPorts) < n {
				n = len(inputPorts)
			}
			for i := 0; i < n; i++ {
				desired = append(desired, PatchLink{Output: sourcePorts[i], Input: inputPorts[i]})
			}
		}
	}

	return SaveDesiredLinks(desired, DesiredLinksPath)
}

// clientPorts returns every port belonging to clientID from list
// (ListOutputPorts or ListInputPorts), sorted for a stable L/R pairing
// order.
func clientPorts(clientID string, list func() ([]string, error)) ([]string, error) {
	all, err := list()
	if err != nil {
		return nil, err
	}
	prefix := clientID + ":"
	var ports []string
	for _, p := range all {
		if strings.HasPrefix(p, prefix) {
			ports = append(ports, p)
		}
	}
	sort.Strings(ports)
	return ports, nil
}

// pollClientPorts retries clientPorts for up to streamPortsPollTimeout,
// for a client whose process may have only just started.
func pollClientPorts(clientID string, list func() ([]string, error)) ([]string, error) {
	deadline := time.Now().Add(streamPortsPollTimeout)
	for {
		ports, err := clientPorts(clientID, list)
		if err != nil {
			return nil, err
		}
		if len(ports) > 0 || time.Now().After(deadline) {
			return ports, nil
		}
		time.Sleep(streamPortsPollInterval)
	}
}

// loadFfmpegStreamsManifest reads the mount->unit-ID map persisted by
// the last DeployFfmpegStreams call. Returns an empty map, not an
// error, if the file doesn't exist yet (first deploy) -- same
// not-yet-configured handling as LoadTasks.
func loadFfmpegStreamsManifest(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// saveFfmpegStreamsManifest writes m as JSON to path, atomically (temp
// file + rename), same pattern as every other Save* in this package.
func saveFfmpegStreamsManifest(m map[string]string, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
