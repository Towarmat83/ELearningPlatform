package fake

import (
	"context"
	"slices"
	"sort"
	"sync"

	"github.com/genesary/pupitre/course-service/internal/content"
	"github.com/genesary/pupitre/course-service/internal/repository"
)

// PathRepository is an in-memory fake of repository.PathRepository.
type PathRepository struct {
	mu sync.RWMutex
	// paths is keyed by slug; courseSkills lets SkillsOfCourses answer
	// without a CourseRepository, and is seeded by WithCourseSkills.
	paths        map[string]*content.Path
	courseSkills map[string][]string
}

// NewPathRepository builds a fake seeded with the given paths.
func NewPathRepository(seed ...*content.Path) *PathRepository {
	repo := &PathRepository{
		paths:        make(map[string]*content.Path, len(seed)),
		courseSkills: make(map[string][]string),
	}

	for _, path := range seed {
		copied := *path
		repo.paths[copied.Slug] = &copied
	}

	return repo
}

// WithCourseSkills declares the skills a course teaches, so that
// SkillsOfCourses has something to aggregate.
func (f *PathRepository) WithCourseSkills(courseSlug string, skills ...string) *PathRepository {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.courseSkills[courseSlug] = skills

	return f
}

// List returns paths ordered by title, paginated by limit/offset.
func (f *PathRepository) List(_ context.Context, limit, offset int) ([]*content.Path, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	out := make([]*content.Path, 0, len(f.paths))
	for _, path := range f.paths {
		copied := *path
		out = append(out, &copied)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })

	if offset > 0 {
		if offset > len(out) {
			offset = len(out)
		}

		out = out[offset:]
	}

	if limit > 0 && limit < len(out) {
		out = out[:limit]
	}

	return out, nil
}

// Get returns one path, or repository.ErrNotFound.
func (f *PathRepository) Get(_ context.Context, slug string) (*content.Path, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	path, ok := f.paths[slug]
	if !ok {
		return nil, repository.ErrNotFound
	}

	copied := *path

	return &copied, nil
}

// SlugsContainingCourse returns the slugs of every path including courseSlug.
func (f *PathRepository) SlugsContainingCourse(_ context.Context, courseSlug string) ([]string, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	var slugs []string

	for slug, path := range f.paths {
		if slices.Contains(path.Courses, courseSlug) {
			slugs = append(slugs, slug)
		}
	}

	sort.Strings(slugs)

	return slugs, nil
}

// SkillsOfCourses returns the deduplicated union of the declared skills of
// the given courses.
func (f *PathRepository) SkillsOfCourses(_ context.Context, courseSlugs []string) ([]string, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	seen := make(map[string]struct{})

	var skills []string

	for _, courseSlug := range courseSlugs {
		for _, skill := range f.courseSkills[courseSlug] {
			if _, dup := seen[skill]; dup {
				continue
			}

			seen[skill] = struct{}{}

			skills = append(skills, skill)
		}
	}

	sort.Strings(skills)

	return skills, nil
}

// Upsert replaces a path definition wholesale.
func (f *PathRepository) Upsert(_ context.Context, path *content.Path) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	copied := *path
	f.paths[copied.Slug] = &copied

	return nil
}

// Create inserts a new path, reporting repository.ErrConflict when the
// slug is taken.
func (f *PathRepository) Create(_ context.Context, path *content.Path) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if _, exists := f.paths[path.Slug]; exists {
		return repository.ErrConflict
	}

	copied := *path
	f.paths[copied.Slug] = &copied

	return nil
}

// Delete removes a path.
func (f *PathRepository) Delete(_ context.Context, slug string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if _, exists := f.paths[slug]; !exists {
		return repository.ErrNotFound
	}

	delete(f.paths, slug)

	return nil
}
