package content

import (
	"crypto/sha256"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const replicatedPrefix = "/uploads/"

// ReplicatedPath returns the local /uploads/ URL path for a replicated module resource.
// For text modules or when replication is false, it returns the original Src unchanged.
// The downloaded file is cached on disk; subsequent calls skip the download.
func ReplicatedPath(m Module, uploadsDir string) string {
	if !m.Replication || m.Type == "text" {
		return m.Src
	}
	if m.Src == "" {
		return ""
	}

	ext := extension(m.Src)
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(m.Src)))[:16]
	filename := hash + ext
	localPath := filepath.Join(uploadsDir, filename)

	if _, err := os.Stat(localPath); err == nil {
		return replicatedPrefix + filename
	}

	if err := downloadFile(m.Src, localPath); err != nil {
		slog.Error("replication: download failed — returning local path anyway",
			"src", m.Src, "dest", localPath, "err", err)
		return replicatedPrefix + filename
	}

	slog.Info("replication: resource cached", "src", m.Src, "local", filename)
	return replicatedPrefix + filename
}

func downloadFile(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	f, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close() //nolint:errcheck

	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = os.Remove(dest)
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}

func extension(url string) string {
	if idx := strings.LastIndex(url, "."); idx >= 0 {
		ext := strings.ToLower(url[idx:])
		if idx2 := strings.IndexAny(ext, "?#"); idx2 >= 0 {
			ext = ext[:idx2]
		}
		if len(ext) <= 5 {
			return ext
		}
	}
	return ""
}
