package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	DatabaseURL    string
	Port           int
	CORSOrigins    []string
	UserServiceURL string // e.g. http://user-service:8081
}

func Load() *Config {
	port := 8083
	if v := os.Getenv("PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			port = p
		}
	}

	origins := []string{"http://localhost:3000"}
	if v := os.Getenv("CORS_ORIGINS"); v != "" {
		origins = strings.Split(v, ",")
	}

	userSvcURL := os.Getenv("USER_SERVICE_URL")
	if userSvcURL == "" {
		userSvcURL = "http://user-service:8081"
	}

	return &Config{
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		Port:           port,
		CORSOrigins:    origins,
		UserServiceURL: userSvcURL,
	}
}
