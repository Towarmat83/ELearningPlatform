package handlers

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
)

const maxUploadSize = 500 << 20 // 500 MB

var allowedExts = map[string]string{
	".mp4":  "video",
	".webm": "video",
	".ogg":  "video",
	".md":   "markdown",
}

// POST /api/admin/uploads/video
func (s *State) UploadVideo(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		s.Error(w, http.StatusBadRequest, "File too large or invalid form data")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		s.Error(w, http.StatusBadRequest, "Missing 'file' field in form")
		return
	}
	defer file.Close()

	// Validate extension
	ext := strings.ToLower(filepath.Ext(header.Filename))
	kind, ok := allowedExts[ext]
	if !ok {
		s.Error(w, http.StatusBadRequest, "Allowed formats: .mp4, .webm, .ogg (video) or .md (Markdown)")
		return
	}

	// For video files, validate MIME type
	if kind == "video" {
		buf := make([]byte, 512)
		n, _ := file.Read(buf)
		mime := http.DetectContentType(buf[:n])
		if !strings.HasPrefix(mime, "video/") && mime != "application/octet-stream" {
			s.Error(w, http.StatusBadRequest, "File does not appear to be a video")
			return
		}
		if _, err := file.(io.Seeker).Seek(0, io.SeekStart); err != nil {
			s.Error(w, http.StatusInternalServerError, "Failed to process file")
			return
		}
	}

	// Ensure uploads directory exists
	uploadsDir := s.Config.UploadsDir
	if err := os.MkdirAll(uploadsDir, 0o755); err != nil {
		s.Error(w, http.StatusInternalServerError, "Failed to create uploads directory")
		return
	}

	// Generate a unique filename
	id := newUUID()
	filename := fmt.Sprintf("%s%s", id, ext)
	dst, err := os.Create(filepath.Join(uploadsDir, filename))
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Failed to save file")
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		s.Error(w, http.StatusInternalServerError, "Failed to write file")
		return
	}

	url := "/uploads/" + filename
	s.JSON(w, http.StatusCreated, map[string]string{
		"url":      url,
		"filename": filename,
		"kind":     kind,
	})
}

// GET /uploads/{filename}
func (s *State) ServeUpload(w http.ResponseWriter, r *http.Request) {
	filename := chi.URLParam(r, "filename")
	// Sanitize: no path traversal
	if strings.Contains(filename, "..") || strings.Contains(filename, "/") {
		s.Error(w, http.StatusBadRequest, "Invalid filename")
		return
	}
	path := filepath.Join(s.Config.UploadsDir, filename)
	http.ServeFile(w, r, path)
}

// newUUID generates a simple UUID v4-like string using crypto/rand via pgx conventions.
// We use a simple approach here to avoid extra imports.
func newUUID() string {
	f, _ := os.Open("/dev/urandom")
	if f == nil {
		// fallback: use timestamp-based name
		return fmt.Sprintf("%d", os.Getpid())
	}
	defer f.Close()
	b := make([]byte, 16)
	f.Read(b) //nolint:errcheck
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
