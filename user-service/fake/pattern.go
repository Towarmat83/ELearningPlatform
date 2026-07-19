package fake

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/genesary/pupitre/user-service/internal/models"
	"github.com/genesary/pupitre/user-service/internal/repository"
)

// patternGlobalScope mirrors repository's unexported scope constant.
const patternGlobalScope = "global"

// PatternRepository is an in-memory repository.PatternRepository for tests.
type PatternRepository struct {
	mu       sync.Mutex
	patterns []models.MarkdownPattern

	// Err, when set, is returned by every method call (simulates a DB failure).
	Err error
}

// NewPatternRepository builds a fake PatternRepository seeded with patterns.
func NewPatternRepository(seed ...models.MarkdownPattern) *PatternRepository {
	return &PatternRepository{patterns: append([]models.MarkdownPattern{}, seed...)}
}

func (f *PatternRepository) findIndexByID(id uuid.UUID) int {
	for i := range f.patterns {
		if f.patterns[i].ID == id {
			return i
		}
	}

	return -1
}

func (f *PatternRepository) findIndexByName(name string) int {
	for i := range f.patterns {
		if f.patterns[i].Name == name {
			return i
		}
	}

	return -1
}

func (f *PatternRepository) findIndexByNameScope(name, scope string) int {
	for i := range f.patterns {
		if f.patterns[i].Name == name && f.patterns[i].Scope == scope {
			return i
		}
	}

	return -1
}

func (f *PatternRepository) ListGlobal(_ context.Context) ([]models.MarkdownPattern, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return nil, f.Err
	}

	list := make([]models.MarkdownPattern, 0)

	for _, p := range f.patterns {
		if p.Scope == patternGlobalScope {
			list = append(list, p)
		}
	}

	return list, nil
}

func (f *PatternRepository) ListForCourse(_ context.Context, courseSlug string) ([]models.MarkdownPattern, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return nil, f.Err
	}

	list := make([]models.MarkdownPattern, 0)

	for _, p := range f.patterns {
		if p.Scope == patternGlobalScope || p.Scope == courseSlug {
			list = append(list, p)
		}
	}

	return list, nil
}

func (f *PatternRepository) Get(_ context.Context, id uuid.UUID) (*models.MarkdownPattern, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return nil, f.Err
	}

	i := f.findIndexByID(id)
	if i < 0 {
		return nil, repository.ErrPatternNotFound
	}

	cp := f.patterns[i]

	return &cp, nil
}

func (f *PatternRepository) Create(
	_ context.Context, name, label, description, parameter, html, css, js, scope, createdBy string,
) (*models.MarkdownPattern, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return nil, f.Err
	}

	if f.findIndexByNameScope(name, scope) >= 0 {
		return nil, errors.New("duplicate pattern name in scope")
	}

	now := time.Now()

	createdByID, err := uuid.Parse(createdBy)

	var createdByPtr *uuid.UUID
	if err == nil {
		createdByPtr = &createdByID
	}

	pattern := models.MarkdownPattern{
		ID: uuid.New(), Name: name, Label: label, Description: description, Parameter: parameter,
		HTML: html, CSS: css, JS: js, Scope: scope, CreatedBy: createdByPtr,
		CreatedAt: now, UpdatedAt: now,
	}

	f.patterns = append(f.patterns, pattern)

	return &pattern, nil
}

func (f *PatternRepository) UpdateByName(
	_ context.Context, oldName, name, label, description, parameter, html, css, js, scope string,
) (*models.MarkdownPattern, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return nil, f.Err
	}

	i := f.findIndexByName(oldName)
	if i < 0 {
		return nil, repository.ErrPatternNotFound
	}

	f.patterns[i].Name = name
	f.patterns[i].Label = label
	f.patterns[i].Description = description
	f.patterns[i].Parameter = parameter
	f.patterns[i].HTML = html
	f.patterns[i].CSS = css
	f.patterns[i].JS = js
	f.patterns[i].Scope = scope
	f.patterns[i].FromConfig = false
	f.patterns[i].UpdatedAt = time.Now()

	cp := f.patterns[i]

	return &cp, nil
}

func (f *PatternRepository) DeleteByName(_ context.Context, name string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return false, f.Err
	}

	i := f.findIndexByName(name)
	if i < 0 {
		return false, nil
	}

	f.patterns = append(f.patterns[:i], f.patterns[i+1:]...)

	return true, nil
}

func (f *PatternRepository) DeleteByIDAndScope(_ context.Context, id uuid.UUID, scope string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return false, f.Err
	}

	i := f.findIndexByID(id)
	if i < 0 || f.patterns[i].Scope != scope {
		return false, nil
	}

	f.patterns = append(f.patterns[:i], f.patterns[i+1:]...)

	return true, nil
}

func (f *PatternRepository) UpsertFromConfig(
	_ context.Context, name, label, description, parameter, html, css, js, scope string,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return f.Err
	}

	now := time.Now()

	i := f.findIndexByNameScope(name, scope)
	if i >= 0 {
		f.patterns[i].Label = label
		f.patterns[i].Description = description
		f.patterns[i].Parameter = parameter
		f.patterns[i].HTML = html
		f.patterns[i].CSS = css
		f.patterns[i].JS = js
		f.patterns[i].FromConfig = true
		f.patterns[i].UpdatedAt = now

		return nil
	}

	f.patterns = append(f.patterns, models.MarkdownPattern{
		ID: uuid.New(), Name: name, Label: label, Description: description, Parameter: parameter,
		HTML: html, CSS: css, JS: js, Scope: scope, FromConfig: true, CreatedAt: now, UpdatedAt: now,
	})

	return nil
}

func (f *PatternRepository) UpsertFromCRD(_ context.Context, name, label, description, html, css, js, scope string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return f.Err
	}

	now := time.Now()

	i := f.findIndexByNameScope(name, scope)
	if i >= 0 {
		f.patterns[i].Label = label
		f.patterns[i].Description = description
		f.patterns[i].HTML = html
		f.patterns[i].CSS = css
		f.patterns[i].JS = js
		f.patterns[i].FromConfig = true
		f.patterns[i].UpdatedAt = now

		return nil
	}

	f.patterns = append(f.patterns, models.MarkdownPattern{
		ID: uuid.New(), Name: name, Label: label, Description: description,
		HTML: html, CSS: css, JS: js, Scope: scope, FromConfig: true, CreatedAt: now, UpdatedAt: now,
	})

	return nil
}

func (f *PatternRepository) DeleteFromCRD(_ context.Context, name, scope string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return f.Err
	}

	i := f.findIndexByNameScope(name, scope)
	if i < 0 || !f.patterns[i].FromConfig {
		return nil
	}

	f.patterns = append(f.patterns[:i], f.patterns[i+1:]...)

	return nil
}
