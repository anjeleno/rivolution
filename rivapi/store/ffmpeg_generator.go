package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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
		return fmt.Sprintf(
			"exec ffmpeg -nostdin -loglevel warning -f jack -i %s "+
				"-c:a libmp3lame -b:a %dk -content_type audio/mpeg "+
				"-ice_name %s -ice_genre %s -ice_description %s "+
				"%s",
			shellQuote(clientID), s.Bitrate,
			shellQuote(name), shellQuote(genre), shellQuote(description),
			shellQuote(icecastURL(cfg, s)),
		), nil
	case "ogg":
		return fmt.Sprintf(
			"exec ffmpeg -nostdin -loglevel warning -f jack -i %s "+
				"-c:a libvorbis -q:a %s -content_type application/ogg "+
				"-ice_name %s -ice_genre %s -ice_description %s "+
				"%s",
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
		// libfdk-aac2 build.
		return fmt.Sprintf(
			"exec ffmpeg -nostdin -loglevel warning -f jack -i %s "+
				"-f s16le -ar %d -ac 2 - | "+
				"fdkaac --silent --bitrate %d000 --profile 2 --raw --raw-channels 2 --raw-rate %d -f 2 -o - - | "+
				"exec ffmpeg -nostdin -loglevel warning -f aac -i - -c:a copy "+
				"-content_type audio/aacp -ice_name %s -ice_genre %s -ice_description %s "+
				"%s",
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

	return saveFfmpegStreamsManifest(current, FfmpegStreamsManifestPath)
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
