package content

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type cachedRepo struct {
	path        string
	clonedAt    time.Time
	failedUntil time.Time
}

type GitCache struct {
	mu             sync.Mutex
	cacheDir       string
	ttl            time.Duration
	failureBackoff time.Duration
	repos          map[string]*cachedRepo
	cloning        map[string]chan struct{}
}

func NewGitCache(cacheDir string, ttl time.Duration) *GitCache {
	os.MkdirAll(cacheDir, 0755)
	return &GitCache{
		cacheDir:       cacheDir,
		ttl:            ttl,
		failureBackoff: 30 * time.Second,
		repos:          make(map[string]*cachedRepo),
		cloning:        make(map[string]chan struct{}),
	}
}

func (gc *GitCache) cacheKey(rawURL, branch string) string {
	h := sha256.Sum256([]byte(rawURL + ":" + branch))
	return fmt.Sprintf("%x", h[:16])
}

func (gc *GitCache) FetchModuleContent(rawURL, branch, filePath, token string) ([]byte, error) {
	key := gc.cacheKey(rawURL, branch)

	repoDir, err := gc.getOrClone(key, rawURL, branch, token)
	if err != nil {
		return nil, err
	}

	fullPath := filepath.Join(repoDir, filePath)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("read file %s from cached repo: %w", filePath, err)
	}
	return data, nil
}

func (gc *GitCache) getOrClone(key, rawURL, branch, token string) (string, error) {
	gc.mu.Lock()

	if cr, ok := gc.repos[key]; ok {
		if cr.path != "" && time.Since(cr.clonedAt) < gc.ttl {
			path := cr.path
			gc.mu.Unlock()
			return path, nil
		}
		if !cr.failedUntil.IsZero() && time.Now().Before(cr.failedUntil) {
			gc.mu.Unlock()
			return "", fmt.Errorf("git clone failed recently, retry in %v", time.Until(cr.failedUntil))
		}
	}

	if wait, ok := gc.cloning[key]; ok {
		gc.mu.Unlock()
		<-wait
		gc.mu.Lock()
		if cr, ok := gc.repos[key]; ok {
			if cr.path != "" {
				path := cr.path
				gc.mu.Unlock()
				return path, nil
			}
			if !cr.failedUntil.IsZero() && time.Now().Before(cr.failedUntil) {
				gc.mu.Unlock()
				return "", fmt.Errorf("git clone failed recently, retry in %v", time.Until(cr.failedUntil))
			}
		}
		gc.mu.Unlock()
		return gc.getOrClone(key, rawURL, branch, token)
	}

	done := make(chan struct{})
	gc.cloning[key] = done
	gc.mu.Unlock()

	repoDir := filepath.Join(gc.cacheDir, key)
	authURL := buildAuthURL(rawURL, token)

	slog.Debug("cloning repo into cache", "url", rawURL, "branch", branch, "dir", repoDir)

	if _, err := os.Stat(repoDir); err == nil {
		os.RemoveAll(repoDir)
	}

	var stderr bytes.Buffer
	cmd := exec.Command("git", "clone", "--depth=1", "--branch="+branch, authURL, repoDir)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		os.RemoveAll(repoDir)

		gc.mu.Lock()
		gc.repos[key] = &cachedRepo{failedUntil: time.Now().Add(gc.failureBackoff)}
		delete(gc.cloning, key)
		gc.mu.Unlock()
		close(done)

		slog.Error("git clone failed", "stderr", stderr.String())
		return "", fmt.Errorf("git clone failed: %s", sanitizeGitOutput(stderr.String(), token))
	}

	gc.mu.Lock()
	gc.repos[key] = &cachedRepo{path: repoDir, clonedAt: time.Now()}
	delete(gc.cloning, key)
	gc.mu.Unlock()
	close(done)

	return repoDir, nil
}

func (gc *GitCache) Clear() {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	for _, cr := range gc.repos {
		if cr.path != "" {
			os.RemoveAll(cr.path)
		}
	}
	gc.repos = make(map[string]*cachedRepo)
}

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

var globalGitCache = NewGitCache(filepath.Join(os.TempDir(), "elearning-git-cache"), 10*time.Minute)

func SetGlobalGitCache(gc *GitCache) {
	globalGitCache = gc
}

func FetchModuleContent(rawURL, branch, filePath, token string) ([]byte, error) {
	return globalGitCache.FetchModuleContent(rawURL, branch, filePath, token)
}

func fetchGitRaw(rawURL, branch, filePath, token string) ([]byte, error) {
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
