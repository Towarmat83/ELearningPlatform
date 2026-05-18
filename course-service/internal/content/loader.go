package content

import (
	"bytes"
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

func NewStore() *Store {
	return &Store{courses: make(map[string]*Course)}
}

func (s *Store) Get(slug string) *Course {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.courses[slug]
}

func (s *Store) List() []*Course {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Course, 0, len(s.courses))
	for _, c := range s.courses {
		if c.IsPublished {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	return out
}

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

func (s *Store) Put(c *Course) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.courses[c.Slug] = c
}

func (s *Store) DeleteBySource(source string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for slug, c := range s.courses {
		if c.Source == source {
			delete(s.courses, slug)
		}
	}
}

var orderPrefix = regexp.MustCompile(`^(\d+)-`)

// ParseMarkdownLesson parses a markdown file with optional YAML front matter.
func ParseMarkdownLesson(path string, order int) (Lesson, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Lesson{}, err
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

func extractFrontmatter(data []byte) (title, body string) {
	data = bytes.TrimSpace(data)
	if !bytes.HasPrefix(data, []byte("---")) {
		return "", string(data)
	}
	rest := data[3:]
	idx := bytes.Index(rest, []byte("---"))
	if idx < 0 {
		return "", string(data)
	}
	frontmatter := rest[:idx]
	body = strings.TrimSpace(string(rest[idx+3:]))

	var fm lessonFrontmatter
	if err := yaml.Unmarshal(frontmatter, &fm); err == nil {
		return fm.Title, body
	}
	return "", body
}
