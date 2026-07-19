// Package repository defines data-access interfaces for course-service's
// persisted entities, decoupling handlers from the underlying storage
// technology (GORM/Postgres in production, in-memory fakes in tests).
package repository

import (
	"context"

	"gorm.io/gorm"

	"github.com/genesary/pupitre/course-service/internal/models"
)

// maxLabResults caps the number of rows returned by List, mirroring the
// LIMIT already enforced by the raw-SQL query it replaces.
const maxLabResults = 500

// LabCheckRepository persists and queries recorded lab check outcomes.
type LabCheckRepository interface {
	// List returns up to 500 lab checks, most recent first, optionally
	// filtered to a single course slug (empty string means no filter).
	List(ctx context.Context, courseSlug string) ([]models.LabCheck, error)
	// Create inserts a new lab check row.
	Create(ctx context.Context, check *models.LabCheck) error
}

// gormLabCheckRepository is the GORM-backed LabCheckRepository.
type gormLabCheckRepository struct {
	db *gorm.DB
}

// NewGormLabCheckRepository builds a LabCheckRepository backed by db.
func NewGormLabCheckRepository(db *gorm.DB) LabCheckRepository {
	return &gormLabCheckRepository{db: db}
}

func (r *gormLabCheckRepository) List(ctx context.Context, courseSlug string) ([]models.LabCheck, error) {
	query := r.db.WithContext(ctx).Order("checkedat DESC").Limit(maxLabResults)
	if courseSlug != "" {
		query = query.Where("courseslug = ?", courseSlug)
	}

	results := make([]models.LabCheck, 0)

	err := query.Find(&results).Error
	if err != nil {
		return nil, err
	}

	return results, nil
}

func (r *gormLabCheckRepository) Create(ctx context.Context, check *models.LabCheck) error {
	return r.db.WithContext(ctx).Create(check).Error
}
