package content

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Store holds all courses in memory and is safe for concurrent reads/writes.
type Store struct {
	mu      sync.RWMutex
	courses map[string]*Course
}

// NewStore returns an empty, ready-to-use in-memory course store.
func NewStore() *Store {
	return &Store{courses: make(map[string]*Course)}
}

// Get returns the course with the given slug, or nil if not found.
func (s *Store) Get(slug string) *Course {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.courses[slug]
}

// List returns all public courses sorted alphabetically by title.
func (s *Store) List() []*Course {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*Course, 0, len(s.courses))
	for _, c := range s.courses {
		if c.IsPublic {
			out = append(out, c)
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })

	return out
}

// All returns every course (public and private) sorted alphabetically by title.
func (s *Store) All() []*Course {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*Course, 0, len(s.courses))
	for _, c := range s.courses {
		out = append(out, c)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })

	return out
}

// Put adds or replaces a course in the store.
func (s *Store) Put(c *Course) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.courses[c.Slug] = c
}

// DeleteBySource removes all courses whose Source field equals source.
func (s *Store) DeleteBySource(source string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for slug, c := range s.courses {
		if c.Source == source {
			delete(s.courses, slug)
		}
	}
}

// PathStore holds all learning paths in memory, safe for concurrent access.
type PathStore struct {
	mu    sync.RWMutex
	paths map[string]*Path
}

// NewPathStore returns an empty, ready-to-use in-memory path store.
func NewPathStore() *PathStore {
	return &PathStore{paths: make(map[string]*Path)}
}

// Get returns the path with the given slug, or nil if not found.
func (s *PathStore) Get(slug string) *Path {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.paths[slug]
}

// List returns all paths. Order is undefined; sorting is the caller's
// responsibility.
func (s *PathStore) List() []*Path {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*Path, 0, len(s.paths))
	for _, p := range s.paths {
		out = append(out, p)
	}

	return out
}

// Put adds or replaces a path in the store.
func (s *PathStore) Put(p *Path) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.paths[p.Slug] = p
}

// DeleteBySource removes all paths whose Source field equals source.
func (s *PathStore) DeleteBySource(source string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var toDelete []string

	for slug, p := range s.paths {
		if p.Source == source {
			toDelete = append(toDelete, slug)
		}
	}

	for _, slug := range toDelete {
		delete(s.paths, slug)
	}
}

// orderPrefix matches the leading "NN-" order prefix on lesson filenames.
var orderPrefix = regexp.MustCompile(`^(\d+)-`)

// ParseMarkdownLesson parses a markdown file with optional YAML front matter.
func ParseMarkdownLesson(path string, order int) (Lesson, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return Lesson{}, fmt.Errorf("reading lesson file %s: %w", path, err)
	}

	title, body := extractFrontmatter(data)
	base := filepath.Base(path)

	slug := strings.TrimSuffix(base, ".md")
	if m := orderPrefix.FindStringSubmatch(slug); m != nil {
		slug = slug[len(m[0]):]
	}

	if title == "" {
		title = slug
	}

	return Lesson{
		Slug:    slug,
		Title:   title,
		Order:   order,
		Content: body,
	}, nil
}

// FetchModuleIndex fetches and parses a module index YAML file from git.
// Index entries that omit src or ref inherit them from the parent module.
func FetchModuleIndex(ctx context.Context, gc *GitCache, parent Module, token string) ([]Module, error) {
	data, err := gc.FetchModuleContent(ctx, parent.Src, parent.Ref, parent.Path, token)
	if err != nil {
		return nil, err
	}

	var entries []ModuleIndexEntry

	err = yaml.Unmarshal(data, &entries)
	if err != nil {
		return nil, fmt.Errorf("parse module index: %w", err)
	}

	modules := make([]Module, 0, len(entries))
	for _, entry := range entries {
		src, ref, typ := entry.Src, entry.Ref, entry.Type
		if src == "" {
			src = parent.Src
		}

		if ref == "" {
			ref = parent.Ref
		}

		if typ == "" {
			typ = moduleTypeText
		}

		modules = append(modules, Module{
			Name:          entry.Name,
			Type:          typ,
			Src:           src,
			Ref:           ref,
			Path:          entry.Path,
			Hidden:        entry.Hidden,
			Prerequisites: entry.Prerequisites,
		})
	}

	return modules, nil
}

// extractFrontmatter splits data into an optional YAML front matter title
// and the remaining markdown body.
//
//nolint:nonamedreturns // gocritic(unnamedResult) wants names here.
func extractFrontmatter(data []byte) (title, body string) {
	data = bytes.TrimSpace(data)
	if !bytes.HasPrefix(data, []byte("---")) {
		return "", string(data)
	}

	rest := data[3:]

	before, after, ok := bytes.Cut(rest, []byte("---"))
	if !ok {
		return "", string(data)
	}

	frontmatter := before
	body = strings.TrimSpace(string(after))

	var fm lessonFrontmatter

	err := yaml.Unmarshal(frontmatter, &fm)
	if err == nil {
		return fm.Title, body
	}

	return "", body
}
