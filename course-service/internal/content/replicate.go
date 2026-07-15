package content

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
)

// replicatedPrefix is the URL path prefix under which replicated module
// resources are served once downloaded.
const replicatedPrefix = "/uploads/"

// replicateDirPerm is the permission used when creating the uploads
// directory.
const replicateDirPerm = 0o750

// maxReplicateExtLength is the longest file extension (including the
// leading dot) considered valid when deriving a cached filename.
const maxReplicateExtLength = 5

// ReplicatedPath returns the local /uploads/ URL path for a replicated
// module resource.
// For text modules or when replication is false, it returns the original
// Src unchanged. The downloaded file is cached on disk; subsequent calls
// skip the download.
func ReplicatedPath(ctx context.Context, mod Module, uploadsDir string) string {
	if !mod.Replication || mod.Type == moduleTypeText {
		return mod.Src
	}

	if mod.Src == "" {
		return ""
	}

	ext := extension(mod.Src)
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(mod.Src)))[:16]
	filename := hash + ext
	localPath := filepath.Join(uploadsDir, filename)

	_, err := os.Stat(localPath)
	if err == nil {
		return replicatedPrefix + filename
	}

	err = downloadFile(ctx, mod.Src, localPath)
	if err != nil {
		zap.L().Error("replication: download failed — returning local path anyway",
			zap.String("src", mod.Src), zap.String("dest", localPath), zap.Error(err))

		return replicatedPrefix + filename
	}

	zap.L().Info("replication: resource cached", zap.String("src", mod.Src), zap.String("local", filename))

	return replicatedPrefix + filename
}

// downloadFile downloads the resource at rawURL and writes it to dest,
// creating any missing parent directories.
func downloadFile(ctx context.Context, rawURL, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("http get: %w", err)
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	err = os.MkdirAll(filepath.Dir(dest), replicateDirPerm)
	if err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	file, err := os.Create(filepath.Clean(dest))
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}

	//nolint:errcheck // closing the file post-write cannot recover from
	// an error here; write success is already validated below.
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		_ = os.Remove(dest)

		return fmt.Errorf("write file: %w", err)
	}

	return nil
}

// extension returns the lowercased file extension (including the leading
// dot) from rawURL, or "" if none is found or it looks implausibly long.
func extension(rawURL string) string {
	idx := strings.LastIndex(rawURL, ".")
	if idx < 0 {
		return ""
	}

	ext := strings.ToLower(rawURL[idx:])

	idx2 := strings.IndexAny(ext, "?#")
	if idx2 >= 0 {
		ext = ext[:idx2]
	}

	if len(ext) <= maxReplicateExtLength {
		return ext
	}

	return ""
}
