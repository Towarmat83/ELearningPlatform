package content

import (
	"os"
	"path"
	"strings"

	"gopkg.in/yaml.v3"
)

type Credential struct {
	URL   string `yaml:"url"`
	Token string `yaml:"token"`
}

type GitCredentialStore struct {
	entries []Credential
}

func LoadCredentials(path string) (*GitCredentialStore, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var doc struct {
		Credentials []Credential `yaml:"credentials"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}

	return &GitCredentialStore{entries: doc.Credentials}, nil
}

// Match returns the token for the first credential whose URL pattern matches repoURL.
func (s *GitCredentialStore) Match(repoURL string) string {
	if s == nil {
		return ""
	}

	for _, c := range s.entries {
		if matchGitURL(c.URL, repoURL) {
			return c.Token
		}
	}

	return ""
}

// matchGitURL checks whether pattern matches repoURL.
// Pattern is a host/path glob like "github.com/myorg/*".
func matchGitURL(pattern, repoURL string) bool {
	repoURL = strings.TrimPrefix(repoURL, "https://")
	repoURL = strings.TrimPrefix(repoURL, "http://")
	repoURL = strings.TrimPrefix(repoURL, "git@")
	pattern = strings.TrimPrefix(pattern, "https://")
	pattern = strings.TrimPrefix(pattern, "http://")
	pattern = strings.TrimPrefix(pattern, "git@")

	matched, err := path.Match(pattern, repoURL)

	return err == nil && matched
}
