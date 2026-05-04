package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL  string
	JWTSecret    string
	JWTExpiryH   int
	Port         int
	CORSOrigins  []string
	UploadsDir   string

	// Content-as-code
	CoursesDir      string
	ReposDir        string
	RepoTokenSecret string

	GitLabClientID     string
	GitLabClientSecret string
	GitLabURL          string
	GitHubClientID     string
	GitHubClientSecret string
	OAuthRedirectBase  string
}

func Load() *Config {
	godotenv.Load()

	return &Config{
		DatabaseURL: getEnv("DATABASE_URL", "postgres://elearning:elearning@localhost:5432/elearning"),
		JWTSecret:   getEnv("JWT_SECRET", "change-me-in-production-use-a-long-random-string"),
		JWTExpiryH:  getEnvInt("JWT_EXPIRY_HOURS", 24),
		Port:        getEnvInt("PORT", 8080),
		CORSOrigins: strings.Split(getEnv("CORS_ORIGINS", "http://localhost:3000,http://localhost:5173"), ","),
		UploadsDir:  getEnv("UPLOADS_DIR", "./uploads"),

		CoursesDir:      getEnv("COURSES_DIR", "./courses"),
		ReposDir:        getEnv("REPOS_DIR", "./data/repos"),
		RepoTokenSecret: getEnv("REPO_TOKEN_SECRET", "change-me-use-a-random-32-char-secret"),

		GitLabClientID:     os.Getenv("GITLAB_CLIENT_ID"),
		GitLabClientSecret: os.Getenv("GITLAB_CLIENT_SECRET"),
		GitLabURL:          strings.TrimRight(getEnv("GITLAB_URL", "https://gitlab.com"), "/"),
		GitHubClientID:     os.Getenv("GITHUB_CLIENT_ID"),
		GitHubClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),
		OAuthRedirectBase:  getEnv("OAUTH_REDIRECT_BASE", "http://localhost:3000"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
