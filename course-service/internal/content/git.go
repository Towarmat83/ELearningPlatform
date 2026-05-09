package content

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// FetchModuleContent clones a git repo and reads a single file.
// Returns the raw file content. The temp clone is cleaned up after reading.
func FetchModuleContent(rawURL, branch, filePath, token string) ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "elearning-git-*")
	if err != nil {
		return nil, fmt.Errorf("temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	authURL := buildAuthURL(rawURL, token)

	slog.Debug("cloning module repo", "url", rawURL, "branch", branch)
	var stderr bytes.Buffer
	cmd := exec.Command("git", "clone", "--depth=1", "--branch="+branch, authURL, tmpDir)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		slog.Error("git clone failed", "stderr", stderr.String())
		return nil, fmt.Errorf("git clone failed: %s", sanitizeGitOutput(stderr.String(), token))
	}

	fullPath := filepath.Join(tmpDir, filePath)
	f, err := os.Open(fullPath)
	if err != nil {
		return nil, fmt.Errorf("open file %s in repo: %w", filePath, err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read file %s: %w", filePath, err)
	}

	return data, nil
}

// sanitizeGitOutput removes token from git error output before surfacing it.
func sanitizeGitOutput(output, token string) string {
	out := strings.TrimSpace(output)
	if token != "" && strings.Contains(out, token) {
		out = strings.ReplaceAll(out, token, "***")
	}
	if out == "" {
		return "(no output)"
	}
	return out
}

// buildAuthURL injects a token into an HTTPS URL for authentication.
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
