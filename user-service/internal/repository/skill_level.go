package repository

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/genesary/pupitre/user-service/internal/models"
)

// SkillLevelRow is a single row returned by skill level listing queries.
type SkillLevelRow struct {
	Skill string `json:"skill"`
	Level string `json:"level"`
}

// SkillLevelRepository is the persistence boundary for the user_skill_levels
// table.
type SkillLevelRepository interface {
	// Upsert sets the skill level for userID/skill to the higher of the stored
	// level and the provided level. Unknown difficulty strings are ignored.
	Upsert(ctx context.Context, userID, skill, level string) error
	// ListByUser returns all skill levels for userID, ordered by skill name.
	ListByUser(ctx context.Context, userID string) ([]SkillLevelRow, error)
}

// Numeric ordinals for the three difficulty levels — used to compare and
// store the "highest reached" level for a skill.
const (
	difficultyOrderBeginner     = 1
	difficultyOrderIntermediate = 2
	difficultyOrderAdvanced     = 3
)

// difficultyOrder maps a difficulty string to a comparable integer so that
// the "max" semantics of Upsert can be expressed as a numeric comparison.
func difficultyOrder(d string) int {
	switch d {
	case "beginner":
		return difficultyOrderBeginner
	case "intermediate":
		return difficultyOrderIntermediate
	case "advanced":
		return difficultyOrderAdvanced
	default:
		return 0
	}
}

// gormSkillLevelRepository is the GORM-backed SkillLevelRepository.
type gormSkillLevelRepository struct {
	db *gorm.DB
}

// NewGormSkillLevelRepository builds a SkillLevelRepository backed by db.
//
//nolint:ireturn // repository constructors return the interface type by design
func NewGormSkillLevelRepository(db *gorm.DB) SkillLevelRepository {
	return &gormSkillLevelRepository{db: db}
}

// Upsert inserts or updates the user's level for skill. The stored level is
// only updated when the incoming level is strictly higher than the current one.
func (r *gormSkillLevelRepository) Upsert(ctx context.Context, userID, skill, level string) error {
	order := difficultyOrder(level)
	if order == 0 {
		return nil
	}

	err := r.db.WithContext(ctx).Exec(`
		INSERT INTO user_skill_levels (userid, skill, level, levelorder, updatedat)
		VALUES (?::uuid, ?, ?, ?, NOW())
		ON CONFLICT (userid, skill) DO UPDATE
		SET level      = EXCLUDED.level,
		    levelorder = EXCLUDED.levelorder,
		    updatedat  = NOW()
		WHERE EXCLUDED.levelorder > user_skill_levels.levelorder
	`, userID, skill, level, order).Error
	if err != nil {
		return fmt.Errorf("upsert skill level: %w", err)
	}

	return nil
}

// ListByUser returns all skill levels for userID ordered alphabetically by
// skill name.
func (r *gormSkillLevelRepository) ListByUser(ctx context.Context, userID string) ([]SkillLevelRow, error) {
	var rows []SkillLevelRow

	err := r.db.WithContext(ctx).
		Model(&models.UserSkillLevel{}).
		Select("skill, level").
		Where("userid = ?::uuid", userID).
		Order("skill ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list skill levels: %w", err)
	}

	return rows, nil
}
