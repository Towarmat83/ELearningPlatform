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

// defaultInternalSecret is the fallback service-to-service shared secret.
const defaultInternalSecret = "change-me-internal-secret"

// defaultFrontendURL is the default local frontend origin for CORS.
const defaultFrontendURL = "http://localhost:3000"

// defaultViteDevURL is the Vite development server origin for CORS.
const defaultViteDevURL = "http://localhost:5173"

// defaultGitLabBaseURL is the default GitLab instance base URL.
const defaultGitLabBaseURL = "http://gitlab.local:8880"

// Config holds checker-service runtime configuration.
type Config struct {
	Port                   int
	CORSOrigins            []string
	GitLabToken            string
	GitLabBaseURL          string
	RateLimitRequests      int
	RateLimitWindowSeconds int
	InternalSecret         string
}

// Load builds a Config from environment variables, falling back to defaults.
func Load() *Config {
	cfg := &Config{
		Port:                   defaultPort,
		CORSOrigins:            []string{defaultFrontendURL, defaultViteDevURL},
		GitLabBaseURL:          defaultGitLabBaseURL,
		RateLimitRequests:      defaultRateLimitRequests,
		RateLimitWindowSeconds: defaultRateLimitWindowSeconds,
		InternalSecret:         defaultInternalSecret,
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

	cfg.RateLimitRequests = rateLimitFromEnv("RATE_LIMIT_REQUESTS", cfg.RateLimitRequests)
	cfg.RateLimitWindowSeconds = rateLimitFromEnv("RATE_LIMIT_WINDOW_SECONDS", cfg.RateLimitWindowSeconds)

	if v := os.Getenv("INTERNAL_SERVICE_SECRET"); v != "" {
		cfg.InternalSecret = v
	}

	return cfg
}

// rateLimitFromEnv parses the env var as a non-negative integer. 0 means
// disabled; negatives and non-integers fall back to current.
func rateLimitFromEnv(key string, current int) int {
	v := os.Getenv(key)
	if v == "" {
		return current
	}

	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return current
	}

	return n
}
