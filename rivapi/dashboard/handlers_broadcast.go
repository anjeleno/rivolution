package dashboard

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/anjeleno/rivolution/rivapi/store"
)

type broadcastPageData struct {
	baseData
	Config     store.BroadcastConfig
	ConfigJS   template.JS // safe for Alpine x-data initialization
	SaveResult *broadcastSaveResult
}

type broadcastSaveResult struct {
	Success bool
	Steps   []string
	Error   string
}

func (h *Handler) broadcastPageData(r *http.Request, result *broadcastSaveResult) (broadcastPageData, error) {
	cfg, err := store.LoadBroadcastConfig(h.cfg.BroadcastConfigPath)
	if err != nil {
		return broadcastPageData{}, err
	}
	cfgJSON, err := json.Marshal(cfg)
	if err != nil {
		return broadcastPageData{}, err
	}
	return broadcastPageData{
		baseData:   h.base(r, "Broadcast", "broadcast"),
		Config:     cfg,
		ConfigJS:   template.JS(cfgJSON),
		SaveResult: result,
	}, nil
}

// Broadcast handles GET /broadcast.
func (h *Handler) Broadcast(w http.ResponseWriter, r *http.Request) {
	data, err := h.broadcastPageData(r, nil)
	if err != nil {
		http.Error(w, "error loading broadcast config: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tmplBroadcast.ExecuteTemplate(w, "base", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// BroadcastSave handles POST /broadcast/save.
// Parses the form, saves the JSON config, generates icecast.xml and radio.liq,
// then restarts both services. Always returns the full broadcast page so the
// operator sees the outcome inline.
func (h *Handler) BroadcastSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	result := &broadcastSaveResult{}

	cfg, parseErr := parseBroadcastForm(r)
	if parseErr != nil {
		result.Error = "form parse error: " + parseErr.Error()
		data, _ := h.broadcastPageData(r, result)
		_ = tmplBroadcast.ExecuteTemplate(w, "base", data)
		return
	}

	// ProgramSource lives on /patchbay now (spec: the "what feeds every
	// stream" decision belongs next to routing, not stream encoding), so
	// this form no longer submits it. parseBroadcastForm builds cfg from
	// scratch, so without this it would silently blank out whatever
	// /patchbay last set on every unrelated /broadcast save. Best-effort:
	// if there's no existing config yet (first-ever save), ProgramSource
	// just stays empty, same as today's "not configured yet" meaning.
	if existing, err := store.LoadBroadcastConfig(h.cfg.BroadcastConfigPath); err == nil {
		cfg.ProgramSource = existing.ProgramSource
	}

	// Save JSON config first so the state is durable even if deploy steps fail.
	if err := store.SaveBroadcastConfig(cfg, h.cfg.BroadcastConfigPath); err != nil {
		result.Error = "saving config: " + err.Error()
		data, _ := h.broadcastPageData(r, result)
		_ = tmplBroadcast.ExecuteTemplate(w, "base", data)
		return
	}
	result.Steps = append(result.Steps, "Config saved.")

	// Generate and install icecast.xml.
	if err := store.GenerateIcecastXML(cfg); err != nil {
		result.Error = "generating icecast.xml: " + err.Error()
		data, _ := h.broadcastPageData(r, result)
		_ = tmplBroadcast.ExecuteTemplate(w, "base", data)
		return
	}
	result.Steps = append(result.Steps, "icecast.xml written to /etc/icecast2/icecast.xml.")

	// Restart icecast2.
	if out, err := exec.Command("sudo", "systemctl", "restart", "icecast2.service").CombinedOutput(); err != nil {
		result.Error = "restarting icecast2: " + err.Error() + ": " + strings.TrimSpace(string(out))
		data, _ := h.broadcastPageData(r, result)
		_ = tmplBroadcast.ExecuteTemplate(w, "base", data)
		return
	}
	result.Steps = append(result.Steps, "icecast2 restarted.")

	// Deploy one always-on systemd service per stream (ffmpeg: JACK
	// capture -> encode -> Icecast push), replacing the single
	// radio.liq/liquidsoap.service this used to be. See
	// rivapi/store/ffmpeg_generator.go and docs/handoff/2026-07-09.md
	// for why Liquidsoap was replaced.
	if err := store.DeployFfmpegStreams(cfg); err != nil {
		result.Error = "deploying stream services: " + err.Error()
		data, _ := h.broadcastPageData(r, result)
		_ = tmplBroadcast.ExecuteTemplate(w, "base", data)
		return
	}
	time.Sleep(400 * time.Millisecond)
	result.Steps = append(result.Steps, fmt.Sprintf("%d stream service(s) deployed and started.", len(cfg.Streams)))

	result.Success = true
	data, err := h.broadcastPageData(r, result)
	if err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	if err := tmplBroadcast.ExecuteTemplate(w, "base", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// parseBroadcastForm reads the POST form into a BroadcastConfig.
// Streams are submitted as a JSON blob in the "streams_json" field,
// serialized client-side by Alpine before form submission.
func parseBroadcastForm(r *http.Request) (store.BroadcastConfig, error) {
	var cfg store.BroadcastConfig

	cfg.Station = store.StationDefaults{
		Name:        strings.TrimSpace(r.FormValue("station_name")),
		Genre:       strings.TrimSpace(r.FormValue("station_genre")),
		Description: strings.TrimSpace(r.FormValue("station_description")),
		URL:         strings.TrimSpace(r.FormValue("station_url")),
	}

	port := 8000
	if p, err := parseInt(r.FormValue("icecast_port")); err == nil {
		port = p
	}
	maxClients := 100
	if m, err := parseInt(r.FormValue("icecast_max_clients")); err == nil {
		maxClients = m
	}
	burstSize := 65535
	if b, err := parseInt(r.FormValue("icecast_burst_size")); err == nil {
		burstSize = b
	}
	cfg.Icecast = store.IcecastCfg{
		Hostname:       strings.TrimSpace(r.FormValue("icecast_hostname")),
		Port:           port,
		AdminEmail:     strings.TrimSpace(r.FormValue("icecast_admin_email")),
		Location:       strings.TrimSpace(r.FormValue("icecast_location")),
		SourcePassword: strings.TrimSpace(r.FormValue("icecast_source_password")),
		RelayPassword:  strings.TrimSpace(r.FormValue("icecast_relay_password")),
		AdminUser:      strings.TrimSpace(r.FormValue("icecast_admin_user")),
		AdminPassword:  strings.TrimSpace(r.FormValue("icecast_admin_password")),
		MaxClients:     maxClients,
		BurstSize:      burstSize,
	}

	liqPort := 8000
	if p, err := parseInt(r.FormValue("liq_icecast_port")); err == nil {
		liqPort = p
	}
	sampleRate := 48000
	if s, err := parseInt(r.FormValue("liq_sample_rate")); err == nil {
		sampleRate = s
	}
	cfg.Liquidsoap = store.LiquidsoapCfg{
		IcecastHost: strings.TrimSpace(r.FormValue("liq_icecast_host")),
		IcecastPort: liqPort,
		JackInputID: strings.TrimSpace(r.FormValue("liq_jack_input_id")),
		LogPath:     strings.TrimSpace(r.FormValue("liq_log_path")),
		SampleRate:  sampleRate,
	}

	streamsJSON := r.FormValue("streams_json")
	if streamsJSON != "" {
		if err := json.Unmarshal([]byte(streamsJSON), &cfg.Streams); err != nil {
			return cfg, err
		}
	}

	return cfg, nil
}

func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &n)
	return n, err
}
