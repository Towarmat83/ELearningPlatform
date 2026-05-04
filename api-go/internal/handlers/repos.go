package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/elearning/api-go/internal/content"
)

type repoResponse struct {
	ID           string  `json:"id"`
	URL          string  `json:"url"`
	Branch       string  `json:"branch"`
	HasToken     bool    `json:"has_token"`
	Status       string  `json:"status"`
	ErrorMessage *string `json:"error_message"`
	LastSyncedAt *string `json:"last_synced_at"`
	CreatedAt    string  `json:"created_at"`
}

// GET /api/my/repos
func (s *State) ListRepos(w http.ResponseWriter, r *http.Request) {
	claims := s.claims(r)
	rows, err := s.Pool.Query(r.Context(), `
		SELECT id::text, url, branch, (token_enc IS NOT NULL) AS has_token,
		       status, error_message, last_synced_at::text, created_at::text
		FROM git_repos WHERE user_id = $1::uuid ORDER BY created_at DESC`, claims.Subject)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer rows.Close()

	repos := make([]repoResponse, 0)
	for rows.Next() {
		var rr repoResponse
		if err := rows.Scan(&rr.ID, &rr.URL, &rr.Branch, &rr.HasToken,
			&rr.Status, &rr.ErrorMessage, &rr.LastSyncedAt, &rr.CreatedAt); err != nil {
			continue
		}
		repos = append(repos, rr)
	}
	s.JSON(w, http.StatusOK, map[string]any{"repos": repos})
}

// POST /api/my/repos
func (s *State) AddRepo(w http.ResponseWriter, r *http.Request) {
	claims := s.claims(r)
	var req struct {
		URL    string `json:"url"`
		Branch string `json:"branch"`
		Token  string `json:"token"`
	}
	if err := decode(r, &req); err != nil {
		s.Error(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	req.URL = strings.TrimSpace(req.URL)
	if req.URL == "" {
		s.Error(w, http.StatusBadRequest, "url is required")
		return
	}
	if !strings.HasPrefix(req.URL, "http://") && !strings.HasPrefix(req.URL, "https://") {
		s.Error(w, http.StatusBadRequest, "only http/https URLs are supported")
		return
	}
	req.URL = normalizeRepoURL(req.URL)
	if req.Branch == "" {
		req.Branch = "main"
	}

	var tokenEnc *string
	if req.Token != "" {
		key := content.DeriveKey(s.Config.RepoTokenSecret)
		enc, err := content.EncryptToken(req.Token, key)
		if err != nil {
			s.Error(w, http.StatusInternalServerError, "Token encryption failed")
			return
		}
		tokenEnc = &enc
	}

	var rr repoResponse
	err := s.Pool.QueryRow(r.Context(), `
		INSERT INTO git_repos (user_id, url, branch, token_enc)
		VALUES ($1::uuid, $2, $3, $4)
		RETURNING id::text, url, branch, (token_enc IS NOT NULL), status, error_message,
		          last_synced_at::text, created_at::text`,
		claims.Subject, req.URL, req.Branch, tokenEnc).
		Scan(&rr.ID, &rr.URL, &rr.Branch, &rr.HasToken,
			&rr.Status, &rr.ErrorMessage, &rr.LastSyncedAt, &rr.CreatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "unique") {
			s.Error(w, http.StatusConflict, "This repository is already added")
			return
		}
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	s.JSON(w, http.StatusCreated, rr)
}

// DELETE /api/my/repos/{id}
func (s *State) DeleteRepo(w http.ResponseWriter, r *http.Request) {
	claims := s.claims(r)
	id := param(r, "id")

	var repoURL string
	err := s.Pool.QueryRow(r.Context(),
		`SELECT url FROM git_repos WHERE id = $1::uuid AND user_id = $2::uuid`,
		id, claims.Subject).Scan(&repoURL)
	if err != nil {
		s.Error(w, http.StatusNotFound, "Repository not found")
		return
	}

	if _, err := s.Pool.Exec(r.Context(),
		`DELETE FROM git_repos WHERE id = $1::uuid AND user_id = $2::uuid`,
		id, claims.Subject); err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}

	// Remove courses loaded from this repo
	s.Content.DeleteBySource(repoURL)

	// Clean up cloned directory
	repoDir := filepath.Join(s.Config.ReposDir, id)
	os.RemoveAll(repoDir) //nolint:errcheck

	s.JSON(w, http.StatusOK, map[string]string{"message": "Repository removed"})
}

// POST /api/my/repos/{id}/sync
func (s *State) SyncRepo(w http.ResponseWriter, r *http.Request) {
	claims := s.claims(r)
	id := param(r, "id")

	var repoURL, branch string
	var tokenEnc *string
	err := s.Pool.QueryRow(r.Context(), `
		SELECT url, branch, token_enc
		FROM git_repos WHERE id = $1::uuid AND user_id = $2::uuid`,
		id, claims.Subject).Scan(&repoURL, &branch, &tokenEnc)
	if err != nil {
		s.Error(w, http.StatusNotFound, "Repository not found")
		return
	}

	// Mark as syncing
	s.Pool.Exec(r.Context(), //nolint:errcheck
		`UPDATE git_repos SET status = 'syncing', error_message = NULL WHERE id = $1::uuid`, id)

	var token string
	if tokenEnc != nil {
		key := content.DeriveKey(s.Config.RepoTokenSecret)
		decrypted, err := content.DecryptToken(*tokenEnc, key)
		if err != nil {
			s.updateRepoStatus(id, "error", "token decryption failed: "+err.Error())
			s.Error(w, http.StatusInternalServerError, "Token decryption failed")
			return
		}
		token = decrypted
	}

	repoDir := filepath.Join(s.Config.ReposDir, id)
	syncErr := s.Content.SyncRepo(repoURL, branch, token, repoDir, repoURL)
	if syncErr != nil {
		s.updateRepoStatus(id, "error", syncErr.Error())
		s.Error(w, http.StatusBadGateway, fmt.Sprintf("Sync failed: %s", syncErr))
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	s.Pool.Exec(r.Context(), //nolint:errcheck
		`UPDATE git_repos SET status = 'synced', error_message = NULL, last_synced_at = NOW()
		 WHERE id = $1::uuid`, id)

	s.JSON(w, http.StatusOK, map[string]any{
		"message":        "Sync successful",
		"last_synced_at": now,
	})
}

func (s *State) updateRepoStatus(id, status, errMsg string) {
	s.Pool.Exec(context.Background(), //nolint:errcheck
		`UPDATE git_repos SET status = $2, error_message = $3 WHERE id = $1::uuid`,
		id, status, errMsg)
}

// normalizeRepoURL strips GitHub/GitLab sub-paths so pasted tree URLs become repo roots.
// https://github.com/user/repo/tree/branch/path → https://github.com/user/repo
// https://gitlab.com/user/repo/-/tree/branch    → https://gitlab.com/user/repo
func normalizeRepoURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	// Remove trailing .git if present
	u.Path = strings.TrimSuffix(u.Path, ".git")
	// Split path into segments, keep only the first two non-empty ones (user + repo)
	parts := strings.FieldsFunc(u.Path, func(r rune) bool { return r == '/' })
	if len(parts) <= 2 {
		u.Path = "/" + strings.Join(parts, "/")
		u.RawQuery = ""
		u.Fragment = ""
		return u.String()
	}
	u.Path = "/" + parts[0] + "/" + parts[1]
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}
