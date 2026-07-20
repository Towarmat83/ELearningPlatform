// Package fake provides in-memory fakes of course-service's repository
// interfaces, so handler tests can control persisted state without a real
// database connection.
package fake

import (
	"context"
	"slices"
	"sync"

	"github.com/genesary/pupitre/course-service/internal/models"
)

// LabCheckRepository is an in-memory fake of repository.LabCheckRepository.
type LabCheckRepository struct {
	mu     sync.Mutex
	checks []models.LabCheck
	nextID int64
}

// NewLabCheckRepository builds a fake seeded with the given checks.
func NewLabCheckRepository(seed ...models.LabCheck) *LabCheckRepository {
	return &LabCheckRepository{checks: append([]models.LabCheck{}, seed...), nextID: int64(len(seed)) + 1}
}

// List returns checks most-recent-first, optionally filtered by course slug.
func (f *LabCheckRepository) List(_ context.Context, courseSlug string) ([]models.LabCheck, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	results := make([]models.LabCheck, 0, len(f.checks))

	for _, check := range slices.Backward(f.checks) {
		if courseSlug != "" && check.CourseSlug != courseSlug {
			continue
		}

		results = append(results, check)
	}

	return results, nil
}

// Create appends check to the fake store, assigning it an ID.
func (f *LabCheckRepository) Create(_ context.Context, check *models.LabCheck) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	check.ID = f.nextID
	f.nextID++
	f.checks = append(f.checks, *check)

	return nil
}
