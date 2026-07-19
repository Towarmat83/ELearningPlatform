package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/genesary/pupitre/user-service/internal/models"
)

// ErrPatternNotFound is returned by lookups that find no matching pattern.
var ErrPatternNotFound = errors.New("pattern not found")

// patternGlobalScope is the scope value shared by patterns visible to every
// course, as opposed to a single course-specific scope.
const patternGlobalScope = "global"

// patternReturning lists every markdown_patterns column, used by Create and
// UpdateByName's RETURNING clause.
const patternReturning = `id, name, label, description, parameter, html, css, js, scope, from_config, createdby, createdat, updatedat`

// PatternRepository is the persistence boundary for the markdown_patterns
// table.
type PatternRepository interface {
	ListGlobal(ctx context.Context) ([]models.MarkdownPattern, error)
	ListForCourse(ctx context.Context, courseSlug string) ([]models.MarkdownPattern, error)
	Get(ctx context.Context, id uuid.UUID) (*models.MarkdownPattern, error)
	Create(ctx context.Context, name, label, description, parameter, html, css, js, scope, createdBy string) (*models.MarkdownPattern, error)
	UpdateByName(ctx context.Context, oldName, name, label, description, parameter, html, css, js, scope string) (*models.MarkdownPattern, error)
	DeleteByName(ctx context.Context, name string) (bool, error)
	DeleteByIDAndScope(ctx context.Context, id uuid.UUID, scope string) (bool, error)

	// UpsertFromConfig and UpsertFromCRD are named ON CONFLICT (name, scope)
	// DO UPDATE upserts, kept as raw SQL per the ORM migration plan's
	// guidance. They intentionally differ: config upserts also refresh
	// `parameter`, matching the original hand-rolled SQL for each caller
	// (patterns.go's LoadPatternsFromConfig vs. pattern_watcher.go's CRD sync).
	UpsertFromConfig(ctx context.Context, name, label, description, parameter, html, css, js, scope string) error
	UpsertFromCRD(ctx context.Context, name, label, description, html, css, js, scope string) error
	DeleteFromCRD(ctx context.Context, name, scope string) error
}

type gormPatternRepository struct {
	db *gorm.DB
}

// NewGormPatternRepository builds a PatternRepository backed by db.
func NewGormPatternRepository(db *gorm.DB) PatternRepository {
	return &gormPatternRepository{db: db}
}

func (r *gormPatternRepository) ListGlobal(ctx context.Context) ([]models.MarkdownPattern, error) {
	patterns := make([]models.MarkdownPattern, 0)

	err := r.db.WithContext(ctx).Where("scope = ?", patternGlobalScope).Order("name").Find(&patterns).Error
	if err != nil {
		return nil, fmt.Errorf("list global patterns: %w", err)
	}

	return patterns, nil
}

func (r *gormPatternRepository) ListForCourse(ctx context.Context, courseSlug string) ([]models.MarkdownPattern, error) {
	patterns := make([]models.MarkdownPattern, 0)

	err := r.db.WithContext(ctx).
		Where("scope = ? OR scope = ?", patternGlobalScope, courseSlug).
		Order("scope, name").Find(&patterns).Error
	if err != nil {
		return nil, fmt.Errorf("list course patterns: %w", err)
	}

	return patterns, nil
}

func (r *gormPatternRepository) Get(ctx context.Context, id uuid.UUID) (*models.MarkdownPattern, error) {
	var pattern models.MarkdownPattern

	err := r.db.WithContext(ctx).Where("id = ?", id).First(&pattern).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPatternNotFound
		}

		return nil, fmt.Errorf("get pattern: %w", err)
	}

	return &pattern, nil
}

func (r *gormPatternRepository) Create(
	ctx context.Context, name, label, description, parameter, html, css, js, scope, createdBy string,
) (*models.MarkdownPattern, error) {
	var pattern models.MarkdownPattern

	err := r.db.WithContext(ctx).Raw(`
		INSERT INTO markdown_patterns (name, label, description, parameter, html, css, js, scope, createdby)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING `+patternReturning,
		name, label, description, parameter, html, css, js, scope, createdBy).Scan(&pattern).Error
	if err != nil {
		return nil, fmt.Errorf("create pattern: %w", err)
	}

	return &pattern, nil
}

func (r *gormPatternRepository) UpdateByName(
	ctx context.Context, oldName, name, label, description, parameter, html, css, js, scope string,
) (*models.MarkdownPattern, error) {
	var pattern models.MarkdownPattern

	err := r.db.WithContext(ctx).Raw(`
		UPDATE markdown_patterns
		SET name = ?, label = ?, description = ?, parameter = ?, html = ?, css = ?, js = ?,
		    scope = ?, from_config = FALSE, updatedat = NOW()
		WHERE name = ?
		RETURNING `+patternReturning,
		name, label, description, parameter, html, css, js, scope, oldName).Scan(&pattern).Error
	if err != nil {
		return nil, fmt.Errorf("update pattern: %w", err)
	}

	if pattern.ID == uuid.Nil {
		return nil, ErrPatternNotFound
	}

	return &pattern, nil
}

func (r *gormPatternRepository) DeleteByName(ctx context.Context, name string) (bool, error) {
	result := r.db.WithContext(ctx).Where("name = ?", name).Delete(&models.MarkdownPattern{})
	if result.Error != nil {
		return false, fmt.Errorf("delete pattern: %w", result.Error)
	}

	return result.RowsAffected > 0, nil
}

func (r *gormPatternRepository) DeleteByIDAndScope(ctx context.Context, id uuid.UUID, scope string) (bool, error) {
	result := r.db.WithContext(ctx).Where("id = ? AND scope = ?", id, scope).Delete(&models.MarkdownPattern{})
	if result.Error != nil {
		return false, fmt.Errorf("delete course pattern: %w", result.Error)
	}

	return result.RowsAffected > 0, nil
}

func (r *gormPatternRepository) UpsertFromConfig(
	ctx context.Context, name, label, description, parameter, html, css, js, scope string,
) error {
	err := r.db.WithContext(ctx).Exec(`
		INSERT INTO markdown_patterns (name, label, description, parameter, html, css, js, scope, from_config)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, TRUE)
		ON CONFLICT (name, scope) DO UPDATE
		  SET label       = EXCLUDED.label,
		      description = EXCLUDED.description,
		      parameter   = EXCLUDED.parameter,
		      html        = EXCLUDED.html,
		      css         = EXCLUDED.css,
		      js          = EXCLUDED.js,
		      from_config = TRUE,
		      updatedat   = NOW()`,
		name, label, description, parameter, html, css, js, scope).Error
	if err != nil {
		return fmt.Errorf("upsert pattern %q: %w", name, err)
	}

	return nil
}

func (r *gormPatternRepository) UpsertFromCRD(
	ctx context.Context, name, label, description, html, css, js, scope string,
) error {
	err := r.db.WithContext(ctx).Exec(`
		INSERT INTO markdown_patterns (name, label, description, html, css, js, scope, from_config)
		VALUES (?, ?, ?, ?, ?, ?, ?, TRUE)
		ON CONFLICT (name, scope) DO UPDATE SET
			label       = EXCLUDED.label,
			description = EXCLUDED.description,
			html        = EXCLUDED.html,
			css         = EXCLUDED.css,
			js          = EXCLUDED.js,
			from_config = TRUE,
			updatedat   = NOW()`,
		name, label, description, html, css, js, scope).Error
	if err != nil {
		return fmt.Errorf("upsert pattern from CRD %q: %w", name, err)
	}

	return nil
}

func (r *gormPatternRepository) DeleteFromCRD(ctx context.Context, name, scope string) error {
	err := r.db.WithContext(ctx).Exec(
		`DELETE FROM markdown_patterns WHERE name = ? AND scope = ? AND from_config = TRUE`, name, scope).Error
	if err != nil {
		return fmt.Errorf("delete pattern from CRD %q: %w", name, err)
	}

	return nil
}
