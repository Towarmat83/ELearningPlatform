// Package config loads checker-service configuration from env vars.
package config

import (
	"os"
	"strconv"
	"strings"
)

// defaultPort is used when the PORT env var is unset or invalid.
const defaultPort = 8083

// defaultRateLimitRequests is the default max requests per IP per window.
const defaultRateLimitRequests = 20

// defaultRateLimitWindowSeconds is the default rate-limit window in seconds.
const defaultRateLimitWindowSeconds = 60

// Config holds checker-service runtime configuration.
type Config struct {
	Port                   int
	CORSOrigins            []string
	GitLabToken            string
	GitLabBaseURL          string
	RateLimitRequests      int
	RateLimitWindowSeconds int
}

// Load builds a Config from environment variables, falling back to defaults.
func Load() *Config {
	cfg := &Config{
		Port:                   defaultPort,
		CORSOrigins:            []string{"http://localhost:3000", "http://localhost:5173"},
		GitLabBaseURL:          "http://gitlab.local:8880",
		RateLimitRequests:      defaultRateLimitRequests,
		RateLimitWindowSeconds: defaultRateLimitWindowSeconds,
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

	cfg.RateLimitRequests = positiveIntFromEnv("RATE_LIMIT_REQUESTS", cfg.RateLimitRequests)
	cfg.RateLimitWindowSeconds = positiveIntFromEnv("RATE_LIMIT_WINDOW_SECONDS", cfg.RateLimitWindowSeconds)

	return cfg
}

// positiveIntFromEnv returns the parsed value of the environment variable key
// when it is a positive integer, or current otherwise.
func positiveIntFromEnv(key string, current int) int {
	v := os.Getenv(key)
	if v == "" {
		return current
	}

	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return current
	}

	return n
}
