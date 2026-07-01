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
	"contentType":  liqContentType,
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
  url="{{.EffectiveURL $.Station}}",{{contentType .}}
  radio
)
{{end}}
`))

// liqEncoder returns the Liquidsoap encoder format string for a stream.
// AAC streams use %external piped through fdkaac (package: fdkaac, Ubuntu
// universe). MP3 uses %mp3 (lame), OGG uses %ogg(%vorbis).
func liqEncoder(s StreamConfig, sampleRate int) string {
	switch s.Codec {
	case "mp3":
		return fmt.Sprintf("%%mp3(bitrate=%d)", s.Bitrate)
	case "he-aac-v1":
		return fmt.Sprintf(
			"%%external(channels=2, samplerate=%d, header=false, restart_on_crash=true,\n"+
				`    process="fdkaac --bitrate %d000 --profile 5 --raw --raw-channels 2 --raw-rate %d -i - -o -")`,
			sampleRate, s.Bitrate, sampleRate,
		)
	case "he-aac-v2":
		return fmt.Sprintf(
			"%%external(channels=2, samplerate=%d, header=false, restart_on_crash=true,\n"+
				`    process="fdkaac --bitrate %d000 --profile 29 --raw --raw-channels 2 --raw-rate %d -i - -o -")`,
			sampleRate, s.Bitrate, sampleRate,
		)
	case "ogg":
		return fmt.Sprintf("%%ogg(%%vorbis(bitrate=%d))", s.Bitrate)
	default:
		return fmt.Sprintf("%%mp3(bitrate=%d)", s.Bitrate)
	}
}

// liqContentType returns the content_type line for AAC streams, empty for others.
func liqContentType(s StreamConfig) string {
	switch s.Codec {
	case "he-aac-v1", "he-aac-v2":
		return "\n  content_type=\"audio/aacp\","
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
	// Create the log directory Liquidsoap needs; Liquidsoap will fail on start
	// if the directory doesn't exist.
	if cfg.Liquidsoap.LogPath != "" {
		if err := os.MkdirAll(filepath.Dir(cfg.Liquidsoap.LogPath), 0755); err != nil {
			return fmt.Errorf("creating log directory: %w", err)
		}
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
