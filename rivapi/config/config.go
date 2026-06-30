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
