package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

type ProviderConfig struct {
	ID           string `yaml:"id"`
	Name         string `yaml:"name"`
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	IssuerURL    string `yaml:"issuer_url"`
}

type Config struct {
	DatabaseURL       string           `yaml:"database_url"`
	JWTSecret         string           `yaml:"jwt_secret"`
	JWTExpiryH        int              `yaml:"jwt_expiry_hours"`
	Port              int              `yaml:"port"`
	CORSOrigins       []string         `yaml:"cors_origins"`
	OAuthRedirectBase string           `yaml:"oauth_redirect_base"`
	Providers         []ProviderConfig `yaml:"providers"`
	CourseServiceURL  string           `yaml:"course_service_url"`
	// AdminPassword is the password (or pre-computed bcrypt hash) for the
	// bootstrapped admin account.
	// Resolution order:
	//  1. ADMIN_PASSWORD_FILE — path to a mounted secret file (K8s volume).
	//     The file is watched for changes so no pod restart is needed.
	//  2. ADMIN_PASSWORD — direct env var (set once at startup).
	//  3. Neither set — hardcoded default "Admin@1234" with a loud warning.
	AdminPassword     string `yaml:"-"`
	AdminPasswordFile string `yaml:"-"` // path to the mounted secret file, if any
}

func (c *Config) FindProvider(id string) *ProviderConfig {
	for i := range c.Providers {
		if c.Providers[i].ID == id {
			return &c.Providers[i]
		}
	}
	return nil
}

func Load() *Config {
	c := &Config{
		DatabaseURL:       "postgres://elearning:elearning@localhost:5432/elearning",
		JWTSecret:         "change-me-in-production-use-a-long-random-string",
		JWTExpiryH:        24,
		Port:              8081,
		CORSOrigins:       []string{"http://localhost:3000", "http://localhost:5173"},
		OAuthRedirectBase: "http://localhost:3000",
		CourseServiceURL:  "http://course-service:8082",
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
	if v := os.Getenv("OAUTH_REDIRECT_BASE"); v != "" {
		c.OAuthRedirectBase = v
	}
	if v := os.Getenv("COURSE_SERVICE_URL"); v != "" {
		c.CourseServiceURL = v
	}
	// ADMIN_PASSWORD_FILE takes priority: read initial value from the mounted
	// secret file so the watcher goroutine can detect future changes.
	if f := os.Getenv("ADMIN_PASSWORD_FILE"); f != "" {
		c.AdminPasswordFile = f
		if data, err := os.ReadFile(f); err == nil {
			c.AdminPassword = strings.TrimSpace(string(data))
		} else {
			// File not readable yet (e.g. volume not mounted) — fall through
			// to ADMIN_PASSWORD env var below.
			c.AdminPasswordFile = f // keep path so watcher retries
		}
	}
	// ADMIN_PASSWORD env var as fallback (or when no file path is configured).
	if c.AdminPassword == "" {
		c.AdminPassword = os.Getenv("ADMIN_PASSWORD")
	}

	return c
}
