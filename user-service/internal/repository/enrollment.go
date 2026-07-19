package repository

import (
	"context"
	"fmt"

	"gorm.io/gorm"

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
	err := r.db.WithContext(ctx).Exec(`
		INSERT INTO enrollments (userid, courseslug)
		VALUES (?::uuid, ?)
		ON CONFLICT DO NOTHING`, userID, courseSlug).Error
	if err != nil {
		return fmt.Errorf("create enrollment: %w", err)
	}

	return nil
}

// Delete removes userID's enrollment in courseSlug.
func (r *gormEnrollmentRepository) Delete(ctx context.Context, userID, courseSlug string) error {
	err := r.db.WithContext(ctx).Exec(`
		DELETE FROM enrollments WHERE userid = ?::uuid AND courseslug = ?`, userID, courseSlug).Error
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

// MyEnrollments mirrors the original hand-rolled join+subquery exactly (kept
// as raw SQL, not the query builder, per the ORM migration plan's guidance
// on reporting queries).
func (r *gormEnrollmentRepository) MyEnrollments(ctx context.Context, userID string) ([]EnrollmentRow, error) {
	var rows []EnrollmentRow

	err := r.db.WithContext(ctx).Raw(`
		SELECT e.courseslug AS slug,
		       COUNT(DISTINCT lp.lessonslug) + COALESCE(mp.passedmodules, 0) AS completed_labs,
		       COALESCE(mp.totalscore, 0) AS total_score,
		       GREATEST(MAX(lp.viewed_at), mp.lastactivity)::text AS last_activity
		FROM enrollments e
		LEFT JOIN lesson_progress lp ON lp.userid = e.userid AND lp.courseslug = e.courseslug
		LEFT JOIN (
			SELECT courseslug,
			       SUM(bestscore) AS totalscore,
			       COUNT(*) FILTER (WHERE passed) AS passedmodules,
			       MAX(updatedat) AS lastactivity
			FROM module_progress
			WHERE userid = ?::uuid
			GROUP BY courseslug
		) mp ON mp.courseslug = e.courseslug
		WHERE e.userid = ?::uuid
		GROUP BY e.courseslug, mp.totalscore, mp.passedmodules, mp.lastactivity, e.enrolledat
		ORDER BY e.enrolledat DESC`, userID, userID).Scan(&rows).Error
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
		Distinct("courseslug").Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("count distinct enrolled courses: %w", err)
	}

	return count, nil
}

// ListByCourse mirrors the original hand-rolled join exactly (kept as raw
// SQL, not the query builder, per the ORM migration plan's guidance on
// reporting queries).
func (r *gormEnrollmentRepository) ListByCourse(ctx context.Context, courseSlug string) ([]CourseEnrollmentRow, error) {
	var rows []CourseEnrollmentRow

	err := r.db.WithContext(ctx).Raw(`
		SELECT u.id::text AS user_id, u.username, u.email, e.enrolledat::text AS enrolled_at
		FROM enrollments e
		JOIN users u ON u.id = e.userid
		WHERE e.courseslug = ?
		ORDER BY e.enrolledat DESC`, courseSlug).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list course enrollments: %w", err)
	}

	return rows, nil
}
