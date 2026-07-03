package config

import (
	"os"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	// Clear env vars that might affect defaults
	for _, k := range []string{"JWT_SECRET", "PORT", "CORS_ORIGINS", "GIT_CACHE_TTL",
		"UPLOADS_DIR", "K8S_NAMESPACE", "USER_SERVICE_URL", "GIT_TOKEN",
		"GIT_CREDENTIALS_PATH", "JWT_EXPIRY_HOURS", "KUBECONFIG", "CONFIG_PATH"} {
		os.Unsetenv(k)
	}

	c := Load()

	if c.JWTSecret != "change-me-in-production-use-a-long-random-string" {
		t.Errorf("unexpected default JWTSecret: %q", c.JWTSecret)
	}

	if c.Port != 8082 {
		t.Errorf("unexpected default Port: %d", c.Port)
	}

	if c.JWTExpiryH != 24 {
		t.Errorf("unexpected default JWTExpiryH: %d", c.JWTExpiryH)
	}

	if c.UploadsDir != "./uploads" {
		t.Errorf("unexpected default UploadsDir: %q", c.UploadsDir)
	}

	if c.K8sNamespace != "default" {
		t.Errorf("unexpected default K8sNamespace: %q", c.K8sNamespace)
	}

	if c.GitCacheTTL != 10 {
		t.Errorf("unexpected default GitCacheTTL: %d", c.GitCacheTTL)
	}

	if len(c.CORSOrigins) == 0 {
		t.Error("expected non-empty default CORSOrigins")
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	os.Setenv("JWT_SECRET", "my-secret")
	os.Setenv("PORT", "9090")
	os.Setenv("JWT_EXPIRY_HOURS", "48")
	os.Setenv("CORS_ORIGINS", "http://a.com,http://b.com")
	os.Setenv("UPLOADS_DIR", "/tmp/uploads")
	os.Setenv("K8S_NAMESPACE", "prod")
	os.Setenv("USER_SERVICE_URL", "http://user:9000")
	os.Setenv("GIT_TOKEN", "tok123")
	os.Setenv("GIT_CREDENTIALS_PATH", "/etc/creds.yaml")
	os.Setenv("GIT_CACHE_TTL", "30")

	defer func() {
		for _, k := range []string{"JWT_SECRET", "PORT", "JWT_EXPIRY_HOURS", "CORS_ORIGINS",
			"UPLOADS_DIR", "K8S_NAMESPACE", "USER_SERVICE_URL", "GIT_TOKEN",
			"GIT_CREDENTIALS_PATH", "GIT_CACHE_TTL"} {
			os.Unsetenv(k)
		}
	}()

	c := Load()

	if c.JWTSecret != "my-secret" {
		t.Errorf("expected JWTSecret=my-secret, got %q", c.JWTSecret)
	}

	if c.Port != 9090 {
		t.Errorf("expected Port=9090, got %d", c.Port)
	}

	if c.JWTExpiryH != 48 {
		t.Errorf("expected JWTExpiryH=48, got %d", c.JWTExpiryH)
	}

	if len(c.CORSOrigins) != 2 {
		t.Errorf("expected 2 CORS origins, got %d", len(c.CORSOrigins))
	}

	if c.UploadsDir != "/tmp/uploads" {
		t.Errorf("expected UploadsDir=/tmp/uploads, got %q", c.UploadsDir)
	}

	if c.K8sNamespace != "prod" {
		t.Errorf("expected K8sNamespace=prod, got %q", c.K8sNamespace)
	}

	if c.UserServiceURL != "http://user:9000" {
		t.Errorf("expected UserServiceURL, got %q", c.UserServiceURL)
	}

	if c.GitToken != "tok123" {
		t.Errorf("expected GitToken=tok123, got %q", c.GitToken)
	}

	if c.GitCredentialsPath != "/etc/creds.yaml" {
		t.Errorf("expected GitCredentialsPath, got %q", c.GitCredentialsPath)
	}

	if c.GitCacheTTL != 30 {
		t.Errorf("expected GitCacheTTL=30, got %d", c.GitCacheTTL)
	}
}

func TestLoad_InvalidPort_Ignored(t *testing.T) {
	os.Setenv("PORT", "notanumber")

	defer os.Unsetenv("PORT")

	c := Load()
	if c.Port != 8082 {
		t.Errorf("expected default port 8082 on invalid PORT env, got %d", c.Port)
	}
}

func TestLoad_InvalidGitCacheTTL_Ignored(t *testing.T) {
	os.Setenv("GIT_CACHE_TTL", "0")

	defer os.Unsetenv("GIT_CACHE_TTL")

	c := Load()
	if c.GitCacheTTL != 10 {
		t.Errorf("expected default GitCacheTTL=10 when 0 provided, got %d", c.GitCacheTTL)
	}
}

func TestLoad_ConfigFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	f.WriteString(`
jwt_secret: from-file
port: 7777
cors_origins:
  - http://file.com
`)
	f.Close()

	os.Setenv("CONFIG_PATH", f.Name())

	defer os.Unsetenv("CONFIG_PATH")

	c := Load()
	if c.JWTSecret != "from-file" {
		t.Errorf("expected JWTSecret=from-file, got %q", c.JWTSecret)
	}

	if c.Port != 7777 {
		t.Errorf("expected Port=7777, got %d", c.Port)
	}
}

func TestLoad_Kubeconfig(t *testing.T) {
	os.Setenv("KUBECONFIG", "/home/user/.kube/config")

	defer os.Unsetenv("KUBECONFIG")

	c := Load()
	if c.Kubeconfig != "/home/user/.kube/config" {
		t.Errorf("expected Kubeconfig set, got %q", c.Kubeconfig)
	}
}
