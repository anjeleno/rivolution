package store

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
)

// LiquidsoapScriptPath is where rivapi writes the generated radio.liq.
// The liquidsoap.service drop-in points ExecStart here.
const LiquidsoapScriptPath = "/home/rd/etc/liquidsoap/radio.liq"

var liquidsoapTmpl = template.Must(template.New("liquidsoap").Funcs(template.FuncMap{
	"liqEncoder":   liqEncoder,
	"aacExtras":    liqAACExtras,
	"liqStreamURL": liqStreamURL,
}).Parse(`#!/usr/bin/liquidsoap

set("log.file.path", "{{.Liquidsoap.LogPath}}")
set("frame.audio.samplerate", {{.Liquidsoap.SampleRate}})
set("icy.metadata", true)

radio = input.jack(id="{{.Liquidsoap.JackInputID}}")
radio = mksafe(radio)

{{range .Streams}}
output.icecast(
  {{liqEncoder . $.Liquidsoap.SampleRate}},
  host="{{$.Liquidsoap.IcecastHost}}",
  port={{$.Liquidsoap.IcecastPort}},
  password="{{$.Icecast.SourcePassword}}",
  mount="{{.Mount}}",
  name="{{.EffectiveName $.Station}}",
  genre="{{.EffectiveGenre $.Station}}",
  description="{{.EffectiveDescription $.Station}}",
  url="{{liqStreamURL . $.Liquidsoap.IcecastHost $.Liquidsoap.IcecastPort}}",{{aacExtras .}}
  radio
)
{{end}}
`))

// liqEncoder returns the Liquidsoap encoder format string for a stream.
// AAC streams use %external piped through fdkaac (package: fdkaac, Ubuntu
// universe). MP3 uses %mp3 (lame), OGG uses %ogg(%vorbis).
//
// he-aac-v1/v2 currently fall back to plain AAC-LC (fdkaac --profile 2):
// Ubuntu's libfdk-aac2 package ships with SBR encoding disabled (patent
// restriction on the SBR encoder specifically, separate from the
// unencumbered AAC-LC encoder and SBR decoder) — profiles 5 (HE-AAC) and
// 29 (HE-AAC v2) both fail with "unsupported profile" on this build.
// Revisit if a non-distro fdk-aac build with SBR encoding becomes
// available.
//
// -f 2 (ADTS) is required for stdout streaming: fdkaac's default
// transport format is muxed into an M4A container, which needs to seek
// back and write its moov box — "stdout streaming is not available on
// M4A output". ADTS is a continuous streamable bitstream instead.
func liqEncoder(s StreamConfig, sampleRate int) string {
	switch s.Codec {
	case "mp3":
		return fmt.Sprintf("%%mp3(bitrate=%d)", s.Bitrate)
	case "he-aac-v1", "he-aac-v2":
		return fmt.Sprintf(
			"%%external(channels=2, samplerate=%d, header=false, restart_on_crash=true,\n"+
				`    process="fdkaac --bitrate %d000 --profile 2 --raw --raw-channels 2 --raw-rate %d -f 2 -o - -")`,
			sampleRate, s.Bitrate, sampleRate,
		)
	case "ogg":
		return fmt.Sprintf("%%ogg(%%vorbis(quality=%.1f))", s.Quality)
	default:
		return fmt.Sprintf("%%mp3(bitrate=%d)", s.Bitrate)
	}
}

// liqStreamURL returns the direct playback URL for a stream. When the stream
// has an explicit URL override it is used as-is; otherwise the URL is
// constructed from the Icecast host, port, and mount so that Icecast renders
// a correct "Listen Live" link for the mount.
func liqStreamURL(s StreamConfig, host string, port int) string {
	if s.URL != "" {
		return s.URL
	}
	return fmt.Sprintf("http://%s:%d%s", host, port, s.Mount)
}

// liqAACExtras returns extra output.icecast() arguments needed for AAC
// streams (which use the %external encoder), empty for others:
//   - format: output.icecast's MIME-type argument is named "format"
//     (Liquidsoap 2.4.x), not "content_type" (an older API name).
//   - send_icy_metadata: Liquidsoap can infer this for known formats like
//     %mp3, but not for %external, and errors at load time if left unset.
func liqAACExtras(s StreamConfig) string {
	switch s.Codec {
	case "he-aac-v1", "he-aac-v2":
		return "\n  format=\"audio/aacp\",\n  send_icy_metadata=true,"
	}
	return ""
}

// GenerateLiquidsoapScript renders radio.liq from cfg and writes it to
// LiquidsoapScriptPath. No privilege needed — the path is rd-owned.
// Also creates the log directory so Liquidsoap can open its log file on start.
func GenerateLiquidsoapScript(cfg BroadcastConfig) error {
	var buf bytes.Buffer
	if err := liquidsoapTmpl.Execute(&buf, cfg); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(LiquidsoapScriptPath), 0755); err != nil {
		return err
	}
	// Create the log file (and its directory) if absent. Liquidsoap opens
	// the log for appending on start and will fail if the path doesn't exist.
	if cfg.Liquidsoap.LogPath != "" {
		if err := os.MkdirAll(filepath.Dir(cfg.Liquidsoap.LogPath), 0755); err != nil {
			return fmt.Errorf("creating log directory: %w", err)
		}
		f, err := os.OpenFile(cfg.Liquidsoap.LogPath, os.O_CREATE|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("creating log file: %w", err)
		}
		f.Close()
	}
	return os.WriteFile(LiquidsoapScriptPath, buf.Bytes(), 0644)
}

// wrapOutput wraps a command error with its combined output for error messages.
func wrapOutput(op string, err error, out []byte) error {
	if len(out) > 0 {
		return fmt.Errorf("%s: %w: %s", op, err, bytes.TrimSpace(out))
	}
	return fmt.Errorf("%s: %w", op, err)
}
