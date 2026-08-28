package handlers

import (
	"net/http"

	"go.uber.org/zap"
)

// skillModuleEntry is a single module entry in a skill drill-down response.
type skillModuleEntry struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Index       int    `json:"index"`
	Type        string `json:"type"`
	CourseSlug  string `json:"courseSlug"`
	CourseTitle string `json:"courseTitle"`
}

// ListSkillModules godoc
// @Summary  List all modules tagged with a given skill
// @Tags     Skills
// @Produce  json
// @Param    slug  path  string  true  "Skill slug"
// @Success  200   {object}  map[string]interface{}
// @Router   /api/skills/{slug}/modules [get].
func (s *State) ListSkillModules(writer http.ResponseWriter, req *http.Request) {
	skill := param(req, "slug")

	// One indexed query over course_modules replaces scanning every module
	// of every course in the catalog.
	found, err := s.Repos.Courses.ModulesBySkill(req.Context(), skill)
	if err != nil {
		zap.L().Error("list skill modules failed", zap.String("skill", skill), zap.Error(err))
		s.Error(writer, http.StatusInternalServerError, internalErrorMessage)

		return
	}

	modules := make([]skillModuleEntry, 0, len(found))
	for _, mod := range found {
		modules = append(modules, skillModuleEntry{
			Name:        mod.Name,
			Slug:        mod.Slug,
			Index:       mod.Index,
			Type:        mod.Type,
			CourseSlug:  mod.CourseSlug,
			CourseTitle: mod.CourseTitle,
		})
	}

	s.JSON(writer, http.StatusOK, map[string]any{"skill": skill, modulesTypeValue: modules})
}
