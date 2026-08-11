package handlers

import (
	"testing"

	"github.com/genesary/pupitre/course-service/internal/content"
)

// newTestStateWithCourseSkills returns a State seeded with courses that carry
// skill tags, so buildSkillTotals has something to count.
func newTestStateWithCourseSkills(t *testing.T) *State {
	t.Helper()

	mock := newUserServiceMock()
	t.Cleanup(mock.Close)

	s := newTestState(t, mock)

	s.Content.Put(&content.Course{
		Slug:       "go-basics",
		Title:      "Go Basics",
		Difficulty: "beginner",
		IsPublic:   true,
		Skills:     []string{"golang", "backend"},
	})
	s.Content.Put(&content.Course{
		Slug:       "go-advanced",
		Title:      "Go Advanced",
		Difficulty: "advanced",
		IsPublic:   true,
		Skills:     []string{"golang"},
	})
	s.Content.Put(&content.Course{
		Slug:       "docker-intro",
		Title:      "Docker Intro",
		Difficulty: "beginner",
		IsPublic:   true,
		Skills:     []string{"docker", "backend"},
	})

	return s
}

// TestBuildSkillTotals_NoMatch verifies zero is returned for a skill that
// no course in the catalog covers.
func TestBuildSkillTotals_NoMatch(t *testing.T) {
	t.Parallel()

	s := newTestStateWithCourseSkills(t)
	totals := s.buildSkillTotals(map[string]struct{}{"kubernetes": {}})

	if totals["kubernetes"] != 0 {
		t.Errorf("want 0 for unmatched skill, got %d", totals["kubernetes"])
	}
}

// TestBuildSkillTotals_SingleSkill verifies the count is 1 for a skill
// covered by exactly one course.
func TestBuildSkillTotals_SingleSkill(t *testing.T) {
	t.Parallel()

	s := newTestStateWithCourseSkills(t)
	totals := s.buildSkillTotals(map[string]struct{}{"docker": {}})

	if totals["docker"] != 1 {
		t.Errorf("want 1 course for docker, got %d", totals["docker"])
	}
}

// TestBuildSkillTotals_MultiSkill verifies each skill is counted independently
// when multiple skills are requested at once.
func TestBuildSkillTotals_MultiSkill(t *testing.T) {
	t.Parallel()

	s := newTestStateWithCourseSkills(t)
	totals := s.buildSkillTotals(map[string]struct{}{"golang": {}, "backend": {}})

	if totals["golang"] != 2 {
		t.Errorf("want 2 courses for golang, got %d", totals["golang"])
	}

	if totals["backend"] != 2 {
		t.Errorf("want 2 courses for backend, got %d", totals["backend"])
	}
}

// TestBuildSkillTotals_EmptyInput verifies an empty skill set returns an
// empty map without panicking.
func TestBuildSkillTotals_EmptyInput(t *testing.T) {
	t.Parallel()

	s := newTestStateWithCourseSkills(t)
	totals := s.buildSkillTotals(map[string]struct{}{})

	if len(totals) != 0 {
		t.Errorf("want empty totals, got %v", totals)
	}
}
