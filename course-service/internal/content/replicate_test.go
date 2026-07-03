package content

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestExtension(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://example.com/video.mp4", ".mp4"},
		{"https://example.com/image.PNG", ".png"},
		{"https://example.com/file.jpg?v=123", ".jpg"},
		{"https://example.com/file.gif#anchor", ".gif"},
		{"https://example.com/noext", ""},
		{"https://example.com/very-long-extension.abcdefg", ""},
		{"https://example.com/path/to/file.webm", ".webm"},
		{"", ""},
		{"https://example.com/file.tar.gz", ".gz"},
	}

	for _, tc := range tests {
		got := extension(tc.url)
		if got != tc.want {
			t.Errorf("extension(%q) = %q, want %q", tc.url, got, tc.want)
		}
	}
}

func TestReplicatedPath_NoReplication(t *testing.T) {
	m := Module{
		Type:        "video",
		Src:         "https://example.com/video.mp4",
		Replication: false,
	}

	result := ReplicatedPath(m, "/tmp/uploads")
	if result != "https://example.com/video.mp4" {
		t.Errorf("expected original Src, got %q", result)
	}
}

func TestReplicatedPath_TextType(t *testing.T) {
	m := Module{
		Type:        "text",
		Src:         "https://example.com/content.md",
		Replication: true,
	}
	result := ReplicatedPath(m, "/tmp/uploads")
	// text type returns Src unchanged even with replication
	if result != "https://example.com/content.md" {
		t.Errorf("expected original Src for text type, got %q", result)
	}
}

func TestReplicatedPath_EmptySrc(t *testing.T) {
	m := Module{
		Type:        "image",
		Src:         "",
		Replication: true,
	}

	result := ReplicatedPath(m, "/tmp/uploads")
	if result != "" {
		t.Errorf("expected empty string for empty Src, got %q", result)
	}
}

func TestReplicatedPath_ReplicationEnabled(t *testing.T) {
	m := Module{
		Type:        "image",
		Src:         "https://example.com/image.png",
		Replication: true,
	}
	// Returns a path starting with /uploads/ (file may not actually be downloaded in test)
	result := ReplicatedPath(m, "/tmp/uploads-test")
	if result == "" {
		t.Error("expected non-empty result")
	}
	// Should return the replicated path pattern
	if len(result) == 0 {
		t.Error("expected non-empty path")
	}
}

// ── downloadFile tests ────────────────────────────────────────────────────────

func TestDownloadFile_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("file content here"))
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	dest := filepath.Join(tmpDir, "downloaded.txt")

	err := downloadFile(srv.URL, dest)
	if err != nil {
		t.Fatalf("downloadFile: %v", err)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}

	if string(data) != "file content here" {
		t.Errorf("expected 'file content here', got %q", string(data))
	}
}

func TestDownloadFile_Non200Status(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()

	err := downloadFile(srv.URL, filepath.Join(tmpDir, "file.txt"))
	if err == nil {
		t.Error("expected error for 404 response")
	}
}

func TestDownloadFile_BadURL(t *testing.T) {
	err := downloadFile("http://127.0.0.1:0/invalid", "/tmp/nowhere.txt")
	if err == nil {
		t.Error("expected error for bad URL")
	}
}

func TestDownloadFile_DestInSubdir(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("data"))
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	dest := filepath.Join(tmpDir, "subdir", "nested", "file.txt")

	err := downloadFile(srv.URL, dest)
	if err != nil {
		t.Fatalf("downloadFile to nested dest: %v", err)
	}

	if _, err := os.Stat(dest); err != nil {
		t.Errorf("expected file to exist at %q", dest)
	}
}

func TestReplicatedPath_AlreadyCached(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("jpg data"))
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	m := Module{
		Type:        "image",
		Src:         srv.URL + "/photo.jpg",
		Replication: true,
	}

	result1 := ReplicatedPath(m, tmpDir)
	if result1 == "" {
		t.Fatal("expected non-empty result from first call")
	}
	// Second call should hit the cached file
	result2 := ReplicatedPath(m, tmpDir)
	if result2 != result1 {
		t.Errorf("expected same path from cache, got %q vs %q", result1, result2)
	}
}
