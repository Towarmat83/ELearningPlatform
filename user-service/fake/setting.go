package fake

import (
	"context"
	"sync"
	"time"

	"github.com/genesary/pupitre/user-service/internal/models"
)

// SettingRepository is an in-memory repository.SettingRepository for tests.
type SettingRepository struct {
	mu       sync.Mutex
	settings map[string]models.PlatformSetting

	// Err, when set, is returned by every method call (simulates a DB failure).
	Err error
}

// NewSettingRepository builds a fake SettingRepository seeded with settings.
func NewSettingRepository(seed ...models.PlatformSetting) *SettingRepository {
	m := make(map[string]models.PlatformSetting, len(seed))
	for _, s := range seed {
		m[s.Key] = s
	}

	return &SettingRepository{settings: m}
}

func (f *SettingRepository) Get(_ context.Context, key string) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return "", false, f.Err
	}

	s, ok := f.settings[key]
	if !ok {
		return "", false, nil
	}

	return s.Value, true, nil
}

func (f *SettingRepository) List(_ context.Context) ([]models.PlatformSetting, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return nil, f.Err
	}

	list := make([]models.PlatformSetting, 0, len(f.settings))
	for _, s := range f.settings {
		list = append(list, s)
	}

	return list, nil
}

func (f *SettingRepository) Upsert(_ context.Context, key, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return f.Err
	}

	f.settings[key] = models.PlatformSetting{Key: key, Value: value, UpdatedAt: time.Now()}

	return nil
}
