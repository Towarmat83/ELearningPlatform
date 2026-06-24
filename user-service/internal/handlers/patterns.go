package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"

	"github.com/elearning/user-service/internal/db"
)

type MarkdownPattern struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Label       string    `json:"label"`
	Description string    `json:"description"`
	Parameter   string    `json:"parameter"`
	HTML        string    `json:"html"`
	CSS         string    `json:"css"`
	JS          string    `json:"js"`
	Scope       string    `json:"scope"`
	FromConfig  bool      `json:"from_config"`
	CreatedBy   *string   `json:"created_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type patternConfigFile struct {
	Patterns []struct {
		Name        string `yaml:"name"`
		Label       string `yaml:"label"`
		Description string `yaml:"description"`
		Parameter   string `yaml:"parameter"`
		HTML        string `yaml:"html"`
		CSS         string `yaml:"css"`
		JS          string `yaml:"js"`
		Scope       string `yaml:"scope"`
	} `yaml:"patterns"`
}

func LoadPatternsFromConfig(ctx context.Context, pool db.Pool, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var cfg patternConfigFile
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return err
	}
	for _, p := range cfg.Patterns {
		scope := p.Scope
		if scope == "" {
			scope = "global"
		}
		_, err := pool.Exec(ctx, `
			INSERT INTO markdown_patterns (name, label, description, parameter, html, css, js, scope, from_config)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, TRUE)
			ON CONFLICT (name, scope) DO UPDATE
			  SET label       = EXCLUDED.label,
			      description = EXCLUDED.description,
			      parameter   = EXCLUDED.parameter,
			      html        = EXCLUDED.html,
			      css         = EXCLUDED.css,
			      js          = EXCLUDED.js,
			      from_config = TRUE,
			      updated_at  = NOW()`,
			p.Name, p.Label, p.Description, p.Parameter, p.HTML, p.CSS, p.JS, scope)
		if err != nil {
			return err
		}
	}
	slog.Info("markdown patterns loaded from config", "path", path, "count", len(cfg.Patterns))
	return nil
}

func scanPattern(rows interface {
	Scan(...any) error
}) (MarkdownPattern, error) {
	var p MarkdownPattern
	err := rows.Scan(
		&p.ID, &p.Name, &p.Label, &p.Description, &p.Parameter,
		&p.HTML, &p.CSS, &p.JS, &p.Scope,
		&p.FromConfig, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt,
	)
	return p, err
}

const patternSelect = `
	SELECT id::text, name, label, description, parameter, html, css, js, scope,
	       from_config, created_by::text, created_at, updated_at
	FROM markdown_patterns`

// ListPatterns godoc
// @Summary   List global markdown patterns
// @Tags      Patterns
// @Security  BearerAuth
// @Produce   json
// @Success   200  {object}  map[string]interface{}
// @Router    /api/patterns [get]
func (s *State) ListPatterns(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Pool.Query(r.Context(),
		patternSelect+` WHERE scope = 'global' ORDER BY name`)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer rows.Close()

	var patterns []MarkdownPattern
	for rows.Next() {
		p, err := scanPattern(rows)
		if err != nil {
			s.Error(w, http.StatusInternalServerError, "Scan error")
			return
		}
		patterns = append(patterns, p)
	}
	if patterns == nil {
		patterns = []MarkdownPattern{}
	}
	s.JSON(w, http.StatusOK, map[string]any{"patterns": patterns})
}

// GetPattern godoc
// @Summary   Get a single markdown pattern
// @Tags      Patterns
// @Security  BearerAuth
// @Produce   json
// @Param     id  path  string  true  "Pattern ID"
// @Success   200  {object}  MarkdownPattern
// @Router    /api/patterns/{id} [get]
func (s *State) GetPattern(w http.ResponseWriter, r *http.Request) {
	id := param(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		s.Error(w, http.StatusBadRequest, "Invalid pattern ID")
		return
	}
	row := s.Pool.QueryRow(r.Context(), patternSelect+` WHERE id = $1`, id)
	p, err := scanPattern(row)
	if err != nil {
		s.Error(w, http.StatusNotFound, "Pattern not found")
		return
	}
	s.JSON(w, http.StatusOK, p)
}

// CreatePattern godoc
// @Summary   Create a markdown pattern (admin)
// @Tags      Patterns
// @Security  BearerAuth
// @Accept    json
// @Produce   json
// @Success   201  {object}  MarkdownPattern
// @Router    /api/patterns [post]
func (s *State) CreatePattern(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string `json:"name"`
		Label       string `json:"label"`
		Description string `json:"description"`
		Parameter   string `json:"parameter"`
		HTML        string `json:"html"`
		CSS         string `json:"css"`
		JS          string `json:"js"`
		Scope       string `json:"scope"`
	}
	if err := decode(r, &body); err != nil {
		s.Error(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if body.Name == "" || body.Label == "" || body.HTML == "" {
		s.Error(w, http.StatusBadRequest, "name, label and html are required")
		return
	}
	if body.Scope == "" {
		body.Scope = "global"
	}
	claims := s.claims(r)
	row := s.Pool.QueryRow(r.Context(), `
		INSERT INTO markdown_patterns (name, label, description, parameter, html, css, js, scope, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING `+patternReturning,
		body.Name, body.Label, body.Description, body.Parameter, body.HTML, body.CSS, body.JS,
		body.Scope, claims.Subject)
	p, err := scanPattern(row)
	if err != nil {
		s.Error(w, http.StatusConflict, "Pattern with this name already exists in this scope")
		return
	}
	s.JSON(w, http.StatusCreated, p)
}

// UpdatePattern godoc
// @Summary   Update a markdown pattern (admin)
// @Tags      Patterns
// @Security  BearerAuth
// @Accept    json
// @Produce   json
// @Param     id  path  string  true  "Pattern ID"
// @Success   200  {object}  MarkdownPattern
// @Router    /api/patterns/{id} [put]
func (s *State) UpdatePattern(w http.ResponseWriter, r *http.Request) {
	name := param(r, "name")
	if name == "" {
		s.Error(w, http.StatusBadRequest, "Missing pattern name")
		return
	}
	var body struct {
		Name        string `json:"name"`
		Label       string `json:"label"`
		Description string `json:"description"`
		Parameter   string `json:"parameter"`
		HTML        string `json:"html"`
		CSS         string `json:"css"`
		JS          string `json:"js"`
		Scope       string `json:"scope"`
	}
	if err := decode(r, &body); err != nil {
		s.Error(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if body.HTML == "" {
		s.Error(w, http.StatusBadRequest, "html is required")
		return
	}
	if body.Name == "" {
		body.Name = name
	}
	if body.Scope == "" {
		body.Scope = "global"
	}
	row := s.Pool.QueryRow(r.Context(), `
		UPDATE markdown_patterns
		SET name = $2, label = $3, description = $4, parameter = $5, html = $6, css = $7, js = $8,
		    scope = $9, from_config = FALSE, updated_at = NOW()
		WHERE name = $1
		RETURNING `+patternReturning,
		name, body.Name, body.Label, body.Description, body.Parameter, body.HTML, body.CSS, body.JS, body.Scope)
	p, err := scanPattern(row)
	if err != nil {
		s.Error(w, http.StatusNotFound, "Pattern not found")
		return
	}
	s.JSON(w, http.StatusOK, p)
}

// DeletePattern godoc
// @Summary   Delete a markdown pattern (admin)
// @Tags      Patterns
// @Security  BearerAuth
// @Param     id  path  string  true  "Pattern ID"
// @Success   204
// @Router    /api/patterns/{id} [delete]
func (s *State) DeletePattern(w http.ResponseWriter, r *http.Request) {
	name := param(r, "name")
	if name == "" {
		s.Error(w, http.StatusBadRequest, "Missing pattern name")
		return
	}
	tag, err := s.Pool.Exec(r.Context(),
		`DELETE FROM markdown_patterns WHERE name = $1`, name)
	if err != nil || tag.RowsAffected() == 0 {
		s.Error(w, http.StatusNotFound, "Pattern not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListCoursePatterns godoc
// @Summary   List patterns for a course (global + course-specific)
// @Tags      Patterns
// @Security  BearerAuth
// @Produce   json
// @Param     slug  path  string  true  "Course slug"
// @Success   200  {object}  map[string]interface{}
// @Router    /api/courses/{slug}/patterns [get]
func (s *State) ListCoursePatterns(w http.ResponseWriter, r *http.Request) {
	slug := param(r, "slug")
	rows, err := s.Pool.Query(r.Context(),
		patternSelect+` WHERE scope = 'global' OR scope = $1 ORDER BY scope, name`, slug)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer rows.Close()

	var patterns []MarkdownPattern
	for rows.Next() {
		p, err := scanPattern(rows)
		if err != nil {
			s.Error(w, http.StatusInternalServerError, "Scan error")
			return
		}
		patterns = append(patterns, p)
	}
	if patterns == nil {
		patterns = []MarkdownPattern{}
	}
	s.JSON(w, http.StatusOK, map[string]any{"patterns": patterns})
}

// CreateCoursePattern godoc
// @Summary   Create a course-specific markdown pattern (admin)
// @Tags      Patterns
// @Security  BearerAuth
// @Accept    json
// @Produce   json
// @Param     slug  path  string  true  "Course slug"
// @Success   201  {object}  MarkdownPattern
// @Router    /api/courses/{slug}/patterns [post]
func (s *State) CreateCoursePattern(w http.ResponseWriter, r *http.Request) {
	slug := param(r, "slug")
	var body struct {
		Name        string `json:"name"`
		Label       string `json:"label"`
		Description string `json:"description"`
		Parameter   string `json:"parameter"`
		HTML        string `json:"html"`
		CSS         string `json:"css"`
		JS          string `json:"js"`
	}
	if err := decode(r, &body); err != nil {
		s.Error(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if body.Name == "" || body.Label == "" || body.HTML == "" {
		s.Error(w, http.StatusBadRequest, "name, label and html are required")
		return
	}
	claims := s.claims(r)
	row := s.Pool.QueryRow(r.Context(), `
		INSERT INTO markdown_patterns (name, label, description, parameter, html, css, js, scope, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING `+patternReturning,
		body.Name, body.Label, body.Description, body.Parameter, body.HTML, body.CSS, body.JS,
		slug, claims.Subject)
	p, err := scanPattern(row)
	if err != nil {
		s.Error(w, http.StatusConflict, "Pattern with this name already exists in this scope")
		return
	}
	s.JSON(w, http.StatusCreated, p)
}

// DeleteCoursePattern godoc
// @Summary   Delete a course-specific markdown pattern (admin)
// @Tags      Patterns
// @Security  BearerAuth
// @Param     slug  path  string  true  "Course slug"
// @Param     id    path  string  true  "Pattern ID"
// @Success   204
// @Router    /api/courses/{slug}/patterns/{id} [delete]
func (s *State) DeleteCoursePattern(w http.ResponseWriter, r *http.Request) {
	slug := param(r, "slug")
	id := param(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		s.Error(w, http.StatusBadRequest, "Invalid pattern ID")
		return
	}
	tag, err := s.Pool.Exec(r.Context(),
		`DELETE FROM markdown_patterns WHERE id = $1 AND scope = $2`, id, slug)
	if err != nil || tag.RowsAffected() == 0 {
		s.Error(w, http.StatusNotFound, "Pattern not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// patternReturning is the column list matching scanPattern order (without the SELECT prefix).
const patternReturning = `id::text, name, label, description, parameter, html, css, js, scope,
	from_config, created_by::text, created_at, updated_at`
