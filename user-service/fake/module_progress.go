package fake

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/genesary/pupitre/user-service/internal/models"
)

// ModuleProgressRepository is an in-memory repository.ModuleProgressRepository
// for tests.
type ModuleProgressRepository struct {
	mu       sync.Mutex
	progress []models.ModuleProgress
	// Err, when set, is returned by every method call (simulates a DB failure).
	Err error
}

// NewModuleProgressRepository builds a fake ModuleProgressRepository seeded
// with progress rows.
func NewModuleProgressRepository(seed ...models.ModuleProgress) *ModuleProgressRepository {
	return &ModuleProgressRepository{progress: append([]models.ModuleProgress{}, seed...)}
}

func (f *ModuleProgressRepository) findIndex(userID, courseSlug string, moduleIndex int) int {
	for i := range f.progress {
		p := f.progress[i]
		if p.UserID == userID && p.CourseSlug == courseSlug && p.ModuleIndex == moduleIndex {
			return i
		}
	}

	return -1
}

func (f *ModuleProgressRepository) RecordProgress(
	_ context.Context, userID, courseSlug string, moduleIndex int, moduleSlug string, score, maxScore int, passed bool,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return f.Err
	}

	now := time.Now()

	i := f.findIndex(userID, courseSlug, moduleIndex)
	if i < 0 {
		var completedAt *time.Time
		if passed {
			completedAt = &now
		}

		var slugPtr *string
		if moduleSlug != "" {
			slugPtr = &moduleSlug
		}

		f.progress = append(f.progress, models.ModuleProgress{
			ID: uuid.NewString(), UserID: userID, CourseSlug: courseSlug, ModuleIndex: moduleIndex,
			ModuleSlug: slugPtr, BestScore: score, MaxScore: maxScore, Passed: passed, Attempts: 1,
			CompletedAt: completedAt, UpdatedAt: now,
		})

		return nil
	}

	p := &f.progress[i]
	p.Attempts++

	if score > p.BestScore {
		p.BestScore = score
	}

	p.MaxScore = maxScore
	p.Passed = p.Passed || passed

	if p.ModuleSlug == nil && moduleSlug != "" {
		p.ModuleSlug = &moduleSlug
	}

	if passed && p.CompletedAt == nil {
		p.CompletedAt = &now
	}

	p.UpdatedAt = now

	return nil
}

func (f *ModuleProgressRepository) TotalScore(_ context.Context, userID, courseSlug string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return 0, f.Err
	}

	var total int64

	for _, p := range f.progress {
		if p.UserID == userID && p.CourseSlug == courseSlug {
			total += int64(p.BestScore)
		}
	}

	return total, nil
}

func (f *ModuleProgressRepository) PassedModuleSlugs(_ context.Context, userID, courseSlug string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return nil, f.Err
	}

	slugs := make([]string, 0)

	for _, p := range f.progress {
		if p.UserID == userID && p.CourseSlug == courseSlug && p.Passed && p.ModuleSlug != nil {
			slugs = append(slugs, *p.ModuleSlug)
		}
	}

	return slugs, nil
}

func (f *ModuleProgressRepository) ListByUserCourse(_ context.Context, userID, courseSlug string) ([]models.ModuleProgress, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return nil, f.Err
	}

	list := make([]models.ModuleProgress, 0)

	for _, p := range f.progress {
		if p.UserID == userID && p.CourseSlug == courseSlug {
			list = append(list, p)
		}
	}

	return list, nil
}

func (f *ModuleProgressRepository) CompletedCourseSlugs(_ context.Context, userID string, slugs []string) ([]string, error) {
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
		if p.UserID == userID && p.Passed && wanted[p.CourseSlug] && !seen[p.CourseSlug] {
			seen[p.CourseSlug] = true

			completed = append(completed, p.CourseSlug)
		}
	}

	return completed, nil
}
