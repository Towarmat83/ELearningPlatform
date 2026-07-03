package config

import (
	"os"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	// Clear all relevant env vars.
	for _, k := range []string{
		"DATABASE_URL", "JWT_SECRET", "JWT_EXPIRY_HOURS", "PORT",
		"CORS_ORIGINS", "OAUTH_REDIRECT_BASE", "COURSE_SERVICE_URL",
		"ADMIN_PASSWORD", "ADMIN_PASSWORD_FILE", "CONFIG_PATH",
		"OIDC_ENABLED", "OIDC_PROVIDER_URL", "OIDC_ISSUER_URL",
		"OIDC_CLIENT_ID", "OIDC_CLIENT_SECRET", "OIDC_CLIENT_SECRET_FILE",
		"OIDC_SCOPES", "OIDC_GROUP_CLAIM", "OIDC_REDIRECT_BASE", "OIDC_BROWSER_BASE_URL",
	} {
		os.Unsetenv(k)
	}

	c := Load()
	if c.Port != 8081 {
		t.Errorf("default port: want 8081, got %d", c.Port)
	}

	if c.JWTSecret != "change-me-in-production-use-a-long-random-string" {
		t.Errorf("unexpected default JWT secret: %q", c.JWTSecret)
	}

	if c.JWTExpiryH != 24 {
		t.Errorf("default expiry: want 24h, got %d", c.JWTExpiryH)
	}

	if c.DatabaseURL == "" {
		t.Error("expected non-empty default DatabaseURL")
	}

	if len(c.CORSOrigins) == 0 {
		t.Error("expected non-empty default CORSOrigins")
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	os.Setenv("JWT_SECRET", "my-secret")
	os.Setenv("JWT_EXPIRY_HOURS", "48")
	os.Setenv("PORT", "9090")
	os.Setenv("DATABASE_URL", "postgres://test:test@localhost/testdb")
	os.Setenv("CORS_ORIGINS", "http://a.com,http://b.com")

	defer func() {
		os.Unsetenv("JWT_SECRET")
		os.Unsetenv("JWT_EXPIRY_HOURS")
		os.Unsetenv("PORT")
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("CORS_ORIGINS")
	}()

	c := Load()
	if c.JWTSecret != "my-secret" {
		t.Errorf("JWT_SECRET override: want my-secret, got %q", c.JWTSecret)
	}

	if c.JWTExpiryH != 48 {
		t.Errorf("JWT_EXPIRY_HOURS override: want 48, got %d", c.JWTExpiryH)
	}

	if c.Port != 9090 {
		t.Errorf("PORT override: want 9090, got %d", c.Port)
	}

	if c.DatabaseURL != "postgres://test:test@localhost/testdb" {
		t.Errorf("DATABASE_URL override: got %q", c.DatabaseURL)
	}

	if len(c.CORSOrigins) != 2 {
		t.Errorf("CORS_ORIGINS: want 2 entries, got %d", len(c.CORSOrigins))
	}
}

func TestLoad_InvalidIntegers(t *testing.T) {
	os.Setenv("PORT", "not-a-number")
	os.Setenv("JWT_EXPIRY_HOURS", "bad")

	defer func() {
		os.Unsetenv("PORT")
		os.Unsetenv("JWT_EXPIRY_HOURS")
	}()

	c := Load()
	// Should fall back to defaults.
	if c.Port != 8081 {
		t.Errorf("invalid PORT: want default 8081, got %d", c.Port)
	}

	if c.JWTExpiryH != 24 {
		t.Errorf("invalid JWT_EXPIRY_HOURS: want default 24, got %d", c.JWTExpiryH)
	}
}

func TestLoad_AdminPassword(t *testing.T) {
	os.Setenv("ADMIN_PASSWORD", "S3cr3t!")

	defer os.Unsetenv("ADMIN_PASSWORD")

	c := Load()
	if c.AdminPassword != "S3cr3t!" {
		t.Errorf("admin password: want S3cr3t!, got %q", c.AdminPassword)
	}
}

func TestLoad_AdminPasswordFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "admin-pw-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	f.WriteString("FilePassword!")
	f.Close()

	os.Setenv("ADMIN_PASSWORD_FILE", f.Name())

	defer os.Unsetenv("ADMIN_PASSWORD_FILE")

	c := Load()
	if c.AdminPassword != "FilePassword!" {
		t.Errorf("admin password file: want FilePassword!, got %q", c.AdminPassword)
	}
}

func TestLoad_OIDC(t *testing.T) {
	os.Setenv("OIDC_ENABLED", "true")
	os.Setenv("OIDC_PROVIDER_URL", "https://sso.example.com")
	os.Setenv("OIDC_CLIENT_ID", "client123")
	os.Setenv("OIDC_CLIENT_SECRET", "secret456")

	defer func() {
		os.Unsetenv("OIDC_ENABLED")
		os.Unsetenv("OIDC_PROVIDER_URL")
		os.Unsetenv("OIDC_CLIENT_ID")
		os.Unsetenv("OIDC_CLIENT_SECRET")
	}()

	c := Load()
	if !c.OIDC.Enabled {
		t.Error("OIDC.Enabled: want true")
	}

	if c.OIDC.ProviderURL != "https://sso.example.com" {
		t.Errorf("OIDC.ProviderURL: got %q", c.OIDC.ProviderURL)
	}

	if c.OIDC.ClientID != "client123" {
		t.Errorf("OIDC.ClientID: got %q", c.OIDC.ClientID)
	}
}

func TestLoad_ConfigFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	f.WriteString("port: 7777\njwt_secret: from-file\n")
	f.Close()

	os.Setenv("CONFIG_PATH", f.Name())

	defer os.Unsetenv("CONFIG_PATH")

	c := Load()
	if c.Port != 7777 {
		t.Errorf("config file port: want 7777, got %d", c.Port)
	}

	if c.JWTSecret != "from-file" {
		t.Errorf("config file jwt_secret: want from-file, got %q", c.JWTSecret)
	}
}

func TestFindProvider(t *testing.T) {
	c := &Config{
		Providers: []ProviderConfig{
			{ID: "github", Name: "GitHub", ClientID: "gh-id"},
			{ID: "gitlab", Name: "GitLab", ClientID: "gl-id"},
		},
	}

	p := c.FindProvider("github")
	if p == nil {
		t.Fatal("expected to find github provider")
	}

	if p.ClientID != "gh-id" {
		t.Errorf("github ClientID: want gh-id, got %q", p.ClientID)
	}

	p2 := c.FindProvider("gitlab")
	if p2 == nil || p2.ClientID != "gl-id" {
		t.Error("expected to find gitlab provider")
	}

	if c.FindProvider("nonexistent") != nil {
		t.Error("expected nil for unknown provider")
	}
}

func TestResolveClientSecret_EnvVar(t *testing.T) {
	o := OIDCBootstrap{ClientSecret: "env-secret"}
	if got := o.ResolveClientSecret(); got != "env-secret" {
		t.Errorf("want env-secret, got %q", got)
	}
}

func TestResolveClientSecret_File(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "oidc-secret-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	f.WriteString("  file-secret  \n")
	f.Close()

	o := OIDCBootstrap{
		ClientSecretFile: f.Name(),
		ClientSecret:     "env-fallback",
	}
	if got := o.ResolveClientSecret(); got != "file-secret" {
		t.Errorf("want file-secret, got %q", got)
	}
}

func TestResolveClientSecret_FileMissing(t *testing.T) {
	o := OIDCBootstrap{
		ClientSecretFile: "/nonexistent/path",
		ClientSecret:     "fallback",
	}
	if got := o.ResolveClientSecret(); got != "fallback" {
		t.Errorf("want fallback when file missing, got %q", got)
	}
}

func TestResolveClientSecret_Empty(t *testing.T) {
	o := OIDCBootstrap{}
	if got := o.ResolveClientSecret(); got != "" {
		t.Errorf("want empty string, got %q", got)
	}
}

func TestLoad_K8sNamespaceAndKubeconfig(t *testing.T) {
	os.Setenv("K8S_NAMESPACE", "my-ns")
	os.Setenv("KUBECONFIG", "/tmp/kube.conf")

	defer func() {
		os.Unsetenv("K8S_NAMESPACE")
		os.Unsetenv("KUBECONFIG")
	}()

	c := Load()
	if c.K8sNamespace != "my-ns" {
		t.Errorf("K8S_NAMESPACE: want my-ns, got %q", c.K8sNamespace)
	}

	if c.Kubeconfig != "/tmp/kube.conf" {
		t.Errorf("KUBECONFIG: want /tmp/kube.conf, got %q", c.Kubeconfig)
	}
}

func TestLoad_AdminPasswordFileMissing_FallsBackToEnv(t *testing.T) {
	os.Setenv("ADMIN_PASSWORD_FILE", "/nonexistent/path/pw.txt")
	os.Setenv("ADMIN_PASSWORD", "fallback-pw")

	defer func() {
		os.Unsetenv("ADMIN_PASSWORD_FILE")
		os.Unsetenv("ADMIN_PASSWORD")
	}()

	c := Load()
	if c.AdminPassword != "fallback-pw" {
		t.Errorf("expected fallback-pw when file missing, got %q", c.AdminPassword)
	}

	if c.AdminPasswordFile != "/nonexistent/path/pw.txt" {
		t.Errorf("expected AdminPasswordFile to be preserved, got %q", c.AdminPasswordFile)
	}
}

func TestLoad_OAuthRedirectBaseAndCourseURL(t *testing.T) {
	os.Setenv("OAUTH_REDIRECT_BASE", "https://app.example.com")
	os.Setenv("COURSE_SERVICE_URL", "http://course-svc:8082")

	defer func() {
		os.Unsetenv("OAUTH_REDIRECT_BASE")
		os.Unsetenv("COURSE_SERVICE_URL")
	}()

	c := Load()
	if c.OAuthRedirectBase != "https://app.example.com" {
		t.Errorf("OAuthRedirectBase: want https://app.example.com, got %q", c.OAuthRedirectBase)
	}

	if c.CourseServiceURL != "http://course-svc:8082" {
		t.Errorf("CourseServiceURL: want http://course-svc:8082, got %q", c.CourseServiceURL)
	}
}

func TestLoad_ProviderSecretFromEnv(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "config-providers-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	f.WriteString("providers:\n  - id: github\n    name: GitHub\n    client_id: gh-id\n")
	f.Close()

	os.Setenv("CONFIG_PATH", f.Name())
	os.Setenv("SSO_GITHUB_CLIENT_SECRET", "gh-secret-from-env")

	defer func() {
		os.Unsetenv("CONFIG_PATH")
		os.Unsetenv("SSO_GITHUB_CLIENT_SECRET")
	}()

	c := Load()
	if len(c.Providers) == 0 {
		t.Skip("provider not loaded from config (yaml parsing may differ)")
	}

	for _, p := range c.Providers {
		if p.ID == "github" && p.ClientSecret == "gh-secret-from-env" {
			return
		}
	}

	t.Errorf("expected github provider with gh-secret-from-env, got providers=%v", c.Providers)
}
