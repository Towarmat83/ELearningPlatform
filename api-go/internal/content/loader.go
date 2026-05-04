package content

import (
	"bufio"
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Store holds all courses in memory and is safe for concurrent reads/writes.
type Store struct {
	mu      sync.RWMutex
	courses map[string]*Course // slug → *Course
}

func NewStore() *Store {
	return &Store{courses: make(map[string]*Course)}
}

// Get returns a course by slug (nil if not found).
func (s *Store) Get(slug string) *Course {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.courses[slug]
}

// List returns all published courses sorted by title.
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

// All returns all courses (including unpublished) for admin use.
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

// Put adds or replaces a course.
func (s *Store) Put(c *Course) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.courses[c.Slug] = c
}

// DeleteBySource removes all courses loaded from a given source (git repo URL).
func (s *Store) DeleteBySource(source string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for slug, c := range s.courses {
		if c.Source == source {
			delete(s.courses, slug)
		}
	}
}

// LoadDir scans dir for course sub-directories and loads them into the store.
// source is "local" for the default courses dir, or the git repo URL.
func (s *Store) LoadDir(dir, source string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	loaded := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		courseDir := filepath.Join(dir, e.Name())
		c, err := loadCourse(courseDir, source)
		if err != nil {
			slog.Warn("skipping course directory", "dir", courseDir, "err", err)
			continue
		}
		s.Put(c)
		loaded++
	}
	slog.Info("courses loaded", "dir", dir, "count", loaded)
	return nil
}

// loadCourse parses a single course directory.
func loadCourse(dir, source string) (*Course, error) {
	slug := filepath.Base(dir)
	meta, err := parseCourseYAML(filepath.Join(dir, "course.yaml"))
	if err != nil {
		return nil, err
	}
	if meta["title"] == "" {
		meta["title"] = slug
	}

	lessons, err := loadLessons(dir)
	if err != nil {
		return nil, err
	}

	return &Course{
		Slug:        slug,
		Title:       meta["title"],
		Description: meta["description"],
		Category:    meta["category"],
		Difficulty:  meta["difficulty"],
		IsPublished: meta["is_published"] == "true",
		Lessons:     lessons,
		Source:      source,
	}, nil
}

// lessonFile pairs a lesson with its numeric sort key.
type lessonFile struct {
	order int
	path  string
}

var orderPrefix = regexp.MustCompile(`^(\d+)-`)

// loadLessons reads all NN-*.md files in dir, sorted by numeric prefix.
func loadLessons(dir string) ([]Lesson, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var files []lessonFile
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".md") {
			continue
		}
		order := 0
		if m := orderPrefix.FindStringSubmatch(name); m != nil {
			for _, r := range m[1] {
				order = order*10 + int(r-'0')
			}
		}
		files = append(files, lessonFile{order: order, path: filepath.Join(dir, name)})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].order < files[j].order })

	lessons := make([]Lesson, 0, len(files))
	for idx, f := range files {
		l, err := loadLesson(f.path, idx+1)
		if err != nil {
			slog.Warn("skipping lesson", "path", f.path, "err", err)
			continue
		}
		lessons = append(lessons, l)
	}
	return lessons, nil
}

// loadLesson parses a single lesson Markdown file.
func loadLesson(path string, order int) (Lesson, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Lesson{}, err
	}

	title, body := extractFrontmatter(data)
	base := filepath.Base(path)
	slug := strings.TrimSuffix(base, ".md")
	// strip numeric prefix for a cleaner slug: "01-intro" → "intro"
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

// extractFrontmatter splits YAML front matter from Markdown body.
// Returns (title, body). Only "title" key is read from frontmatter.
func extractFrontmatter(data []byte) (title, body string) {
	data = bytes.TrimSpace(data)
	if !bytes.HasPrefix(data, []byte("---")) {
		return "", string(data)
	}
	// find closing ---
	rest := data[3:]
	idx := bytes.Index(rest, []byte("---"))
	if idx < 0 {
		return "", string(data)
	}
	frontmatter := rest[:idx]
	body = strings.TrimSpace(string(rest[idx+3:]))

	fm := parseSimpleYAML(frontmatter)
	return fm["title"], body
}

// parseCourseYAML reads course.yaml and returns key→value map.
func parseCourseYAML(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseSimpleYAML(data), nil
}

// parseSimpleYAML parses flat key: value YAML (no nesting, no arrays).
func parseSimpleYAML(data []byte) map[string]string {
	m := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, ':')
		if idx < 1 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		// strip surrounding quotes
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') ||
				(val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		m[key] = val
	}
	return m
}
