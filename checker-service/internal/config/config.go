// Package config loads checker-service configuration from env vars.
package config

import (
	"os"
	"strconv"
	"strings"
)

// defaultPort is used when the PORT env var is unset or invalid.
const defaultPort = 8083

// Config holds checker-service runtime configuration.
type Config struct {
	Port          int
	CORSOrigins   []string
	GitLabToken   string
	GitLabBaseURL string
}

// Load builds a Config from environment variables, falling back to defaults.
func Load() *Config {
	cfg := &Config{
		Port:          defaultPort,
		CORSOrigins:   []string{"http://localhost:3000", "http://localhost:5173"},
		GitLabBaseURL: "http://gitlab.local:8880",
	}

	if v := os.Getenv("PORT"); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil {
			cfg.Port = n
		}
	}

	if v := os.Getenv("CORS_ORIGINS"); v != "" {
		cfg.CORSOrigins = strings.Split(v, ",")
	}

	if v := os.Getenv("GITLAB_TOKEN"); v != "" {
		cfg.GitLabToken = v
	}

	if v := os.Getenv("GITLAB_BASE_URL"); v != "" {
		cfg.GitLabBaseURL = v
	}

	return cfg
}
