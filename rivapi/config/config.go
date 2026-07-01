package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// rdConfPath is the standard Rivendell system config file read by all Rivendell
// binaries. rivapi reads [mySQL] credentials and [dashboard] JwtSecret from it,
// with env vars taking priority for dev/container overrides.
const rdConfPath = "/etc/rd.conf"

type Config struct {
	DBHost     string
	DBPort     int
	DBUser     string
	DBPassword string
	DBName     string

	RdxportURL string

	JWTSecret  string
	ListenAddr string

	// TLS — when both are set, rivapi listens with TLS directly (e.g. via tailscale cert)
	TLSCert string
	TLSKey  string

	// Cookie security — see spec 0012
	TrustProxyHeaders bool // read X-Forwarded-Proto to decide Secure flag
	CookieSecure      bool // force Secure flag regardless of proxy headers

	// Branding — overrides the Rivolution defaults in the dashboard nav
	StationName string
	LogoURL     string
	AccentColor string

	// StereoToolPath is the filesystem path where the dashboard installs
	// and expects to find the Stereo Tool binary. Defaults to /home/rd/bin/stereo_tool,
	// which rd owns and can write to without privilege escalation.
	StereoToolPath string

	// BroadcastConfigPath is where the broadcast dashboard persists its
	// JSON config (station, Icecast, Liquidsoap, stream list).
	BroadcastConfigPath string
}

func Load() *Config {
	// Read /etc/rd.conf first; env vars take priority over file values.
	rdConf := parseRdConf(rdConfPath)

	return &Config{
		DBHost:     getenv("RIVAPI_DB_HOST", rdConf.get("mySQL", "Hostname", "localhost")),
		DBPort:     getenvInt("RIVAPI_DB_PORT", 3306),
		DBUser:     getenv("RIVAPI_DB_USER", rdConf.get("mySQL", "Loginname", "rduser")),
		DBPassword: getenv("RIVAPI_DB_PASSWORD", rdConf.get("mySQL", "Password", "")),
		DBName:     getenv("RIVAPI_DB_NAME", rdConf.get("mySQL", "Database", "Rivendell")),
		RdxportURL: getenv("RIVAPI_RDXPORT_URL", "http://127.0.0.1/rd-bin/rdxport.cgi"),
		JWTSecret:  getenv("RIVAPI_JWT_SECRET", rdConf.get("dashboard", "JwtSecret", "")),
		ListenAddr: getenv("RIVAPI_LISTEN_ADDR", ":8080"),

		TLSCert: getenv("RIVAPI_TLS_CERT", ""),
		TLSKey:  getenv("RIVAPI_TLS_KEY", ""),

		TrustProxyHeaders: getenvBool("RIVAPI_TRUST_PROXY_HEADERS", false),
		CookieSecure:      getenvBool("RIVAPI_COOKIE_SECURE", false),

		StationName: getenv("RIVAPI_STATION_NAME", rdConf.get("dashboard", "StationName", "Rivolution")),
		LogoURL:     getenv("RIVAPI_LOGO_URL", rdConf.get("dashboard", "LogoURL", "")),
		AccentColor: getenv("RIVAPI_ACCENT_COLOR", rdConf.get("dashboard", "AccentColor", "")),

		StereoToolPath:      getenv("RIVAPI_STEREO_TOOL_PATH", "/home/rd/bin/stereo_tool"),
		BroadcastConfigPath: getenv("RIVAPI_BROADCAST_CONFIG", "/home/rd/etc/rivolution/broadcast.json"),
	}
}

// rdConfData holds the parsed contents of an INI-style rd.conf file.
// Keys are stored as map[section]map[key]value; all lookups are case-sensitive
// since rd.conf uses consistent casing throughout.
type rdConfData map[string]map[string]string

func (d rdConfData) get(section, key, fallback string) string {
	if s, ok := d[section]; ok {
		if v, ok := s[key]; ok && v != "" {
			return v
		}
	}
	return fallback
}

// parseRdConf reads an INI-style rd.conf file (`;`-commented, `[Section]` headers,
// `Key=Value` entries). Missing or unreadable files return an empty map silently —
// callers fall back to env vars and hardcoded defaults.
func parseRdConf(path string) rdConfData {
	data := make(rdConfData)
	f, err := os.Open(path)
	if err != nil {
		return data
	}
	defer f.Close()

	current := ""
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			current = line[1 : len(line)-1]
			if _, ok := data[current]; !ok {
				data[current] = make(map[string]string)
			}
			continue
		}
		if current == "" {
			continue
		}
		if idx := strings.IndexByte(line, '='); idx > 0 {
			k := strings.TrimSpace(line[:idx])
			v := strings.TrimSpace(line[idx+1:])
			data[current][k] = v
		}
	}
	return data
}

func (c *Config) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName)
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func getenvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}
