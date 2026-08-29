package handlers

import (
	"errors"
	"net/http"
	"testing"

	"github.com/genesary/pupitre/user-service/fake"
)

// TestMarkLessonComplete_AwardsXP marks a lesson complete and confirms
// lesson XP was recorded.
func TestMarkLessonComplete_AwardsXP(t *testing.T) {
	t.Parallel()

	repos := fake.NewRepositories()
	r := newTestRouterWithRepos(repos)

	rec := htDo(t, r, http.MethodPost, "/api/courses/linux-intro/lessons/intro/complete", "",
		htAuthHeader(t, "student"))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	total, err := repos.XP.Total(t.Context(), "user-uuid-1")
	if err != nil {
		t.Fatalf("XP.Total: %v", err)
	}

	if total == 0 {
		t.Error("expected lesson XP to have been awarded")
	}
}

// TestMarkLessonComplete_XPFailureStillSucceeds keeps a 200 when the XP
// award fails — XP is non-critical, so awardXP only logs its error.
func TestMarkLessonComplete_XPFailureStillSucceeds(t *testing.T) {
	t.Parallel()

	repos := fake.NewRepositories()
	xp := fake.NewXPRepository()
	xp.Err = errors.New("xp table locked")
	repos.XP = xp
	r := newTestRouterWithRepos(repos)

	rec := htDo(t, r, http.MethodPost, "/api/courses/linux-intro/lessons/intro/complete", "",
		htAuthHeader(t, "student"))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200 despite XP failure, got %d", rec.Code)
	}
}
