package handlers

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	"github.com/elearning/course-service/internal/content"
)

type moduleResponse struct {
	Index   int    `json:"index"`
	Name    string `json:"name"`
	Slug    string `json:"slug"`
	Type    string `json:"type"`
	Content string `json:"content,omitempty"`
	Viewed  bool   `json:"viewed"`
	Hidden  bool   `json:"hidden"`
}

func (s *State) viewedLessons(r *http.Request, courseSlug, userID string) map[string]bool {
	u, err := url.Parse(s.Config.UserServiceURL + "/internal/progress/viewed")
	if err != nil {
		return nil
	}
	q := u.Query()
	q.Set("user_id", userID)
	q.Set("course_slug", courseSlug)
	u.RawQuery = q.Encode()

	resp, err := http.Get(u.String())
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	var result struct {
		Viewed []string `json:"viewed"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil
	}
	m := make(map[string]bool, len(result.Viewed))
	for _, slug := range result.Viewed {
		m[slug] = true
	}
	return m
}

// GET /api/courses/{slug}/modules
func (s *State) ListModules(w http.ResponseWriter, r *http.Request) {
	courseSlug := param(r, "slug")
	claims := s.claims(r)

	c := s.Content.Get(courseSlug)
	if c == nil {
		s.Error(w, http.StatusNotFound, "Course not found")
		return
	}
	if claims.Role != "admin" && !c.IsPublished {
		s.Error(w, http.StatusForbidden, "Course not available")
		return
	}

	modules := s.visibleModules(c, r)
	viewed := s.viewedLessons(r, courseSlug, claims.Subject)
	out := make([]moduleResponse, 0, len(modules))
	for i, m := range modules {
		out = append(out, moduleResponse{
			Index:  i,
			Name:   m.Name,
			Slug:   m.Slug(),
			Type:   m.Type,
			Viewed: viewed[m.Slug()],
			Hidden: m.Hidden,
		})
	}
	s.JSON(w, http.StatusOK, map[string]any{"modules": out})
}

// GET /api/courses/{slug}/modules/{index}
func (s *State) GetModule(w http.ResponseWriter, r *http.Request) {
	courseSlug := param(r, "slug")
	indexStr := param(r, "index")
	claims := s.claims(r)

	c := s.Content.Get(courseSlug)
	if c == nil {
		s.Error(w, http.StatusNotFound, "Course not found")
		return
	}
	if claims.Role != "admin" && !c.IsPublished {
		s.Error(w, http.StatusForbidden, "Course not available")
		return
	}

	modules := s.visibleModules(c, r)

	idx, err := strconv.Atoi(indexStr)
	if err != nil || idx < 0 || idx >= len(modules) {
		s.Error(w, http.StatusNotFound, "Module not found")
		return
	}

	m := modules[idx]
	resp := moduleResponse{
		Index:  idx,
		Name:   m.Name,
		Slug:   m.Slug(),
		Type:   m.Type,
		Hidden: m.Hidden,
	}

	viewed := s.viewedLessons(r, courseSlug, claims.Subject)
	resp.Viewed = viewed[m.Slug()]

	switch m.Type {
	case "video", "image":
		resp.Content = content.ReplicatedPath(m, s.Config.UploadsDir)
	case "text":
		if m.HasGitContent() {
			data, err := content.FetchModuleContent(m.Src, m.Ref, m.Path, s.tokenForRepo(m.Src))
			if err != nil {
				s.Error(w, http.StatusInternalServerError, "Failed to fetch module content")
				return
			}
			resp.Content = string(data)
		} else {
			resp.Content = m.Content()
		}
	}

	s.JSON(w, http.StatusOK, resp)
}
