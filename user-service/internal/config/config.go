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

// OIDCBootstrap holds the OIDC settings provisioned at deploy time (Helm).
// When OIDC_ENABLED=true these values are seeded into platform_settings on
// startup (see db.SeedOIDC), so the integration works out of the box without
// anyone having to type the client secret into the admin UI. The secret itself
// is sourced from a mounted Kubernetes Secret (file) or env var — never from
// hardcoded chart values.
type OIDCBootstrap struct {
	Enabled            bool
	ProviderURL        string
	IssuerURL          string
	ClientID           string
	ClientSecret       string // value supplied via OIDC_CLIENT_SECRET
	ClientSecretFile   string // path to a mounted secret file (takes priority)
	Scopes             string
	GroupClaim         string
	RedirectBase       string
	BrowserBaseURL     string
	InsecureSkipVerify bool // skip TLS certificate verification (custom CA / self-signed)
}

// ResolveClientSecret returns the OIDC client secret, preferring the mounted
// secret file (so the value never has to live in an env var) and falling back
// to the OIDC_CLIENT_SECRET env var.
func (o OIDCBootstrap) ResolveClientSecret() string {
	if o.ClientSecretFile != "" {
		if data, err := os.ReadFile(o.ClientSecretFile); err == nil {
			if s := strings.TrimSpace(string(data)); s != "" {
				return s
			}
		}
	}
	return o.ClientSecret
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
	K8sNamespace      string           `yaml:"k8s_namespace"`
	Kubeconfig        string           `yaml:"kubeconfig"`
	// AdminPassword is the password (or pre-computed bcrypt hash) for the
	// bootstrapped admin account.
	// Resolution order:
	//  1. ADMIN_PASSWORD_FILE — path to a mounted secret file (K8s volume).
	//     The file is watched for changes so no pod restart is needed.
	//  2. ADMIN_PASSWORD — direct env var (set once at startup).
	//  3. Neither set — hardcoded default "Admin@1234" with a loud warning.
	AdminPassword     string `yaml:"-"`
	AdminPasswordFile string `yaml:"-"` // path to the mounted secret file, if any
	// OIDC holds deploy-time OIDC configuration (Helm). Seeded into
	// platform_settings on startup when Enabled.
	OIDC OIDCBootstrap `yaml:"-"`
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

	_ = godotenv.Load()

	if path := os.Getenv("CONFIG_PATH"); path != "" {
		if data, err := os.ReadFile(path); err == nil {
			_ = yaml.Unmarshal(data, c)
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
	if v := os.Getenv("K8S_NAMESPACE"); v != "" {
		c.K8sNamespace = v
	}
	if v := os.Getenv("KUBECONFIG"); v != "" {
		c.Kubeconfig = v
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

	// ── OIDC bootstrap (Helm) ──────────────────────────────────────────────
	// The client secret is sourced from a mounted Secret file (preferred) or an
	// env var, so it never has to be hardcoded in chart values.
	c.OIDC = OIDCBootstrap{
		Enabled:            os.Getenv("OIDC_ENABLED") == "true",
		ProviderURL:        os.Getenv("OIDC_PROVIDER_URL"),
		IssuerURL:          os.Getenv("OIDC_ISSUER_URL"),
		ClientID:           os.Getenv("OIDC_CLIENT_ID"),
		ClientSecret:       os.Getenv("OIDC_CLIENT_SECRET"),
		ClientSecretFile:   os.Getenv("OIDC_CLIENT_SECRET_FILE"),
		Scopes:             os.Getenv("OIDC_SCOPES"),
		GroupClaim:         os.Getenv("OIDC_GROUP_CLAIM"),
		RedirectBase:       os.Getenv("OIDC_REDIRECT_BASE"),
		BrowserBaseURL:     os.Getenv("OIDC_BROWSER_BASE_URL"),
		InsecureSkipVerify: os.Getenv("OIDC_INSECURE_SKIP_VERIFY") == "true",
	}

	// ── OAuth provider secrets (Helm) ──────────────────────────────────────
	// Provider client secrets are injected from a Kubernetes Secret rather than
	// baked into the ConfigMap. Convention, per provider id:
	//   SSO_<ID>_CLIENT_SECRET_FILE  — path to a mounted secret file (priority)
	//   SSO_<ID>_CLIENT_SECRET       — direct value
	for i := range c.Providers {
		envKey := "SSO_" + strings.ToUpper(c.Providers[i].ID) + "_CLIENT_SECRET"
		if f := os.Getenv(envKey + "_FILE"); f != "" {
			if data, err := os.ReadFile(f); err == nil {
				c.Providers[i].ClientSecret = strings.TrimSpace(string(data))
			}
		} else if v := os.Getenv(envKey); v != "" {
			c.Providers[i].ClientSecret = v
		}
	}

	return c
}
