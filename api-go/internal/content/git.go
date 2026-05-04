package content

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// SyncRepo clones or pulls a git repository and loads its courses into the store.
// token is the decrypted PAT (empty string for public repos).
// repoDir is where the repo should be cloned on disk.
// source is the canonical URL stored on the course (used to replace old courses on re-sync).
func (s *Store) SyncRepo(rawURL, branch, token, repoDir, source string) error {
	authURL := buildAuthURL(rawURL, token)

	if _, err := os.Stat(filepath.Join(repoDir, ".git")); os.IsNotExist(err) {
		if err := os.MkdirAll(repoDir, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", repoDir, err)
		}
		slog.Info("cloning repository", "url", rawURL, "branch", branch)
		cmd := exec.Command("git", "clone", "--depth=1", "--branch="+branch, authURL, repoDir)
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("git clone: %w", err)
		}
	} else {
		slog.Info("pulling repository", "url", rawURL, "branch", branch)
		fetch := exec.Command("git", "-C", repoDir, "fetch", "--depth=1", "origin", branch)
		fetch.Stderr = os.Stderr
		if err := fetch.Run(); err != nil {
			return fmt.Errorf("git fetch: %w", err)
		}
		reset := exec.Command("git", "-C", repoDir, "reset", "--hard", "origin/"+branch)
		reset.Stderr = os.Stderr
		if err := reset.Run(); err != nil {
			return fmt.Errorf("git reset: %w", err)
		}
	}

	coursesDir := filepath.Join(repoDir, "courses")
	if _, err := os.Stat(coursesDir); os.IsNotExist(err) {
		return fmt.Errorf("repository has no courses/ directory")
	}

	// Remove stale courses from this source before reloading
	s.DeleteBySource(source)
	return s.LoadDir(coursesDir, source)
}

// buildAuthURL injects a token into an HTTPS URL for authentication.
// For GitHub and GitLab PATs: https://{token}@host/path
func buildAuthURL(rawURL, token string) string {
	if token == "" {
		return rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" {
		return rawURL
	}
	if !strings.HasPrefix(u.Scheme, "http") {
		return rawURL
	}
	u.User = url.UserPassword("oauth2", token)
	return u.String()
}
