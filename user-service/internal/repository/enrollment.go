package repository

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/genesary/pupitre/user-service/internal/models"
)

// EnrollmentRow is a single row of a user's enrolled-courses listing, joined
// with aggregate lesson/module progress.
type EnrollmentRow struct {
	Slug          string
	CompletedLabs int64
	TotalScore    int64
	LastActivity  *string
}

// CourseEnrollmentRow is a single row of the admin per-course enrollment
// listing, joined with the enrolled user's identity.
type CourseEnrollmentRow struct {
	UserID     string
	Username   string
	Email      string
	EnrolledAt string
}

// EnrollmentRepository is the persistence boundary for the enrollments
// table.
type EnrollmentRepository interface {
	Create(ctx context.Context, userID, courseSlug string) error
	Delete(ctx context.Context, userID, courseSlug string) error
	Exists(ctx context.Context, userID, courseSlug string) (bool, error)
	MyEnrollments(ctx context.Context, userID string) ([]EnrollmentRow, error)

	// Admin reporting (admin.go)
	CountAll(ctx context.Context) (int64, error)
	CountDistinctCourses(ctx context.Context) (int64, error)
	ListByCourse(ctx context.Context, courseSlug string) ([]CourseEnrollmentRow, error)
}

// gormEnrollmentRepository is the GORM-backed EnrollmentRepository impl.
type gormEnrollmentRepository struct {
	db *gorm.DB
}

// NewGormEnrollmentRepository builds an EnrollmentRepository backed by db.
//
//nolint:ireturn // repository constructors return the interface type by design
func NewGormEnrollmentRepository(db *gorm.DB) EnrollmentRepository {
	return &gormEnrollmentRepository{db: db}
}

// Create enrolls userID in courseSlug, doing nothing if already enrolled.
func (r *gormEnrollmentRepository) Create(ctx context.Context, userID, courseSlug string) error {
	enrollment := models.Enrollment{UserID: userID, CourseSlug: courseSlug}

	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&enrollment).Error
	if err != nil {
		return fmt.Errorf("create enrollment: %w", err)
	}

	return nil
}

// Delete removes userID's enrollment in courseSlug.
func (r *gormEnrollmentRepository) Delete(ctx context.Context, userID, courseSlug string) error {
	err := r.db.WithContext(ctx).
		Where("userid = ?::uuid AND courseslug = ?", userID, courseSlug).
		Delete(&models.Enrollment{}).Error
	if err != nil {
		return fmt.Errorf("delete enrollment: %w", err)
	}

	return nil
}

// Exists reports whether userID is enrolled in courseSlug.
func (r *gormEnrollmentRepository) Exists(ctx context.Context, userID, courseSlug string) (bool, error) {
	var count int64

	err := r.db.WithContext(ctx).Model(&models.Enrollment{}).
		Where("userid = ?::uuid AND courseslug = ?", userID, courseSlug).Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("check enrollment: %w", err)
	}

	return count > 0, nil
}

// MyEnrollments returns userID's enrollments with aggregate lesson/module
// progress, most recently enrolled first.
func (r *gormEnrollmentRepository) MyEnrollments(ctx context.Context, userID string) ([]EnrollmentRow, error) {
	var rows []EnrollmentRow

	err := r.db.WithContext(ctx).Table("enrollments AS e").
		Select(`e.courseslug AS slug,
			COUNT(DISTINCT lp.lessonslug) + COALESCE(mp.passedmodules, 0) AS completed_labs,
			COALESCE(mp.totalscore, 0) AS total_score,
			GREATEST(MAX(lp.viewed_at), mp.lastactivity)::text AS last_activity`).
		Joins("LEFT JOIN lesson_progress lp ON lp.userid = e.userid AND lp.courseslug = e.courseslug").
		Joins(`LEFT JOIN (
			SELECT courseslug,
			       SUM(bestscore) AS totalscore,
			       COUNT(*) FILTER (WHERE passed) AS passedmodules,
			       MAX(updatedat) AS lastactivity
			FROM module_progress
			WHERE userid = ?::uuid
			GROUP BY courseslug
		) mp ON mp.courseslug = e.courseslug`, userID).
		Where("e.userid = ?::uuid", userID).
		Group("e.courseslug, mp.totalscore, mp.passedmodules, mp.lastactivity, e.enrolledat").
		Order("e.enrolledat DESC").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("query my enrollments: %w", err)
	}

	return rows, nil
}

// CountAll returns the total number of enrollments across all courses.
func (r *gormEnrollmentRepository) CountAll(ctx context.Context) (int64, error) {
	var count int64

	err := r.db.WithContext(ctx).Model(&models.Enrollment{}).Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("count enrollments: %w", err)
	}

	return count, nil
}

// CountDistinctCourses returns the number of distinct courses with at least
// one enrollment.
func (r *gormEnrollmentRepository) CountDistinctCourses(ctx context.Context) (int64, error) {
	var count int64

	err := r.db.WithContext(ctx).Model(&models.Enrollment{}).
		Distinct(colCourseSlug).Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("count distinct enrolled courses: %w", err)
	}

	return count, nil
}

// ListByCourse returns every user enrolled in courseSlug, most recently
// enrolled first.
func (r *gormEnrollmentRepository) ListByCourse(ctx context.Context, courseSlug string) ([]CourseEnrollmentRow, error) {
	var rows []CourseEnrollmentRow

	err := r.db.WithContext(ctx).Table("enrollments AS e").
		Select("u.id::text AS user_id, u.username, u.email, e.enrolledat::text AS enrolled_at").
		Joins("JOIN users u ON u.id = e.userid").
		Where("e.courseslug = ?", courseSlug).
		Order("e.enrolledat DESC").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list course enrollments: %w", err)
	}

	return rows, nil
}
