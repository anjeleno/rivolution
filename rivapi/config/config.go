package config

import (
	"fmt"
	"os"
	"strconv"
)

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
}

func Load() *Config {
	return &Config{
		DBHost:     getenv("RIVAPI_DB_HOST", "localhost"),
		DBPort:     getenvInt("RIVAPI_DB_PORT", 3306),
		DBUser:     getenv("RIVAPI_DB_USER", "rduser"),
		DBPassword: getenv("RIVAPI_DB_PASSWORD", ""),
		DBName:     getenv("RIVAPI_DB_NAME", "Rivendell"),
		RdxportURL: getenv("RIVAPI_RDXPORT_URL", "http://127.0.0.1/rd-bin/rdxport.cgi"),
		JWTSecret:  getenv("RIVAPI_JWT_SECRET", ""),
		ListenAddr: getenv("RIVAPI_LISTEN_ADDR", ":8080"),

		TLSCert: getenv("RIVAPI_TLS_CERT", ""),
		TLSKey:  getenv("RIVAPI_TLS_KEY", ""),

		TrustProxyHeaders: getenvBool("RIVAPI_TRUST_PROXY_HEADERS", false),
		CookieSecure:      getenvBool("RIVAPI_COOKIE_SECURE", false),

		StationName: getenv("RIVAPI_STATION_NAME", "Rivolution"),
		LogoURL:     getenv("RIVAPI_LOGO_URL", ""),
		AccentColor: getenv("RIVAPI_ACCENT_COLOR", ""),
	}
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
