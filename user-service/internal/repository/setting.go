package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/genesary/pupitre/user-service/internal/models"
)

// SettingRepository is the persistence boundary for the platform_settings
// table.
type SettingRepository interface {
	Get(ctx context.Context, key string) (string, bool, error)
	List(ctx context.Context) ([]models.PlatformSetting, error)
	Upsert(ctx context.Context, key, value string) error
}

type gormSettingRepository struct {
	db *gorm.DB
}

// NewGormSettingRepository builds a SettingRepository backed by db.
func NewGormSettingRepository(db *gorm.DB) SettingRepository {
	return &gormSettingRepository{db: db}
}

func (r *gormSettingRepository) Get(ctx context.Context, key string) (string, bool, error) {
	var setting models.PlatformSetting

	err := r.db.WithContext(ctx).Where("key = ?", key).First(&setting).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", false, nil
		}

		return "", false, fmt.Errorf("get setting %s: %w", key, err)
	}

	return setting.Value, true, nil
}

func (r *gormSettingRepository) List(ctx context.Context) ([]models.PlatformSetting, error) {
	settings := make([]models.PlatformSetting, 0)

	err := r.db.WithContext(ctx).Order("key").Find(&settings).Error
	if err != nil {
		return nil, fmt.Errorf("list settings: %w", err)
	}

	return settings, nil
}

func (r *gormSettingRepository) Upsert(ctx context.Context, key, value string) error {
	err := r.db.WithContext(ctx).Exec(`
		INSERT INTO platform_settings (key, value, updatedat)
		VALUES (?, ?, NOW())
		ON CONFLICT (key) DO UPDATE SET value = ?, updatedat = NOW()`,
		key, value, value).Error
	if err != nil {
		return fmt.Errorf("upsert setting %s: %w", key, err)
	}

	return nil
}

// ReadSetting reads a platform setting value by key, returning fallback when
// the key is unset or the query fails.
func ReadSetting(ctx context.Context, repo SettingRepository, key, fallback string) string {
	value, ok, err := repo.Get(ctx, key)
	if err != nil || !ok {
		return fallback
	}

	return value
}
