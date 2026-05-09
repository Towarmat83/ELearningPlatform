package handlers

import "net/http"

func (s *State) Health(w http.ResponseWriter, r *http.Request) {
	s.JSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"service": "course-service",
		"version": "1.0.0",
	})
}
