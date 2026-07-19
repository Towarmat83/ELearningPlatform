package repository

import (
	"context"
	"fmt"

	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/genesary/pupitre/user-service/internal/models"
)

// lessonSlugComplete is the sentinel lessonslug value that marks a course as
// fully completed (as opposed to an individual lesson view).
const lessonSlugComplete = "__complete__"

// LessonProgressRepository is the persistence boundary for the
// lesson_progress table.
type LessonProgressRepository interface {
	MarkComplete(ctx context.Context, userID, courseSlug, lessonSlug string) error
	ViewedSlugs(ctx context.Context, userID, courseSlug string) ([]string, error)
	CountViewed(ctx context.Context, userID, courseSlug string) (int64, error)

	// CompletedCourseSlugs returns the subset of slugs the user has marked
	// complete via the __complete__ sentinel. Paired with
	// ModuleProgressRepository's method of the same name — see the doc
	// comment there for why this isn't one cross-aggregate method.
	CompletedCourseSlugs(ctx context.Context, userID string, slugs []string) ([]string, error)
}

type gormLessonProgressRepository struct {
	db *gorm.DB
}

// NewGormLessonProgressRepository builds a LessonProgressRepository backed
// by db.
func NewGormLessonProgressRepository(db *gorm.DB) LessonProgressRepository {
	return &gormLessonProgressRepository{db: db}
}

func (r *gormLessonProgressRepository) MarkComplete(ctx context.Context, userID, courseSlug, lessonSlug string) error {
	err := r.db.WithContext(ctx).Exec(`
		INSERT INTO lesson_progress (userid, courseslug, lessonslug)
		VALUES (?::uuid, ?, ?)
		ON CONFLICT (userid, courseslug, lessonslug) DO NOTHING`,
		userID, courseSlug, lessonSlug).Error
	if err != nil {
		return fmt.Errorf("mark lesson complete: %w", err)
	}

	return nil
}

func (r *gormLessonProgressRepository) ViewedSlugs(ctx context.Context, userID, courseSlug string) ([]string, error) {
	var slugs []string

	err := r.db.WithContext(ctx).Model(&models.LessonProgress{}).
		Where("userid = ?::uuid AND courseslug = ?", userID, courseSlug).
		Pluck("lessonslug", &slugs).Error
	if err != nil {
		return nil, fmt.Errorf("list viewed lessons: %w", err)
	}

	return slugs, nil
}

func (r *gormLessonProgressRepository) CountViewed(ctx context.Context, userID, courseSlug string) (int64, error) {
	var count int64

	err := r.db.WithContext(ctx).Model(&models.LessonProgress{}).
		Where("userid = ?::uuid AND courseslug = ?", userID, courseSlug).Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("count viewed lessons: %w", err)
	}

	return count, nil
}

func (r *gormLessonProgressRepository) CompletedCourseSlugs(ctx context.Context, userID string, slugs []string) ([]string, error) {
	var completed []string

	err := r.db.WithContext(ctx).Model(&models.LessonProgress{}).
		Where("userid = ?::uuid AND lessonslug = ? AND courseslug = ANY(?)", userID, lessonSlugComplete, pq.StringArray(slugs)).
		Distinct().Pluck("courseslug", &completed).Error
	if err != nil {
		return nil, fmt.Errorf("list completed course slugs: %w", err)
	}

	return completed, nil
}
