package repository

import (
	"context"
	"fmt"

	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/genesary/pupitre/user-service/internal/models"
)

// ModuleProgressRepository is the persistence boundary for the
// module_progress table.
type ModuleProgressRepository interface {
	// RecordProgress upserts a module attempt: it keeps the best score, ORs
	// the passed flag across attempts, and increments the attempt count.
	RecordProgress(ctx context.Context, userID, courseSlug string, moduleIndex int, moduleSlug string, score, maxScore int, passed bool) error
	TotalScore(ctx context.Context, userID, courseSlug string) (int64, error)
	PassedModuleSlugs(ctx context.Context, userID, courseSlug string) ([]string, error)
	ListByUserCourse(ctx context.Context, userID, courseSlug string) ([]models.ModuleProgress, error)

	// CompletedCourseSlugs returns the subset of slugs the user has passed
	// at least one module in. Paired with LessonProgressRepository's method
	// of the same name and unioned in the handler layer — see paths.go's
	// completedCoursesCtx — instead of one cross-aggregate repository
	// method, so each repository (and its fake) only ever touches its own
	// table.
	CompletedCourseSlugs(ctx context.Context, userID string, slugs []string) ([]string, error)
}

// gormModuleProgressRepository is the GORM-backed ModuleProgressRepository.
type gormModuleProgressRepository struct {
	db *gorm.DB
}

// NewGormModuleProgressRepository builds a ModuleProgressRepository backed
// by db.
//
//nolint:ireturn // repository constructors return the interface type by design
func NewGormModuleProgressRepository(db *gorm.DB) ModuleProgressRepository {
	return &gormModuleProgressRepository{db: db}
}

// RecordProgress is kept as raw SQL (not the query builder), per the ORM
// migration plan's guidance to port the keep-best upsert verbatim.
func (r *gormModuleProgressRepository) RecordProgress(
	ctx context.Context, userID, courseSlug string, moduleIndex int, moduleSlug string, score, maxScore int, passed bool,
) error {
	err := r.db.WithContext(ctx).Exec(`
		INSERT INTO module_progress (userid, courseslug, moduleindex, moduleslug, bestscore, maxscore, passed, attempts, completed_at)
		VALUES (?::uuid, ?, ?, NULLIF(?, ''), ?, ?, ?, 1, CASE WHEN ? THEN NOW() ELSE NULL END)
		ON CONFLICT (userid, courseslug, moduleindex) DO UPDATE SET
			attempts     = module_progress.attempts + 1,
			bestscore    = GREATEST(module_progress.bestscore, ?),
			maxscore     = ?,
			passed       = module_progress.passed OR ?,
			moduleslug   = COALESCE(module_progress.moduleslug, NULLIF(?, '')),
			completed_at = CASE WHEN (? AND module_progress.completed_at IS NULL) THEN NOW() ELSE module_progress.completed_at END,
			updatedat    = NOW()`,
		userID, courseSlug, moduleIndex, moduleSlug, score, maxScore, passed, passed,
		score, maxScore, passed, moduleSlug, passed).Error
	if err != nil {
		return fmt.Errorf("record module progress: %w", err)
	}

	return nil
}

// TotalScore returns the sum of userID's best scores across every module in
// courseSlug.
func (r *gormModuleProgressRepository) TotalScore(ctx context.Context, userID, courseSlug string) (int64, error) {
	var total int64

	err := r.db.WithContext(ctx).Raw(`
		SELECT COALESCE(SUM(bestscore), 0) FROM module_progress
		WHERE userid = ?::uuid AND courseslug = ?`, userID, courseSlug).Scan(&total).Error
	if err != nil {
		return 0, fmt.Errorf("total module score: %w", err)
	}

	return total, nil
}

// PassedModuleSlugs returns the module slugs userID has passed in
// courseSlug.
func (r *gormModuleProgressRepository) PassedModuleSlugs(ctx context.Context, userID, courseSlug string) ([]string, error) {
	var slugs []string

	err := r.db.WithContext(ctx).Model(&models.ModuleProgress{}).
		Where("userid = ?::uuid AND courseslug = ? AND passed = TRUE AND moduleslug IS NOT NULL", userID, courseSlug).
		Pluck("moduleslug", &slugs).Error
	if err != nil {
		return nil, fmt.Errorf("list passed module slugs: %w", err)
	}

	return slugs, nil
}

// ListByUserCourse returns every module_progress row for userID in
// courseSlug.
func (r *gormModuleProgressRepository) ListByUserCourse(ctx context.Context, userID, courseSlug string) ([]models.ModuleProgress, error) {
	var list []models.ModuleProgress

	err := r.db.WithContext(ctx).
		Where("userid = ?::uuid AND courseslug = ?", userID, courseSlug).
		Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("list module progress: %w", err)
	}

	return list, nil
}

// CompletedCourseSlugs returns the subset of slugs userID has passed at
// least one module in.
func (r *gormModuleProgressRepository) CompletedCourseSlugs(ctx context.Context, userID string, slugs []string) ([]string, error) {
	var completed []string

	err := r.db.WithContext(ctx).Model(&models.ModuleProgress{}).
		Where("userid = ?::uuid AND passed = true AND courseslug = ANY(?)", userID, pq.StringArray(slugs)).
		Distinct().Pluck("courseslug", &completed).Error
	if err != nil {
		return nil, fmt.Errorf("list completed course slugs: %w", err)
	}

	return completed, nil
}
