package handlers

import (
	"net/http"

	"github.com/genesary/pupitre/user-service/internal/repository"
)

// viewedLessons returns the set of lesson slugs the user has completed in a
// course, keyed by lesson slug with value true.
func viewedLessons(s *State, r *http.Request, courseSlug, userID string) map[string]bool {
	slugs, err := s.Repos.LessonProgress.ViewedSlugs(r.Context(), userID, courseSlug)
	if err != nil {
		return nil
	}

	lessonsViewed := make(map[string]bool)
	for _, slug := range slugs {
		lessonsViewed[slug] = true
	}

	return lessonsViewed
}

// MarkLessonComplete godoc
// @Summary   Mark a lesson as complete
// @Tags      Progress
// @Security  BearerAuth
// @Produce   json
// @Param     slug         path  string  true  "Course slug"
// @Param     lessonSlug  path  string  true  "Lesson slug"
// @Success   200   {object}  map[string]string
// @Router    /api/courses/{slug}/lessons/{lessonSlug}/complete [post].
func (s *State) MarkLessonComplete(writer http.ResponseWriter, req *http.Request) {
	courseSlug := param(req, "slug")
	lessonSlug := param(req, "lessonSlug")
	claims := s.claims(req)

	err := s.Repos.LessonProgress.MarkComplete(req.Context(), claims.Subject, courseSlug, lessonSlug)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	s.awardXP(req, claims.Subject, repository.XPSourceLesson, courseSlug+"/"+lessonSlug, repository.XPAmountLesson)

	s.JSON(writer, http.StatusOK, map[string]string{"message": "Lesson marked as complete"})
}
