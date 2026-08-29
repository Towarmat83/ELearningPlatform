package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/genesary/pupitre/course-service/fake"
	"github.com/genesary/pupitre/course-service/internal/config"
	"github.com/genesary/pupitre/course-service/internal/content"
	"github.com/genesary/pupitre/course-service/internal/repository"
)

// prereqCourse returns a public course gated behind a min-score prerequisite.
func prereqCourse() *content.Course {
	return &content.Course{
		Slug: "advanced", Title: "Advanced", IsPublic: true,
		Prerequisites: []content.CoursePrerequisite{
			{Course: "intro", MinScore: 50, Modules: []string{"quiz-1"}},
		},
		Modules: []content.Module{{Name: "Deep Dive", Type: "text", InlineContent: "x"}},
	}
}

// prereqState wires a State whose user-service mock answers the
// course-summary probe per summaryHandler.
func prereqState(t *testing.T, summaryHandler http.HandlerFunc) *State {
	t.Helper()

	userSrv := newUserServiceMockWith(map[string]http.HandlerFunc{
		"/internal/progress/course-summary": summaryHandler,
	})
	t.Cleanup(userSrv.Close)

	cfg := &config.Config{JWTSecret: "test-secret", JWTExpiryH: 24, UserServiceURL: userSrv.URL}

	return NewState(cfg, &repository.Repositories{
		Courses:      fake.NewCourseRepository(prereqCourse()),
		Paths:        fake.NewPathRepository(),
		QuizAttempts: fake.NewQuizAttemptRepository(),
		LabChecks:    fake.NewLabCheckRepository(),
	})
}

// getModules issues an authenticated student module-list request.
func getModules(t *testing.T, s *State) *httptest.ResponseRecorder {
	t.Helper()

	r := BuildRouter(s, s.Config, false)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet,
		"/api/courses/advanced/modules", http.NoBody)
	req.Header.Set("Authorization", authHeader(t, "test-secret"))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	return rec
}

// TestPrereq_MetGrantsAccess lets a learner in when the prerequisite course
// summary shows a passing score and the required module passed.
func TestPrereq_MetGrantsAccess(t *testing.T) {
	t.Parallel()

	s := prereqState(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"totalScore":80,"passedModules":["quiz-1"],"viewedCount":3}`))
	})

	rec := getModules(t, s)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestPrereq_LowScoreDenies blocks access when the score is below MinScore.
func TestPrereq_LowScoreDenies(t *testing.T) {
	t.Parallel()

	s := prereqState(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"totalScore":10,"passedModules":["quiz-1"]}`))
	})

	rec := getModules(t, s)
	if rec.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", rec.Code)
	}
}

// TestPrereq_MissingModuleDenies blocks access when a required module has not
// been passed.
func TestPrereq_MissingModuleDenies(t *testing.T) {
	t.Parallel()

	s := prereqState(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"totalScore":90,"passedModules":[]}`))
	})

	rec := getModules(t, s)
	if rec.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", rec.Code)
	}
}

// TestPrereq_SummaryUnavailableDenies falls back to emptyCoursePrereqSummary
// when the user-service probe errors, which reads as "no progress".
func TestPrereq_SummaryUnavailableDenies(t *testing.T) {
	t.Parallel()

	s := prereqState(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	rec := getModules(t, s)
	if rec.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", rec.Code)
	}
}
