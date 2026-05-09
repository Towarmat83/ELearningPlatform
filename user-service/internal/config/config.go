package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

type Config struct {
	DatabaseURL        string   `yaml:"database_url"`
	JWTSecret          string   `yaml:"jwt_secret"`
	JWTExpiryH         int      `yaml:"jwt_expiry_hours"`
	Port               int      `yaml:"port"`
	CORSOrigins        []string `yaml:"cors_origins"`
	GitLabClientID     string   `yaml:"gitlab_client_id"`
	GitLabClientSecret string   `yaml:"gitlab_client_secret"`
	GitLabURL          string   `yaml:"gitlab_url"`
	GitHubClientID     string   `yaml:"github_client_id"`
	GitHubClientSecret string   `yaml:"github_client_secret"`
	OAuthRedirectBase  string   `yaml:"oauth_redirect_base"`
	CourseServiceURL   string   `yaml:"course_service_url"`
}

func Load() *Config {
	c := &Config{
		DatabaseURL:        "postgres://elearning:elearning@localhost:5432/elearning",
		JWTSecret:          "change-me-in-production-use-a-long-random-string",
		JWTExpiryH:         24,
		Port:               8081,
		CORSOrigins:        []string{"http://localhost:3000", "http://localhost:5173"},
		GitLabClientID:     "",
		GitLabClientSecret: "",
		GitLabURL:          "https://gitlab.com",
		GitHubClientID:     "",
		GitHubClientSecret: "",
		OAuthRedirectBase:  "http://localhost:3000",
		CourseServiceURL:   "http://course-service:8082",
	}

	godotenv.Load()

	if path := os.Getenv("CONFIG_PATH"); path != "" {
		if data, err := os.ReadFile(path); err == nil {
			yaml.Unmarshal(data, c)
		}
	}

	if v := os.Getenv("DATABASE_URL"); v != "" {
		c.DatabaseURL = v
	}
	if v := os.Getenv("JWT_SECRET"); v != "" {
		c.JWTSecret = v
	}
	if v := os.Getenv("JWT_EXPIRY_HOURS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.JWTExpiryH = n
		}
	}
	if v := os.Getenv("PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Port = n
		}
	}
	if v := os.Getenv("CORS_ORIGINS"); v != "" {
		c.CORSOrigins = strings.Split(v, ",")
	}
	if v := os.Getenv("GITLAB_CLIENT_ID"); v != "" {
		c.GitLabClientID = v
	}
	if v := os.Getenv("GITLAB_CLIENT_SECRET"); v != "" {
		c.GitLabClientSecret = v
	}
	if v := os.Getenv("GITLAB_URL"); v != "" {
		c.GitLabURL = strings.TrimRight(v, "/")
	}
	if v := os.Getenv("GITHUB_CLIENT_ID"); v != "" {
		c.GitHubClientID = v
	}
	if v := os.Getenv("GITHUB_CLIENT_SECRET"); v != "" {
		c.GitHubClientSecret = v
	}
	if v := os.Getenv("OAUTH_REDIRECT_BASE"); v != "" {
		c.OAuthRedirectBase = v
	}
	if v := os.Getenv("COURSE_SERVICE_URL"); v != "" {
		c.CourseServiceURL = v
	}

	return c
}
