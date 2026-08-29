package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/genesary/pupitre/user-service/fake"
	"github.com/genesary/pupitre/user-service/internal/config"
	"github.com/genesary/pupitre/user-service/internal/models"
)

// TestMyPaths_SkillKind drives MyPaths for a skill-kind path, exercising
// buildSkillStatuses / fetchSkillModules / skillIsCompleted end to end
// against a mock course-service.
func TestMyPaths_SkillKind(t *testing.T) {
	t.Parallel()

	courseSvc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/api/paths/skills-path":
			_, _ = w.Write([]byte(`{"slug":"skills-path","title":"Skills Path","kind":"skill","skills":["linux","docker"]}`))
		case "/api/skills/linux/modules":
			_, _ = w.Write([]byte(`{"modules":[{"slug":"quiz-linux","type":"quiz","courseSlug":"linux-intro"}]}`))
		case "/api/skills/docker/modules":
			_, _ = w.Write([]byte(`{"modules":[]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer courseSvc.Close()

	userID := uuid.New()

	repos := fake.NewRepositories()
	repos.Paths = fake.NewPathEnrollmentRepository(models.PathEnrollment{
		UserID: userID, PathSlug: "skills-path", EnrolledAt: time.Now(),
	})
	// The linux quiz is passed → the "linux" skill is completed.
	slug := "quiz-linux"
	repos.ModuleProgress = fake.NewModuleProgressRepository(models.ModuleProgress{
		UserID: userID.String(), CourseSlug: "linux-intro", ModuleIndex: 0,
		ModuleSlug: &slug, Passed: true, BestScore: 10, MaxScore: 10,
	})

	cfg := &config.Config{
		JWTSecret: htSecret, JWTExpiryH: htExpiry, CORSOrigins: []string{"*"},
		InternalSecret: htInternalSecret, CourseServiceURL: courseSvc.URL,
	}
	s := &State{Repos: repos, Config: cfg}
	router := BuildRouter(s, cfg, false)

	rec := htDo(t, router, http.MethodGet, "/api/my/paths", "",
		htAuthHeaderForSubject(t, "student", userID.String()))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Paths []struct {
			Slug    string `json:"slug"`
			Kind    string `json:"kind"`
			Courses []struct {
				Slug   string `json:"slug"`
				Status string `json:"status"`
			} `json:"courses"`
		} `json:"paths"`
	}

	err := json.NewDecoder(rec.Body).Decode(&resp)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(resp.Paths) != 1 || resp.Paths[0].Kind != "skill" {
		t.Fatalf("unexpected paths: %+v", resp.Paths)
	}

	statuses := resp.Paths[0].Courses
	if len(statuses) != 2 {
		t.Fatalf("want 2 skill statuses, got %+v", statuses)
	}

	if statuses[0].Slug != "linux" || statuses[0].Status != "completed" {
		t.Errorf("linux status = %+v, want completed", statuses[0])
	}

	// docker has no assessable modules → not completed; since linux is done it
	// is the next available skill.
	if statuses[1].Slug != "docker" || statuses[1].Status != "available" {
		t.Errorf("docker status = %+v, want available", statuses[1])
	}
}

// TestMyPaths_SkillKind_ModuleFetchFails marks every skill "locked" when
// course-service cannot return its modules (buildSkillStatuses error branch).
func TestMyPaths_SkillKind_ModuleFetchFails(t *testing.T) {
	t.Parallel()

	courseSvc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/paths/skills-path" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"slug":"skills-path","title":"Skills","kind":"skill","skills":["linux","docker"]}`))

			return
		}

		w.WriteHeader(http.StatusInternalServerError) // every /api/skills/*/modules call fails
	}))
	defer courseSvc.Close()

	userID := uuid.New()
	repos := fake.NewRepositories()
	repos.Paths = fake.NewPathEnrollmentRepository(models.PathEnrollment{
		UserID: userID, PathSlug: "skills-path", EnrolledAt: time.Now(),
	})

	cfg := &config.Config{
		JWTSecret: htSecret, JWTExpiryH: htExpiry, CORSOrigins: []string{"*"},
		InternalSecret: htInternalSecret, CourseServiceURL: courseSvc.URL,
	}
	s := &State{Repos: repos, Config: cfg}
	router := BuildRouter(s, cfg, false)

	rec := htDo(t, router, http.MethodGet, "/api/my/paths", "",
		htAuthHeaderForSubject(t, "student", userID.String()))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Paths []struct {
			Courses []struct {
				Status string `json:"status"`
			} `json:"courses"`
		} `json:"paths"`
	}

	err := json.NewDecoder(rec.Body).Decode(&resp)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(resp.Paths) != 1 || len(resp.Paths[0].Courses) != 2 {
		t.Fatalf("unexpected skill statuses: %+v", resp.Paths)
	}

	for _, c := range resp.Paths[0].Courses {
		if c.Status != "locked" {
			t.Errorf("status = %q, want locked when modules cannot be fetched", c.Status)
		}
	}
}
