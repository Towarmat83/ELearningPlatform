package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/genesary/pupitre/user-service/internal/models"
)

// BadgeRow is a single row returned by badge listing queries.
type BadgeRow struct {
	CourseSlug string
	EarnedAt   time.Time
}

// BadgeRepository is the persistence boundary for the user_badges table.
type BadgeRepository interface {
	// Award grants courseSlug's badge to userID. Idempotent: re-awarding an
	// already-earned badge is a no-op.
	Award(ctx context.Context, userID, courseSlug string) error
	// MyBadges returns all badges earned by userID, most recently earned first.
	MyBadges(ctx context.Context, userID string) ([]BadgeRow, error)
	// UserBadges returns all badges earned by the given userID (public view).
	UserBadges(ctx context.Context, userID string) ([]BadgeRow, error)
	// EarnedCount returns how many distinct users have earned the badge for courseSlug.
	EarnedCount(ctx context.Context, courseSlug string) (int64, error)
}

// gormBadgeRepository is the GORM-backed BadgeRepository.
type gormBadgeRepository struct {
	db *gorm.DB
}

// NewGormBadgeRepository builds a BadgeRepository backed by db.
//
//nolint:ireturn // repository constructors return the interface type by design
func NewGormBadgeRepository(db *gorm.DB) BadgeRepository {
	return &gormBadgeRepository{db: db}
}

// Award inserts a user_badge row, doing nothing if one already exists.
func (r *gormBadgeRepository) Award(ctx context.Context, userID, courseSlug string) error {
	badge := models.UserBadge{UserID: userID, CourseSlug: courseSlug}

	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&badge).Error
	if err != nil {
		return fmt.Errorf("award badge: %w", err)
	}

	return nil
}

// MyBadges returns all badges earned by userID, most recently earned first.
func (r *gormBadgeRepository) MyBadges(ctx context.Context, userID string) ([]BadgeRow, error) {
	var rows []BadgeRow

	err := r.db.WithContext(ctx).Model(&models.UserBadge{}).
		Select("courseslug AS course_slug, earnedat AS earned_at").
		Where("userid = ?::uuid", userID).
		Order("earnedat DESC").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list my badges: %w", err)
	}

	return rows, nil
}

// UserBadges returns all badges earned by userID (public view).
func (r *gormBadgeRepository) UserBadges(ctx context.Context, userID string) ([]BadgeRow, error) {
	return r.MyBadges(ctx, userID)
}

// EarnedCount returns how many distinct users have earned the badge
// for courseSlug.
func (r *gormBadgeRepository) EarnedCount(ctx context.Context, courseSlug string) (int64, error) {
	var count int64

	err := r.db.WithContext(ctx).Model(&models.UserBadge{}).
		Where("courseslug = ?", courseSlug).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("count badge earners: %w", err)
	}

	return count, nil
}
