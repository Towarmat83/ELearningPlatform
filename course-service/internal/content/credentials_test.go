package content

import (
	"os"
	"testing"
)

func TestMatchGitURL(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		repoURL string
		want    bool
	}{
		{
			name:    "exact match with https prefix",
			pattern: "https://github.com/myorg/myrepo",
			repoURL: "https://github.com/myorg/myrepo",
			want:    true,
		},
		{
			name:    "wildcard org match",
			pattern: "github.com/myorg/*",
			repoURL: "https://github.com/myorg/repo1",
			want:    true,
		},
		{
			name:    "wildcard does not match nested",
			pattern: "github.com/myorg/*",
			repoURL: "https://github.com/myorg/repo1/sub",
			want:    false,
		},
		{
			name:    "different host no match",
			pattern: "github.com/myorg/repo",
			repoURL: "https://gitlab.com/myorg/repo",
			want:    false,
		},
		{
			name:    "http prefix stripped",
			pattern: "github.com/org/repo",
			repoURL: "http://github.com/org/repo",
			want:    true,
		},
		{
			name:    "git@ prefix stripped",
			pattern: "github.com:org/repo",
			repoURL: "git@github.com:org/repo",
			want:    true,
		},
		{
			name:    "pattern without prefix, URL with https",
			pattern: "github.com/myorg/exact-repo",
			repoURL: "https://github.com/myorg/exact-repo",
			want:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := matchGitURL(tc.pattern, tc.repoURL)
			if got != tc.want {
				t.Errorf("matchGitURL(%q, %q) = %v, want %v", tc.pattern, tc.repoURL, got, tc.want)
			}
		})
	}
}

func TestGitCredentialStore_Match_Nil(t *testing.T) {
	var s *GitCredentialStore

	token := s.Match("https://github.com/any/repo")
	if token != "" {
		t.Errorf("expected empty token from nil store, got %q", token)
	}
}

func TestGitCredentialStore_Match_Found(t *testing.T) {
	s := &GitCredentialStore{
		entries: []Credential{
			{URL: "github.com/org1/*", Token: "tok1"},
			{URL: "github.com/org2/*", Token: "tok2"},
		},
	}

	tok := s.Match("https://github.com/org1/myrepo")
	if tok != "tok1" {
		t.Errorf("expected tok1, got %q", tok)
	}

	tok2 := s.Match("https://github.com/org2/another")
	if tok2 != "tok2" {
		t.Errorf("expected tok2, got %q", tok2)
	}
}

func TestGitCredentialStore_Match_NotFound(t *testing.T) {
	s := &GitCredentialStore{
		entries: []Credential{
			{URL: "github.com/org1/*", Token: "tok1"},
		},
	}

	tok := s.Match("https://gitlab.com/org1/repo")
	if tok != "" {
		t.Errorf("expected empty token, got %q", tok)
	}
}

func TestGitCredentialStore_Match_Empty(t *testing.T) {
	s := &GitCredentialStore{}

	tok := s.Match("https://github.com/any/repo")
	if tok != "" {
		t.Errorf("expected empty token from empty store, got %q", tok)
	}
}

func TestLoadCredentials_FileNotFound(t *testing.T) {
	_, err := LoadCredentials("/nonexistent/path/creds.yaml")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoadCredentials_ValidFile(t *testing.T) {
	content := `
credentials:
  - url: "github.com/myorg/*"
    token: "mytoken123"
  - url: "gitlab.com/other/*"
    token: "token456"
`

	f, err := os.CreateTemp(t.TempDir(), "creds-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	f.WriteString(content)
	f.Close()

	store, err := LoadCredentials(f.Name())
	if err != nil {
		t.Fatalf("LoadCredentials failed: %v", err)
	}

	if store == nil {
		t.Fatal("expected non-nil store")
	}

	tok := store.Match("https://github.com/myorg/repo1")
	if tok != "mytoken123" {
		t.Errorf("expected mytoken123, got %q", tok)
	}

	tok2 := store.Match("https://gitlab.com/other/repo")
	if tok2 != "token456" {
		t.Errorf("expected token456, got %q", tok2)
	}
}

func TestLoadCredentials_EmptyFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "empty-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	f.Close()

	store, err := LoadCredentials(f.Name())
	if err != nil {
		t.Fatalf("LoadCredentials on empty file: %v", err)
	}

	if store == nil {
		t.Fatal("expected non-nil store even for empty file")
	}
}

func TestLoadCredentials_InvalidYAML(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "bad-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	// Tab characters in YAML where spaces are expected trigger a hard parse error
	f.WriteString("credentials:\n\t- url: bad\n")
	f.Close()

	_, err = LoadCredentials(f.Name())
	if err == nil {
		t.Error("expected error for invalid YAML (tab indentation)")
	}
}
