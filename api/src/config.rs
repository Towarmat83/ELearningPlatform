use std::env;

#[derive(Clone, Debug)]
pub struct Config {
    pub database_url: String,
    pub jwt_secret: String,
    pub jwt_expiry_hours: u64,
    pub port: u16,
    pub cors_origins: Vec<String>,

    // OAuth2 SSO
    pub gitlab_client_id: Option<String>,
    pub gitlab_client_secret: Option<String>,
    /// Base URL of the GitLab instance, e.g. https://gitlab.com (or self-hosted)
    pub gitlab_url: String,
    pub github_client_id: Option<String>,
    pub github_client_secret: Option<String>,
    /// Base URL of the frontend, e.g. http://localhost:3000
    /// The OAuth redirect_uri will be: {oauth_redirect_base}/auth/callback
    pub oauth_redirect_base: String,
}

impl Config {
    pub fn from_env() -> anyhow::Result<Self> {
        Ok(Self {
            database_url: env::var("DATABASE_URL")
                .unwrap_or_else(|_| "postgres://elearning:elearning@localhost:5432/elearning".to_string()),
            jwt_secret: env::var("JWT_SECRET")
                .unwrap_or_else(|_| "change-me-in-production-use-a-long-random-string".to_string()),
            jwt_expiry_hours: env::var("JWT_EXPIRY_HOURS")
                .unwrap_or_else(|_| "24".to_string())
                .parse()?,
            port: env::var("PORT")
                .unwrap_or_else(|_| "8080".to_string())
                .parse()?,
            cors_origins: env::var("CORS_ORIGINS")
                .unwrap_or_else(|_| "http://localhost:3000,http://localhost:5173".to_string())
                .split(',')
                .map(|s| s.trim().to_string())
                .collect(),

            gitlab_client_id: env::var("GITLAB_CLIENT_ID").ok().filter(|s| !s.is_empty()),
            gitlab_client_secret: env::var("GITLAB_CLIENT_SECRET").ok().filter(|s| !s.is_empty()),
            gitlab_url: env::var("GITLAB_URL")
                .unwrap_or_else(|_| "https://gitlab.com".to_string())
                .trim_end_matches('/')
                .to_string(),
            github_client_id: env::var("GITHUB_CLIENT_ID").ok().filter(|s| !s.is_empty()),
            github_client_secret: env::var("GITHUB_CLIENT_SECRET").ok().filter(|s| !s.is_empty()),
            oauth_redirect_base: env::var("OAUTH_REDIRECT_BASE")
                .unwrap_or_else(|_| "http://localhost:3000".to_string()),
        })
    }
}
