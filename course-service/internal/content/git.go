package content

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// gitCacheFailureBackoff is how long a failed clone blocks retries for
// the same repository key.
const gitCacheFailureBackoff = 30 * time.Second

// gitCacheDirPerm is the permission used when creating the cache
// directory.
const gitCacheDirPerm = 0o750

// gitNoCloneOutput is returned by sanitizeGitOutput when git produced no
// output at all.
const gitNoCloneOutput = "(no output)"

// cachedRepo tracks the on-disk location and health of a single cloned
// repository.
type cachedRepo struct {
	path        string
	clonedAt    time.Time
	failedUntil time.Time
}

// GitCache clones and caches git repositories on disk, deduplicating
// concurrent clones of the same repository/branch.
type GitCache struct {
	mu             sync.Mutex
	cacheDir       string
	ttl            time.Duration
	failureBackoff time.Duration
	repos          map[string]*cachedRepo
	cloning        map[string]chan struct{}
}

// NewGitCache creates a GitCache that stores clones under cacheDir and
// treats them as fresh for ttl.
func NewGitCache(cacheDir string, ttl time.Duration) *GitCache {
	_ = os.MkdirAll(cacheDir, gitCacheDirPerm)

	return &GitCache{
		cacheDir:       cacheDir,
		ttl:            ttl,
		failureBackoff: gitCacheFailureBackoff,
		repos:          make(map[string]*cachedRepo),
		cloning:        make(map[string]chan struct{}),
	}
}

// FetchModuleContent clones (or reuses a cached clone of) rawURL at
// branch and returns the contents of filePath within it.
func (gc *GitCache) FetchModuleContent(rawURL, branch, filePath, token string) ([]byte, error) {
	key := gc.cacheKey(rawURL, branch)

	repoDir, err := gc.getOrClone(key, rawURL, branch, token)
	if err != nil {
		return nil, err
	}

	fullPath := filepath.Join(repoDir, filePath)

	data, err := os.ReadFile(filepath.Clean(fullPath))
	if err != nil {
		return nil, fmt.Errorf("read file %s from cached repo: %w", filePath, err)
	}

	return data, nil
}

// Clear removes every cached repository from disk and forgets them.
func (gc *GitCache) Clear() {
	gc.mu.Lock()
	defer gc.mu.Unlock()

	for _, cached := range gc.repos {
		if cached.path != "" {
			_ = os.RemoveAll(cached.path)
		}
	}

	gc.repos = make(map[string]*cachedRepo)
}

// ClearRepo removes a single cached repository identified by its URL and
// branch.
func (gc *GitCache) ClearRepo(rawURL, branch string) {
	key := gc.cacheKey(rawURL, branch)
	gc.mu.Lock()
	defer gc.mu.Unlock()

	if cached, ok := gc.repos[key]; ok {
		if cached.path != "" {
			_ = os.RemoveAll(cached.path)
		}

		delete(gc.repos, key)
	}
}

// cacheKey derives the cache map key for a repository URL and branch.
func (gc *GitCache) cacheKey(rawURL, branch string) string {
	h := sha256.Sum256([]byte(rawURL + ":" + branch))

	return hex.EncodeToString(h[:16])
}

// getOrClone returns the local path for rawURL/branch, cloning it first
// if it is not already cached and fresh.
func (gc *GitCache) getOrClone(key, rawURL, branch, token string) (string, error) {
	gc.mu.Lock()

	if cached, ok := gc.repos[key]; ok {
		if path, fresh := freshCachedPath(cached, gc.ttl); fresh {
			gc.mu.Unlock()

			return path, nil
		}

		failErr := recentCloneFailure(cached)
		if failErr != nil {
			gc.mu.Unlock()

			return "", failErr
		}
	}

	if wait, waiting := gc.cloning[key]; waiting {
		return gc.awaitClone(key, rawURL, branch, token, wait)
	}

	done := make(chan struct{})
	gc.cloning[key] = done
	gc.mu.Unlock()

	return gc.cloneAndCache(key, rawURL, branch, token, done)
}

// awaitClone blocks until an in-flight clone of key finishes, then reuses
// its result or retries getOrClone. gc.mu must be held on entry.
func (gc *GitCache) awaitClone(key, rawURL, branch, token string, wait chan struct{}) (string, error) {
	gc.mu.Unlock()
	<-wait
	gc.mu.Lock()

	if cached, ok := gc.repos[key]; ok {
		if cached.path != "" {
			path := cached.path
			gc.mu.Unlock()

			return path, nil
		}

		failErr := recentCloneFailure(cached)
		if failErr != nil {
			gc.mu.Unlock()

			return "", failErr
		}
	}

	gc.mu.Unlock()

	return gc.getOrClone(key, rawURL, branch, token)
}

// cloneAndCache clones rawURL/branch into the cache directory, records
// the outcome under key, and wakes any goroutines waiting on done.
// gc.mu must not be held on entry.
func (gc *GitCache) cloneAndCache(key, rawURL, branch, token string, done chan struct{}) (string, error) {
	repoDir := filepath.Join(gc.cacheDir, key)
	authURL := buildAuthURL(rawURL, token)

	slog.Debug("cloning repo into cache", "url", rawURL, "branch", branch, "dir", repoDir)

	_, statErr := os.Stat(repoDir)
	if statErr == nil {
		_ = os.RemoveAll(repoDir)
	}

	var stderr bytes.Buffer

	//nolint:gosec // rawURL/branch are operator-supplied course sources;
	// args are passed discretely to exec, not through a shell.
	cmd := exec.CommandContext(context.Background(), "git", "clone", "--depth=1", "--branch="+branch, authURL, repoDir)

	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		_ = os.RemoveAll(repoDir)

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

// freshCachedPath returns cached's path when present and still within
// ttl, and whether it was returned.
func freshCachedPath(cached *cachedRepo, ttl time.Duration) (string, bool) {
	if cached.path != "" && time.Since(cached.clonedAt) < ttl {
		return cached.path, true
	}

	return "", false
}

// recentCloneFailure returns a non-nil error if cached recorded a clone
// failure that is still within its backoff window.
func recentCloneFailure(cached *cachedRepo) error {
	if !cached.failedUntil.IsZero() && time.Now().Before(cached.failedUntil) {
		return fmt.Errorf("git clone failed recently, retry in %v", time.Until(cached.failedUntil))
	}

	return nil
}

// buildAuthURL embeds token as an OAuth2 basic-auth credential in rawURL
// when it is a plain HTTP(S) URL.
func buildAuthURL(rawURL, token string) string {
	if token == "" {
		return rawURL
	}

	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" {
		return rawURL
	}

	if !strings.HasPrefix(parsed.Scheme, "http") {
		return rawURL
	}

	parsed.User = url.UserPassword("oauth2", token)

	return parsed.String()
}

// sanitizeGitOutput trims output and redacts token from it, if present.
func sanitizeGitOutput(output, token string) string {
	out := strings.TrimSpace(output)
	if token != "" && strings.Contains(out, token) {
		out = strings.ReplaceAll(out, token, "***")
	}

	if out == "" {
		return gitNoCloneOutput
	}

	return out
}
