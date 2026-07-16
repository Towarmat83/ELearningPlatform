// Package config provides application configuration loading for
// user-service, merging built-in defaults, an optional YAML file, and
// environment variable overrides.
package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

const (
	// defaultJWTSecret is the fallback JWT signing secret; it must be
	// overridden in production via the JWT_SECRET env var or config file.
	defaultJWTSecret = "change-me-in-production-use-a-long-random-string"
	// defaultInternalSecret is the fallback service-to-service shared secret;
	// it must be overridden in production via the INTERNAL_SERVICE_SECRET env var.
	defaultInternalSecret = "change-me-internal-secret"
	// defaultJWTExpiryHours is the default JWT token lifetime, in hours.
	defaultJWTExpiryHours = 8
	// defaultPort is the default HTTP listen port.
	defaultPort = 8081
	// defaultLocalOrigin is the default local frontend origin, used as a
	// CORS origin and as the default OAuth redirect base.
	defaultLocalOrigin = "http://localhost:3000"
	// envValueTrue is the value a boolean env var (OIDC_ENABLED,
	// OIDC_INSECURE_SKIP_VERIFY) must equal to be considered enabled.
	envValueTrue = "true"
	// defaultAuthRateLimitRequests is the default auth request cap per IP.
	defaultAuthRateLimitRequests = 10
	// defaultAuthRateLimitWindowSeconds is the auth rate-limit window length.
	defaultAuthRateLimitWindowSeconds = 60
)

// ProviderConfig holds the configuration for a single SSO/OAuth provider
// (e.g. GitHub, GitLab), as loaded from the optional YAML config file.
type ProviderConfig struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name"`

	ClientID string `yaml:"clientId"`

	ClientSecret string `yaml:"clientSecret"`

	IssuerURL string `yaml:"issuerUrl"`
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
	if o.ClientSecretFile == "" {
		return o.ClientSecret
	}

	data, err := os.ReadFile(o.ClientSecretFile)
	if err != nil {
		return o.ClientSecret
	}

	if s := strings.TrimSpace(string(data)); s != "" {
		return s
	}

	return o.ClientSecret
}

// Config holds the application configuration for user-service.
type Config struct {
	DatabaseURL string `yaml:"databaseUrl"`

	JWTSecret string `yaml:"jwtSecret"`

	// OAuthStateSecret signs OAuth CSRF state JWTs. When empty, JWTSecret is
	// used as fallback so existing deployments without this env var keep working.
	OAuthStateSecret string `yaml:"oauthStateSecret"`

	JWTExpiryH int `yaml:"jwtExpiryHours"`
	Port       int `yaml:"port"`

	CORSOrigins []string `yaml:"corsOrigins"`

	OAuthRedirectBase string           `yaml:"oauthRedirectBase"`
	Providers         []ProviderConfig `yaml:"providers"`

	CourseServiceURL string `yaml:"courseServiceUrl"`

	// InternalSecret is the shared secret used to authenticate service-to-service
	// calls on /internal/* routes (X-Internal-Secret header).
	InternalSecret string `yaml:"-"`

	K8sNamespace string `yaml:"k8sNamespace"`
	Kubeconfig   string `yaml:"kubeconfig"`
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

	// AuthRateLimitRequests is the max number of auth requests allowed per IP
	// per AuthRateLimitWindowSeconds. Set AUTH_RATE_LIMIT_REQUESTS=0 to disable.
	AuthRateLimitRequests      int `yaml:"authRateLimitRequests"`
	AuthRateLimitWindowSeconds int `yaml:"authRateLimitWindowSeconds"`
}

// FindProvider returns the ProviderConfig with the given id, or nil if no
// such provider is configured.
func (c *Config) FindProvider(id string) *ProviderConfig {
	for i := range c.Providers {
		if c.Providers[i].ID == id {
			return &c.Providers[i]
		}
	}

	return nil
}

// Load builds a Config from built-in defaults, then overlays an optional
// YAML file referenced by CONFIG_PATH, then overlays environment variable
// overrides.
func Load() *Config {
	_ = godotenv.Load()

	cfg := defaultConfig()

	loadFromFile(cfg)

	cfg.DatabaseURL = stringFromEnv("DATABASE_URL", cfg.DatabaseURL)
	cfg.JWTSecret = stringFromEnv("JWT_SECRET", cfg.JWTSecret)
	cfg.OAuthStateSecret = stringFromEnv("OAUTH_STATE_SECRET", cfg.JWTSecret)
	cfg.JWTExpiryH = intFromEnv("JWT_EXPIRY_HOURS", cfg.JWTExpiryH)
	cfg.Port = intFromEnv("PORT", cfg.Port)
	cfg.CORSOrigins = sliceFromEnv("CORS_ORIGINS", cfg.CORSOrigins)
	cfg.OAuthRedirectBase = stringFromEnv("OAUTH_REDIRECT_BASE", cfg.OAuthRedirectBase)
	cfg.CourseServiceURL = stringFromEnv("COURSE_SERVICE_URL", cfg.CourseServiceURL)
	cfg.InternalSecret = stringFromEnv("INTERNAL_SERVICE_SECRET", cfg.InternalSecret)
	cfg.K8sNamespace = stringFromEnv("K8S_NAMESPACE", cfg.K8sNamespace)
	cfg.Kubeconfig = stringFromEnv("KUBECONFIG", cfg.Kubeconfig)
	cfg.AuthRateLimitRequests = intFromEnv("AUTH_RATE_LIMIT_REQUESTS", cfg.AuthRateLimitRequests)
	cfg.AuthRateLimitWindowSeconds = intFromEnv("AUTH_RATE_LIMIT_WINDOW_SECONDS", cfg.AuthRateLimitWindowSeconds)

	loadAdminPassword(cfg)

	cfg.OIDC = loadOIDCBootstrap()

	loadProviderSecrets(cfg)

	return cfg
}

// defaultConfig returns a Config populated with the built-in default
// values.
func defaultConfig() *Config {
	return &Config{
		DatabaseURL:                "postgres://pupitre:pupitre@localhost:5432/pupitre",
		JWTSecret:                  defaultJWTSecret,
		InternalSecret:             defaultInternalSecret,
		JWTExpiryH:                 defaultJWTExpiryHours,
		Port:                       defaultPort,
		CORSOrigins:                []string{defaultLocalOrigin, "http://localhost:5173"},
		OAuthRedirectBase:          defaultLocalOrigin,
		CourseServiceURL:           "http://course-service:8082",
		AuthRateLimitRequests:      defaultAuthRateLimitRequests,
		AuthRateLimitWindowSeconds: defaultAuthRateLimitWindowSeconds,
	}
}

// loadFromFile overlays cfg with values from the YAML file referenced by
// the CONFIG_PATH environment variable, if set. Errors are ignored: an
// absent or unreadable config file simply leaves cfg at its current
// values.
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

// loadAdminPassword resolves cfg.AdminPassword and cfg.AdminPasswordFile
// from the ADMIN_PASSWORD_FILE and ADMIN_PASSWORD env vars.
//
// ADMIN_PASSWORD_FILE takes priority: the initial value is read from the
// mounted secret file so a watcher goroutine can detect future changes.
// ADMIN_PASSWORD is used as a fallback, or when no file path is
// configured.
func loadAdminPassword(cfg *Config) {
	if pwFile := os.Getenv("ADMIN_PASSWORD_FILE"); pwFile != "" {
		cfg.AdminPasswordFile = pwFile

		data, err := os.ReadFile(pwFile) //nolint:gosec // path is operator-controlled via ADMIN_PASSWORD_FILE, not user input
		if err == nil {
			cfg.AdminPassword = strings.TrimSpace(string(data))
		}
		// If the file is not readable yet (e.g. volume not mounted), fall
		// through to the ADMIN_PASSWORD env var below; the path is kept so
		// the watcher goroutine can retry later.
	}

	if cfg.AdminPassword == "" {
		cfg.AdminPassword = os.Getenv("ADMIN_PASSWORD")
	}
}

// loadOIDCBootstrap builds the OIDC bootstrap settings (Helm) from env
// vars. The client secret is sourced from a mounted Secret file
// (preferred) or an env var, so it never has to be hardcoded in chart
// values.
func loadOIDCBootstrap() OIDCBootstrap {
	return OIDCBootstrap{
		Enabled:            os.Getenv("OIDC_ENABLED") == envValueTrue,
		ProviderURL:        os.Getenv("OIDC_PROVIDER_URL"),
		IssuerURL:          os.Getenv("OIDC_ISSUER_URL"),
		ClientID:           os.Getenv("OIDC_CLIENT_ID"),
		ClientSecret:       os.Getenv("OIDC_CLIENT_SECRET"),
		ClientSecretFile:   os.Getenv("OIDC_CLIENT_SECRET_FILE"),
		Scopes:             os.Getenv("OIDC_SCOPES"),
		GroupClaim:         os.Getenv("OIDC_GROUP_CLAIM"),
		RedirectBase:       os.Getenv("OIDC_REDIRECT_BASE"),
		BrowserBaseURL:     os.Getenv("OIDC_BROWSER_BASE_URL"),
		InsecureSkipVerify: os.Getenv("OIDC_INSECURE_SKIP_VERIFY") == envValueTrue,
	}
}

// loadProviderSecrets overlays each configured provider's ClientSecret from
// a Kubernetes Secret rather than the ConfigMap. Convention, per provider
// id:
//
//	SSO_<ID>_CLIENT_SECRET_FILE — path to a mounted secret file (priority)
//	SSO_<ID>_CLIENT_SECRET      — direct value
func loadProviderSecrets(cfg *Config) {
	for idx := range cfg.Providers {
		envKey := "SSO_" + strings.ToUpper(cfg.Providers[idx].ID) + "_CLIENT_SECRET"

		if secretFile := os.Getenv(envKey + "_FILE"); secretFile != "" {
			data, err := os.ReadFile(secretFile) //nolint:gosec // path is operator-controlled via SSO_<ID>_CLIENT_SECRET_FILE, not user input
			if err == nil {
				cfg.Providers[idx].ClientSecret = strings.TrimSpace(string(data))
			}

			continue
		}

		if v := os.Getenv(envKey); v != "" {
			cfg.Providers[idx].ClientSecret = v
		}
	}
}
