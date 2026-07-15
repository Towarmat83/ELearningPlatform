package handlers

import (
	"net/http"
	"slices"
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

	var modules []skillModuleEntry

	for _, course := range s.Content.List() {
		for idx, mod := range course.Modules {
			if slices.Contains(mod.Skills, skill) {
				modules = append(modules, skillModuleEntry{
					Name:        mod.Name,
					Slug:        mod.Slug(),
					Index:       idx,
					Type:        mod.Type,
					CourseSlug:  course.Slug,
					CourseTitle: course.Title,
				})
			}
		}
	}

	if modules == nil {
		modules = []skillModuleEntry{}
	}

	s.JSON(writer, http.StatusOK, map[string]any{"skill": skill, modulesTypeValue: modules})
}
