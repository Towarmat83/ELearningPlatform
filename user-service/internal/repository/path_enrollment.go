package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/genesary/pupitre/user-service/internal/models"
)

// PathEnrollmentRow is a single row of a user's path-enrollments listing.
type PathEnrollmentRow struct {
	Slug       string
	EnrolledAt time.Time
}

// PathEnrolledUserRow is a single row of the admin per-path enrollment
// listing, joined with the enrolled user's identity.
type PathEnrolledUserRow struct {
	UserID     string
	Email      string
	Role       string
	EnrolledAt time.Time
}

// PathEnrollmentRepository is the persistence boundary for the
// path_enrollments table.
type PathEnrollmentRepository interface {
	MyEnrollments(ctx context.Context, userID string, limit *int, offset int) ([]PathEnrollmentRow, error)
	ListBySlug(ctx context.Context, pathSlug string) ([]PathEnrolledUserRow, error)
	Enroll(ctx context.Context, userID, pathSlug string) error
	Unenroll(ctx context.Context, userID, pathSlug string) error
}

type gormPathEnrollmentRepository struct {
	db *gorm.DB
}

// NewGormPathEnrollmentRepository builds a PathEnrollmentRepository backed by
// db.
func NewGormPathEnrollmentRepository(db *gorm.DB) PathEnrollmentRepository {
	return &gormPathEnrollmentRepository{db: db}
}

func (r *gormPathEnrollmentRepository) MyEnrollments(
	ctx context.Context, userID string, limit *int, offset int,
) ([]PathEnrollmentRow, error) {
	rows := make([]PathEnrollmentRow, 0)

	query := r.db.WithContext(ctx).Model(&models.PathEnrollment{}).
		Select("path_slug AS slug, enrolledat AS enrolled_at").
		Where("userid = ?::uuid", userID).
		Order("enrolledat DESC").
		Offset(offset)
	if limit != nil {
		query = query.Limit(*limit)
	}

	err := query.Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("query path enrollments: %w", err)
	}

	return rows, nil
}

// ListBySlug mirrors the original hand-rolled join exactly (kept as raw SQL,
// not the query builder, per the ORM migration plan's guidance on reporting
// queries).
func (r *gormPathEnrollmentRepository) ListBySlug(ctx context.Context, pathSlug string) ([]PathEnrolledUserRow, error) {
	rows := make([]PathEnrolledUserRow, 0)

	err := r.db.WithContext(ctx).Raw(`
		SELECT u.id::text AS user_id, u.email, u.role, pe.enrolledat AS enrolled_at
		FROM path_enrollments pe
		JOIN users u ON u.id = pe.userid
		WHERE pe.path_slug = ?
		ORDER BY pe.enrolledat DESC`, pathSlug).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list path enrollments: %w", err)
	}

	return rows, nil
}

func (r *gormPathEnrollmentRepository) Enroll(ctx context.Context, userID, pathSlug string) error {
	err := r.db.WithContext(ctx).Exec(`
		INSERT INTO path_enrollments (userid, path_slug) VALUES (?::uuid, ?) ON CONFLICT DO NOTHING`,
		userID, pathSlug).Error
	if err != nil {
		return fmt.Errorf("enroll in path: %w", err)
	}

	return nil
}

func (r *gormPathEnrollmentRepository) Unenroll(ctx context.Context, userID, pathSlug string) error {
	err := r.db.WithContext(ctx).Exec(`
		DELETE FROM path_enrollments WHERE userid = ?::uuid AND path_slug = ?`, userID, pathSlug).Error
	if err != nil {
		return fmt.Errorf("unenroll from path: %w", err)
	}

	return nil
}
