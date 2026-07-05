// Package config provides application configuration loading for
// course-service, merging built-in defaults, an optional YAML file, and
// environment variable overrides.
package config

import (
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// defaultJWTSecret is the fallback JWT signing secret; it must be
	// overridden in production via the JWT_SECRET env var or config file.
	defaultJWTSecret = "change-me-in-production-use-a-long-random-string"
	// defaultJWTExpiryHours is the default JWT token lifetime, in hours.
	defaultJWTExpiryHours = 24
	// defaultPort is the default HTTP listen port.
	defaultPort = 8082
	// defaultUploadsDir is the default directory for uploaded files.
	defaultUploadsDir = "./uploads"
	// defaultK8sNamespace is the default Kubernetes namespace to watch.
	defaultK8sNamespace = "default"
	// defaultGitCacheTTLMinutes is the default git cache TTL, in minutes.
	defaultGitCacheTTLMinutes = 10
)

// Config holds the application configuration for course-service.
type Config struct {
	JWTSecret string `yaml:"jwtSecret"`

	JWTExpiryH int `yaml:"jwtExpiryHours"`
	Port       int `yaml:"port"`

	CORSOrigins []string `yaml:"corsOrigins"`

	UploadsDir string `yaml:"uploadsDir"`
	Kubeconfig string `yaml:"kubeconfig"`

	K8sNamespace string `yaml:"k8sNamespace"`

	UserServiceURL string `yaml:"userServiceUrl"`

	GitToken string `yaml:"gitToken"`

	GitCredentialsPath string `yaml:"gitCredentialsPath"`

	GitCacheTTL int `yaml:"gitCacheTtlMinutes"`

	CheckerServiceURL string `yaml:"checkerServiceUrl"`

	DatabaseURL string `yaml:"databaseUrl"`
}

// Load builds a Config from built-in defaults, then overlays an optional
// YAML file referenced by CONFIG_PATH, then overlays environment variable
// overrides.
func Load() *Config {
	cfg := defaultConfig()

	loadFromFile(cfg)

	cfg.JWTSecret = stringFromEnv("JWT_SECRET", cfg.JWTSecret)
	cfg.JWTExpiryH = intFromEnv("JWT_EXPIRY_HOURS", cfg.JWTExpiryH)
	cfg.Port = intFromEnv("PORT", cfg.Port)
	cfg.CORSOrigins = sliceFromEnv("CORS_ORIGINS", cfg.CORSOrigins)
	cfg.UploadsDir = stringFromEnv("UPLOADS_DIR", cfg.UploadsDir)
	cfg.Kubeconfig = stringFromEnv("KUBECONFIG", cfg.Kubeconfig)
	cfg.K8sNamespace = stringFromEnv("K8S_NAMESPACE", cfg.K8sNamespace)
	cfg.UserServiceURL = stringFromEnv("USER_SERVICE_URL", cfg.UserServiceURL)
	cfg.GitToken = stringFromEnv("GIT_TOKEN", cfg.GitToken)
	cfg.GitCredentialsPath = stringFromEnv("GIT_CREDENTIALS_PATH", cfg.GitCredentialsPath)
	cfg.GitCacheTTL = positiveIntFromEnv("GIT_CACHE_TTL", cfg.GitCacheTTL)
	cfg.CheckerServiceURL = stringFromEnv("CHECKER_SERVICE_URL", cfg.CheckerServiceURL)
	cfg.DatabaseURL = stringFromEnv("DATABASE_URL", cfg.DatabaseURL)

	return cfg
}

// defaultConfig returns a Config populated with the built-in default values.
func defaultConfig() *Config {
	return &Config{
		JWTSecret:          defaultJWTSecret,
		JWTExpiryH:         defaultJWTExpiryHours,
		Port:               defaultPort,
		CORSOrigins:        []string{"http://localhost:3000", "http://localhost:5173"},
		UploadsDir:         defaultUploadsDir,
		Kubeconfig:         "",
		K8sNamespace:       defaultK8sNamespace,
		UserServiceURL:     "http://user-service:8081",
		GitCredentialsPath: "/etc/course-service/git-credentials.yaml",
		GitCacheTTL:        defaultGitCacheTTLMinutes,
		CheckerServiceURL:  "http://checker-service:8083",
	}
}

// loadFromFile overlays cfg with values from the YAML file referenced by the
// CONFIG_PATH environment variable, if set. Errors are ignored: an absent or
// unreadable config file simply leaves cfg at its current values.
func loadFromFile(cfg *Config) {
	path := os.Getenv("CONFIG_PATH")
	if path == "" {
		return
	}

	data, err := os.ReadFile(path) //nolint:gosec // path is operator-controlled via the CONFIG_PATH env var, not user input
	if err != nil {
		return
	}

	_ = yaml.Unmarshal(data, cfg)
}

// stringFromEnv returns the value of the environment variable key, or
// current if the variable is unset or empty.
func stringFromEnv(key, current string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return current
}

// sliceFromEnv returns the comma-separated value of the environment
// variable key split into a slice, or current if the variable is unset or
// empty.
func sliceFromEnv(key string, current []string) []string {
	v := os.Getenv(key)
	if v == "" {
		return current
	}

	return strings.Split(v, ",")
}

// intFromEnv returns the parsed integer value of the environment variable
// key, or current if the variable is unset, empty, or not a valid integer.
func intFromEnv(key string, current int) int {
	v := os.Getenv(key)
	if v == "" {
		return current
	}

	n, err := strconv.Atoi(v)
	if err != nil {
		return current
	}

	return n
}

// positiveIntFromEnv behaves like intFromEnv but also falls back to current
// when the parsed value is not strictly positive.
func positiveIntFromEnv(key string, current int) int {
	n := intFromEnv(key, current)
	if n <= 0 {
		return current
	}

	return n
}
