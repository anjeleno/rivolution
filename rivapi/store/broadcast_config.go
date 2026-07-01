package store

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// BroadcastConfig is the persisted state for the broadcast dashboard.
// Stored as JSON at BroadcastConfigPath; both the Icecast XML and the
// Liquidsoap .liq are generated from this at save time.
type BroadcastConfig struct {
	Station    StationDefaults  `json:"station"`
	Icecast    IcecastCfg       `json:"icecast"`
	Liquidsoap LiquidsoapCfg    `json:"liquidsoap"`
	Streams    []StreamConfig   `json:"streams"`
}

// StationDefaults are shared station identity fields. Any StreamConfig field
// left empty falls back to the corresponding StationDefaults value at
// generate time.
type StationDefaults struct {
	Name        string `json:"name"`
	Genre       string `json:"genre"`
	Description string `json:"description"`
	URL         string `json:"url"`
}

// StreamConfig describes one Liquidsoap output.icecast() call.
// Codec values: "mp3", "he-aac-v1", "he-aac-v2", "ogg".
// Bitrate is in kbps; no preset — fully user-controlled. Used by every
// codec except "ogg", which is quality (VBR) based instead — see Quality.
// Name/Genre/Description/URL override StationDefaults when non-empty.
type StreamConfig struct {
	Mount   string `json:"mount"`
	Codec   string `json:"codec"`
	Bitrate int    `json:"bitrate"`
	// Quality is Liquidsoap's %vorbis VBR quality knob, range -0.2 (lowest)
	// to 1.0 (highest), default 0.3. Only used when Codec is "ogg" — Vorbis
	// has no bitrate= parameter in this Liquidsoap version.
	Quality     float64 `json:"quality"`
	Name        string  `json:"name"`
	Genre       string  `json:"genre"`
	Description string  `json:"description"`
	URL         string `json:"url"`
}

// IcecastCfg holds every field rendered into icecast.xml.
// Sources count is NOT stored — it is auto-calculated as len(Streams)
// at generate time so it always matches the number of active outputs.
type IcecastCfg struct {
	Hostname       string `json:"hostname"`
	Port           int    `json:"port"`
	AdminEmail     string `json:"admin_email"`
	Location       string `json:"location"`
	SourcePassword string `json:"source_password"`
	RelayPassword  string `json:"relay_password"`
	AdminUser      string `json:"admin_user"`
	AdminPassword  string `json:"admin_password"`
	MaxClients     int    `json:"max_clients"`
	BurstSize      int    `json:"burst_size"`
}

// LiquidsoapCfg holds every field rendered into radio.liq.
type LiquidsoapCfg struct {
	IcecastHost string `json:"icecast_host"`
	IcecastPort int    `json:"icecast_port"`
	JackInputID string `json:"jack_input_id"`
	LogPath     string `json:"log_path"`
	SampleRate  int    `json:"sample_rate"`
}

// EffectiveName returns the stream's name if set, else the station default.
func (s StreamConfig) EffectiveName(d StationDefaults) string {
	if s.Name != "" {
		return s.Name
	}
	return d.Name
}

func (s StreamConfig) EffectiveGenre(d StationDefaults) string {
	if s.Genre != "" {
		return s.Genre
	}
	return d.Genre
}

func (s StreamConfig) EffectiveDescription(d StationDefaults) string {
	if s.Description != "" {
		return s.Description
	}
	return d.Description
}

func (s StreamConfig) EffectiveURL(d StationDefaults) string {
	if s.URL != "" {
		return s.URL
	}
	return d.URL
}

// DefaultBroadcastConfig returns a workable starting config seeded with
// the two MP3 streams from the pre-existing manual radio.liq.
func DefaultBroadcastConfig() BroadcastConfig {
	return BroadcastConfig{
		Station: StationDefaults{
			Name:  "My Radio Station",
			Genre: "Various",
			URL:   "http://localhost",
		},
		Icecast: IcecastCfg{
			Hostname:       "localhost",
			Port:           8000,
			AdminEmail:     "admin@localhost",
			Location:       "Earth",
			SourcePassword: "changeme",
			RelayPassword:  "changeme",
			AdminUser:      "admin",
			AdminPassword:  "changeme",
			MaxClients:     100,
			BurstSize:      65535,
		},
		Liquidsoap: LiquidsoapCfg{
			IcecastHost: "localhost",
			IcecastPort: 8000,
			JackInputID: "liquidsoap",
			LogPath:     "/home/rd/Log/liquidsoap.log",
			SampleRate:  48000,
		},
		Streams: []StreamConfig{
			{Mount: "/192", Codec: "mp3", Bitrate: 192},
			{Mount: "/stream", Codec: "mp3", Bitrate: 320},
		},
	}
}

// LoadBroadcastConfig reads the JSON config at path. Returns
// DefaultBroadcastConfig if the file does not exist yet.
func LoadBroadcastConfig(path string) (BroadcastConfig, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return DefaultBroadcastConfig(), nil
	}
	if err != nil {
		return BroadcastConfig{}, err
	}
	var cfg BroadcastConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return BroadcastConfig{}, err
	}
	return cfg, nil
}

// SaveBroadcastConfig writes cfg as JSON to path, creating parent
// directories as needed. The write is atomic (temp file + rename).
func SaveBroadcastConfig(cfg BroadcastConfig, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
