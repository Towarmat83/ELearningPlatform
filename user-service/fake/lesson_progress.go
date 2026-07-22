package fake

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/genesary/pupitre/user-service/internal/models"
)

// lessonSlugComplete is the sentinel lesson slug value that marks a course as
// fully completed.
const lessonSlugComplete = "__complete__"

// LessonProgressRepository is an in-memory repository.LessonProgressRepository
// for tests.
type LessonProgressRepository struct {
	mu       sync.Mutex
	progress []models.LessonProgress
	// Err, when set, is returned by every method call (simulates a DB failure).
	Err error
}

// NewLessonProgressRepository builds a fake LessonProgressRepository seeded
// with progress rows.
func NewLessonProgressRepository(seed ...models.LessonProgress) *LessonProgressRepository {
	return &LessonProgressRepository{progress: append([]models.LessonProgress{}, seed...)}
}

// MarkComplete records that userID viewed lessonSlug in courseSlug, or does
// nothing if already recorded.
func (f *LessonProgressRepository) MarkComplete(_ context.Context, userID, courseSlug, lessonSlug string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return f.Err
	}

	for _, p := range f.progress {
		if p.UserID == userID && p.CourseSlug == courseSlug && p.LessonSlug == lessonSlug {
			return nil
		}
	}

	f.progress = append(f.progress, models.LessonProgress{
		ID: uuid.NewString(), UserID: userID, CourseSlug: courseSlug, LessonSlug: lessonSlug, ViewedAt: time.Now(),
	})

	return nil
}

// ViewedSlugs lists the lesson slugs userID has viewed in courseSlug.
func (f *LessonProgressRepository) ViewedSlugs(_ context.Context, userID, courseSlug string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return nil, f.Err
	}

	slugs := make([]string, 0)

	for _, p := range f.progress {
		if p.UserID == userID && p.CourseSlug == courseSlug {
			slugs = append(slugs, p.LessonSlug)
		}
	}

	return slugs, nil
}

// CountViewed returns the number of lessons userID has viewed in courseSlug.
func (f *LessonProgressRepository) CountViewed(_ context.Context, userID, courseSlug string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return 0, f.Err
	}

	var count int64

	for _, p := range f.progress {
		if p.UserID == userID && p.CourseSlug == courseSlug {
			count++
		}
	}

	return count, nil
}

// ViewedKeys returns the set of "courseSlug/lessonSlug" composite keys for
// all non-sentinel lessons the user has viewed.
func (f *LessonProgressRepository) ViewedKeys(_ context.Context, userID string) (map[string]struct{}, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return nil, f.Err
	}

	out := make(map[string]struct{})

	for _, p := range f.progress {
		if p.UserID == userID && p.LessonSlug != "" && p.LessonSlug != lessonSlugComplete {
			out[p.CourseSlug+"/"+p.LessonSlug] = struct{}{}
		}
	}

	return out, nil
}

// CompletedCourseSlugs filters slugs down to the courses userID has
// completed.
func (f *LessonProgressRepository) CompletedCourseSlugs(_ context.Context, userID string, slugs []string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return nil, f.Err
	}

	wanted := make(map[string]bool, len(slugs))
	for _, s := range slugs {
		wanted[s] = true
	}

	seen := make(map[string]bool)
	completed := make([]string, 0)

	for _, p := range f.progress {
		if p.UserID == userID && p.LessonSlug == lessonSlugComplete && wanted[p.CourseSlug] && !seen[p.CourseSlug] {
			seen[p.CourseSlug] = true

			completed = append(completed, p.CourseSlug)
		}
	}

	return completed, nil
}
