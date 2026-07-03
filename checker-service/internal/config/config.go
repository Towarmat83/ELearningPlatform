package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port          int
	CORSOrigins   []string
	GitLabToken   string
	GitLabBaseURL string
}

func Load() *Config {
	c := &Config{
		Port:          8083,
		CORSOrigins:   []string{"http://localhost:3000", "http://localhost:5173"},
		GitLabBaseURL: "http://gitlab.local:8880",
	}

	if v := os.Getenv("PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Port = n
		}
	}

	if v := os.Getenv("CORS_ORIGINS"); v != "" {
		c.CORSOrigins = strings.Split(v, ",")
	}

	if v := os.Getenv("GITLAB_TOKEN"); v != "" {
		c.GitLabToken = v
	}

	if v := os.Getenv("GITLAB_BASE_URL"); v != "" {
		c.GitLabBaseURL = v
	}

	return c
}
