package handlers

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"go.uber.org/zap"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"

	"github.com/elearning/user-service/internal/db"
)

// MarkdownPattern is a reusable markdown rendering rule (global or
// course-scoped) with an HTML/CSS/JS implementation.
type MarkdownPattern struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Label       string  `json:"label"`
	Description string  `json:"description"`
	Parameter   string  `json:"parameter"`
	HTML        string  `json:"html"`
	CSS         string  `json:"css"`
	JS          string  `json:"js"`
	Scope       string  `json:"scope"`
	FromConfig  bool    `json:"fromConfig"`
	CreatedBy   *string `json:"createdBy,omitempty"`
	// CreatedAt is when the pattern was first created.
	CreatedAt time.Time `json:"createdAt"`
	// UpdatedAt is when the pattern was last modified.
	UpdatedAt time.Time `json:"updatedAt"`
}

// patternConfigFile is the on-disk YAML shape used to seed markdown patterns
// at startup.
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

// patternsGlobalScope is the scope value shared by patterns visible to
// every course, as opposed to a single course-specific scope.
const patternsGlobalScope = "global"

// patternsRespKeyPatterns is the JSON response key holding a pattern list.
const patternsRespKeyPatterns = "patterns"

// LoadPatternsFromConfig reads patterns from the YAML file at path (a
// trusted, operator-supplied startup configuration path, not user input)
// and upserts them into the database.
func LoadPatternsFromConfig(ctx context.Context, pool db.Pool, path string) error {
	data, err := os.ReadFile(path) //nolint:gosec // path is operator-supplied startup config, not user input
	if err != nil {
		return fmt.Errorf("reading pattern config file %q: %w", path, err)
	}

	var cfg patternConfigFile

	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		return fmt.Errorf("parsing pattern config file %q: %w", path, err)
	}

	for _, entry := range cfg.Patterns {
		scope := entry.Scope
		if scope == "" {
			scope = patternsGlobalScope
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
			      updatedAt  = NOW()`,
			entry.Name, entry.Label, entry.Description, entry.Parameter, entry.HTML, entry.CSS, entry.JS, scope)
		if err != nil {
			return fmt.Errorf("upserting pattern %q: %w", entry.Name, err)
		}
	}

	zap.L().Info("markdown patterns loaded from config", zap.String("path", path), zap.Int("count", len(cfg.Patterns)))

	return nil
}

// scanPattern scans a single row (from Query or QueryRow) into a
// MarkdownPattern using the column order of patternSelect / patternReturning.
func scanPattern(rows interface {
	Scan(dest ...any) error
}) (MarkdownPattern, error) {
	var pattern MarkdownPattern

	err := rows.Scan(
		&pattern.ID, &pattern.Name, &pattern.Label, &pattern.Description, &pattern.Parameter,
		&pattern.HTML, &pattern.CSS, &pattern.JS, &pattern.Scope,
		&pattern.FromConfig, &pattern.CreatedBy, &pattern.CreatedAt, &pattern.UpdatedAt,
	)
	if err != nil {
		return pattern, fmt.Errorf("scanning pattern row: %w", err)
	}

	return pattern, nil
}

// patternSelect is the base SELECT statement matching scanPattern's column
// order; callers append a WHERE/ORDER BY clause.
const patternSelect = `
	SELECT id::text, name, label, description, parameter, html, css, js, scope,
	       from_config, createdBy::text, createdAt, updatedAt
	FROM markdown_patterns`

// ListPatterns godoc
// @Summary   List global markdown patterns
// @Tags      Patterns
// @Security  BearerAuth
// @Produce   json
// @Success   200  {object}  map[string]interface{}
// @Router    /api/patterns [get].
func (s *State) ListPatterns(writer http.ResponseWriter, req *http.Request) {
	rows, err := s.Pool.Query(req.Context(),
		patternSelect+` WHERE scope = 'global' ORDER BY name`)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}
	defer rows.Close()

	var patterns []MarkdownPattern

	for rows.Next() {
		pattern, err := scanPattern(rows)
		if err != nil {
			s.Error(writer, http.StatusInternalServerError, "Scan error")

			return
		}

		patterns = append(patterns, pattern)
	}

	if patterns == nil {
		patterns = []MarkdownPattern{}
	}

	s.JSON(writer, http.StatusOK, map[string]any{patternsRespKeyPatterns: patterns})
}

// GetPattern godoc
// @Summary   Get a single markdown pattern
// @Tags      Patterns
// @Security  BearerAuth
// @Produce   json
// @Param     id  path  string  true  "Pattern ID"
// @Success   200  {object}  MarkdownPattern
// @Router    /api/patterns/{id} [get].
func (s *State) GetPattern(writer http.ResponseWriter, req *http.Request) {
	patternID := param(req, "id")

	_, err := uuid.Parse(patternID)
	if err != nil {
		s.Error(writer, http.StatusBadRequest, "Invalid pattern ID")

		return
	}

	row := s.Pool.QueryRow(req.Context(), patternSelect+` WHERE id = $1`, patternID)

	pattern, err := scanPattern(row)
	if err != nil {
		s.Error(writer, http.StatusNotFound, "Pattern not found")

		return
	}

	s.JSON(writer, http.StatusOK, pattern)
}

// CreatePattern godoc
// @Summary   Create a markdown pattern (admin)
// @Tags      Patterns
// @Security  BearerAuth
// @Accept    json
// @Produce   json
// @Success   201  {object}  MarkdownPattern
// @Router    /api/patterns [post].
func (s *State) CreatePattern(writer http.ResponseWriter, req *http.Request) {
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

	err := decode(req, &body)
	if err != nil {
		s.Error(writer, http.StatusBadRequest, "Invalid JSON")

		return
	}

	if body.Name == "" || body.Label == "" || body.HTML == "" {
		s.Error(writer, http.StatusBadRequest, "name, label and html are required")

		return
	}

	if body.Scope == "" {
		body.Scope = patternsGlobalScope
	}

	claims := s.claims(req)
	row := s.Pool.QueryRow(req.Context(), `
		INSERT INTO markdown_patterns (name, label, description, parameter, html, css, js, scope, createdBy)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING `+patternReturning,
		body.Name, body.Label, body.Description, body.Parameter, body.HTML, body.CSS, body.JS,
		body.Scope, claims.Subject)

	pattern, err := scanPattern(row)
	if err != nil {
		s.Error(writer, http.StatusConflict, "Pattern with this name already exists in this scope")

		return
	}

	s.JSON(writer, http.StatusCreated, pattern)
}

// UpdatePattern godoc
// @Summary   Update a markdown pattern (admin)
// @Tags      Patterns
// @Security  BearerAuth
// @Accept    json
// @Produce   json
// @Param     id  path  string  true  "Pattern ID"
// @Success   200  {object}  MarkdownPattern
// @Router    /api/patterns/{id} [put].
func (s *State) UpdatePattern(writer http.ResponseWriter, req *http.Request) {
	name := param(req, "name")
	if name == "" {
		s.Error(writer, http.StatusBadRequest, "Missing pattern name")

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

	err := decode(req, &body)
	if err != nil {
		s.Error(writer, http.StatusBadRequest, "Invalid JSON")

		return
	}

	if body.HTML == "" {
		s.Error(writer, http.StatusBadRequest, "html is required")

		return
	}

	if body.Name == "" {
		body.Name = name
	}

	if body.Scope == "" {
		body.Scope = patternsGlobalScope
	}

	row := s.Pool.QueryRow(req.Context(), `
		UPDATE markdown_patterns
		SET name = $2, label = $3, description = $4, parameter = $5, html = $6, css = $7, js = $8,
		    scope = $9, from_config = FALSE, updatedAt = NOW()
		WHERE name = $1
		RETURNING `+patternReturning,
		name, body.Name, body.Label, body.Description, body.Parameter, body.HTML, body.CSS, body.JS, body.Scope)

	pattern, err := scanPattern(row)
	if err != nil {
		s.Error(writer, http.StatusNotFound, "Pattern not found")

		return
	}

	s.JSON(writer, http.StatusOK, pattern)
}

// DeletePattern godoc
// @Summary   Delete a markdown pattern (admin)
// @Tags      Patterns
// @Security  BearerAuth
// @Param     id  path  string  true  "Pattern ID"
// @Success   204
// @Router    /api/patterns/{id} [delete].
func (s *State) DeletePattern(writer http.ResponseWriter, req *http.Request) {
	name := param(req, "name")
	if name == "" {
		s.Error(writer, http.StatusBadRequest, "Missing pattern name")

		return
	}

	tag, err := s.Pool.Exec(req.Context(),
		`DELETE FROM markdown_patterns WHERE name = $1`, name)
	if err != nil || tag.RowsAffected() == 0 {
		s.Error(writer, http.StatusNotFound, "Pattern not found")

		return
	}

	writer.WriteHeader(http.StatusNoContent)
}

// ListCoursePatterns godoc
// @Summary   List patterns for a course (global + course-specific)
// @Tags      Patterns
// @Security  BearerAuth
// @Produce   json
// @Param     slug  path  string  true  "Course slug"
// @Success   200  {object}  map[string]interface{}
// @Router    /api/courses/{slug}/patterns [get].
func (s *State) ListCoursePatterns(writer http.ResponseWriter, req *http.Request) {
	slug := param(req, "slug")

	rows, err := s.Pool.Query(req.Context(),
		patternSelect+` WHERE scope = 'global' OR scope = $1 ORDER BY scope, name`, slug)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}
	defer rows.Close()

	var patterns []MarkdownPattern

	for rows.Next() {
		pattern, err := scanPattern(rows)
		if err != nil {
			s.Error(writer, http.StatusInternalServerError, "Scan error")

			return
		}

		patterns = append(patterns, pattern)
	}

	if patterns == nil {
		patterns = []MarkdownPattern{}
	}

	s.JSON(writer, http.StatusOK, map[string]any{patternsRespKeyPatterns: patterns})
}

// CreateCoursePattern godoc
// @Summary   Create a course-specific markdown pattern (admin)
// @Tags      Patterns
// @Security  BearerAuth
// @Accept    json
// @Produce   json
// @Param     slug  path  string  true  "Course slug"
// @Success   201  {object}  MarkdownPattern
// @Router    /api/courses/{slug}/patterns [post].
func (s *State) CreateCoursePattern(writer http.ResponseWriter, req *http.Request) {
	slug := param(req, "slug")

	var body struct {
		Name        string `json:"name"`
		Label       string `json:"label"`
		Description string `json:"description"`
		Parameter   string `json:"parameter"`
		HTML        string `json:"html"`
		CSS         string `json:"css"`
		JS          string `json:"js"`
	}

	err := decode(req, &body)
	if err != nil {
		s.Error(writer, http.StatusBadRequest, "Invalid JSON")

		return
	}

	if body.Name == "" || body.Label == "" || body.HTML == "" {
		s.Error(writer, http.StatusBadRequest, "name, label and html required")

		return
	}

	claims := s.claims(req)
	row := s.Pool.QueryRow(req.Context(), `
		INSERT INTO markdown_patterns (name, label, description, parameter, html, css, js, scope, createdBy)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING `+patternReturning,
		body.Name, body.Label, body.Description, body.Parameter, body.HTML, body.CSS, body.JS,
		slug, claims.Subject)

	pattern, err := scanPattern(row)
	if err != nil {
		s.Error(writer, http.StatusConflict, "Pattern name already exists in scope")

		return
	}

	s.JSON(writer, http.StatusCreated, pattern)
}

// DeleteCoursePattern godoc
// @Summary   Delete a course-specific markdown pattern (admin)
// @Tags      Patterns
// @Security  BearerAuth
// @Param     slug  path  string  true  "Course slug"
// @Param     id    path  string  true  "Pattern ID"
// @Success   204
// @Router    /api/courses/{slug}/patterns/{id} [delete].
func (s *State) DeleteCoursePattern(writer http.ResponseWriter, req *http.Request) {
	slug := param(req, "slug")

	patternID := param(req, "id")

	_, err := uuid.Parse(patternID)
	if err != nil {
		s.Error(writer, http.StatusBadRequest, "Invalid pattern ID")

		return
	}

	tag, err := s.Pool.Exec(req.Context(),
		`DELETE FROM markdown_patterns WHERE id = $1 AND scope = $2`, patternID, slug)
	if err != nil || tag.RowsAffected() == 0 {
		s.Error(writer, http.StatusNotFound, "Pattern not found")

		return
	}

	writer.WriteHeader(http.StatusNoContent)
}

// patternReturning lists scanPattern's columns (no SELECT prefix).
const patternReturning = `id::text, name, label, description, parameter, html, css, js, scope,
	from_config, createdBy::text, createdAt, updatedAt`
