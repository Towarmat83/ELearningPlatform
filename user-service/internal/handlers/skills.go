package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"go.uber.org/zap"
)

const (
	// skillModuleTypeQuiz identifies an assessable quiz module type.
	skillModuleTypeQuiz = "quiz"
	// skillModuleTypeLab identifies an assessable lab module type.
	skillModuleTypeLab = "lab"
)

// skillModuleEntry is a module descriptor returned by course-service.
type skillModuleEntry struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Index       int    `json:"index"`
	Type        string `json:"type"`
	CourseSlug  string `json:"courseSlug"`
	CourseTitle string `json:"courseTitle"`
}

// skillModuleStatus extends skillModuleEntry with the user's completion status.
type skillModuleStatus struct {
	skillModuleEntry

	Status string `json:"status"` // "completed" | "available"
}

// fetchSkillModules calls course-service to get all modules tagged with skill.
func (s *State) fetchSkillModules(req *http.Request, skill string) ([]skillModuleEntry, error) {
	if !slugRE.MatchString(skill) {
		return nil, fmt.Errorf("invalid skill slug: %q", skill)
	}

	rawURL := fmt.Sprintf("%s/api/skills/%s/modules", s.Config.CourseServiceURL, skill)

	//nolint:gosec // skill is validated against slugRE; CourseServiceURL is trusted server config
	r, err := http.NewRequestWithContext(req.Context(), http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("build skill modules request: %w", err)
	}

	resp, err := http.DefaultClient.Do(r) //nolint:gosec // URL built from validated slug and trusted CourseServiceURL
	if err != nil {
		return nil, fmt.Errorf("fetch skill modules: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)

		return nil, fmt.Errorf("course-service returned %d for skill %s: %s", resp.StatusCode, skill, body)
	}

	var body struct {
		Modules []skillModuleEntry `json:"modules"`
	}

	err = json.NewDecoder(resp.Body).Decode(&body)
	if err != nil {
		return nil, fmt.Errorf("decode skill modules: %w", err)
	}

	return body.Modules, nil
}

// querySlugPairs executes a two-column (courseSlug, slug) query and returns the
// results as a "courseSlug/slug" → true map. The query must accept a single
// $1::uuid parameter for the user ID.
func (s *State) querySlugPairs(req *http.Request, userID, logKey, query string) map[string]bool {
	rows, err := s.Pool.Query(req.Context(), query, userID)
	if err != nil {
		zap.L().Error("failed to query "+logKey, zap.String("userID", userID), zap.Error(err))

		return nil
	}

	defer rows.Close()

	out := make(map[string]bool)

	for rows.Next() {
		var cs, sl string
		if rows.Scan(&cs, &sl) == nil {
			out[cs+"/"+sl] = true
		}
	}

	return out
}

// passedModulesCtx returns the set of "courseSlug/moduleSlug" for all
// quiz and lab modules the user has passed.
func (s *State) passedModulesCtx(req *http.Request, userID string) map[string]bool {
	return s.querySlugPairs(req, userID, "module progress",
		`SELECT courseSlug, moduleSlug FROM module_progress
		 WHERE userId = $1::uuid AND passed = true AND moduleSlug IS NOT NULL`)
}

// viewedLessonsCtx returns the set of "courseSlug/lessonSlug" for all
// non-quiz/lab modules the user has viewed.
func (s *State) viewedLessonsCtx(req *http.Request, userID string) map[string]bool {
	return s.querySlugPairs(req, userID, "lesson progress",
		`SELECT courseSlug, lessonSlug FROM lesson_progress
		 WHERE userId = $1::uuid AND lessonSlug IS NOT NULL AND lessonSlug != '__complete__'`)
}

// skillIsCompleted returns true when all assessable (quiz/lab) modules in the
// list have been completed by the user. Skills with no assessable modules are
// never considered completed. Quizzes: passed in module_progress; labs: viewed.
func skillIsCompleted(modules []skillModuleEntry, passed, viewed map[string]bool) bool {
	hasAssessable := false

	for _, mod := range modules {
		if mod.Type != skillModuleTypeQuiz && mod.Type != skillModuleTypeLab {
			continue
		}

		hasAssessable = true

		key := mod.CourseSlug + "/" + mod.Slug

		if mod.Type == skillModuleTypeQuiz {
			if !passed[key] {
				return false
			}
		} else {
			if !viewed[key] {
				return false
			}
		}
	}

	return hasAssessable
}

// MySkillModules godoc
// @Summary  List modules teaching a skill with the user's completion status
// @Tags     Skills
// @Security BearerAuth
// @Produce  json
// @Param    slug  path  string  true  "Skill slug"
// @Success  200   {object}  map[string]interface{}
// @Router   /api/my/skills/{slug} [get].
func (s *State) MySkillModules(writer http.ResponseWriter, request *http.Request) {
	claims := s.claims(request)
	skill := param(request, "slug")

	modules, err := s.fetchSkillModules(request, skill)
	if err != nil {
		zap.L().Warn("failed to fetch skill modules", zap.String("skill", skill), zap.Error(err))
		s.Error(writer, http.StatusBadGateway, "failed to fetch skill modules")

		return
	}

	passed := s.passedModulesCtx(request, claims.Subject)
	viewed := s.viewedLessonsCtx(request, claims.Subject)

	result := make([]skillModuleStatus, 0, len(modules))
	prevCompleted := true // first module has no prerequisite

	for _, mod := range modules {
		key := mod.CourseSlug + "/" + mod.Slug

		var isDone bool
		if mod.Type == skillModuleTypeQuiz {
			isDone = passed[key]
		} else {
			isDone = viewed[key] // labs and text/video: viewed in lesson_progress
		}

		var status string

		switch {
		case isDone:
			status = pathStatusCompleted
		case prevCompleted:
			status = pathStatusAvailable
		default:
			status = pathStatusLocked
		}

		prevCompleted = isDone

		result = append(result, skillModuleStatus{skillModuleEntry: mod, Status: status})
	}

	s.JSON(writer, http.StatusOK, map[string]any{pathKindSkill: skill, "modules": result})
}
