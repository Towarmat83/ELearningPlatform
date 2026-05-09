package config

import (
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	JWTSecret          string   `yaml:"jwt_secret"`
	JWTExpiryH         int      `yaml:"jwt_expiry_hours"`
	Port               int      `yaml:"port"`
	CORSOrigins        []string `yaml:"cors_origins"`
	UploadsDir         string   `yaml:"uploads_dir"`
	Kubeconfig         string   `yaml:"kubeconfig"`
	K8sNamespace       string   `yaml:"k8s_namespace"`
	UserServiceURL     string   `yaml:"user_service_url"`
	GitToken           string   `yaml:"git_token"`
	GitCredentialsPath string   `yaml:"git_credentials_path"`
}

func Load() *Config {
	c := &Config{
		JWTSecret:          "change-me-in-production-use-a-long-random-string",
		JWTExpiryH:         24,
		Port:               8082,
		CORSOrigins:        []string{"http://localhost:3000", "http://localhost:5173"},
		UploadsDir:         "./uploads",
		Kubeconfig:         "",
		K8sNamespace:       "default",
		UserServiceURL:     "http://user-service:8081",
		GitCredentialsPath: "/etc/course-service/git-credentials.yaml",
	}

	if path := os.Getenv("CONFIG_PATH"); path != "" {
		if data, err := os.ReadFile(path); err == nil {
			yaml.Unmarshal(data, c)
		}
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
	if v := os.Getenv("UPLOADS_DIR"); v != "" {
		c.UploadsDir = v
	}
	if v := os.Getenv("KUBECONFIG"); v != "" {
		c.Kubeconfig = v
	}
	if v := os.Getenv("K8S_NAMESPACE"); v != "" {
		c.K8sNamespace = v
	}
	if v := os.Getenv("USER_SERVICE_URL"); v != "" {
		c.UserServiceURL = v
	}
	if v := os.Getenv("GIT_TOKEN"); v != "" {
		c.GitToken = v
	}
	if v := os.Getenv("GIT_CREDENTIALS_PATH"); v != "" {
		c.GitCredentialsPath = v
	}

	return c
}
