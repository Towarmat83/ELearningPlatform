package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/genesary/pupitre/internal/httpx"
)

// fetchCourseServiceJSON performs a GET on course-service at urlPath and
// decodes the JSON body into dest. The caller is responsible for validating
// any user-supplied segments before building urlPath.
func (s *State) fetchCourseServiceJSON(req *http.Request, urlPath string, dest any) error {
	rawURL := s.Config.CourseServiceURL + urlPath

	//nolint:gosec // rawURL built from trusted CourseServiceURL and pre-validated slug
	r, err := http.NewRequestWithContext(req.Context(), http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return fmt.Errorf("build request to %s: %w", urlPath, err)
	}

	resp, err := httpx.Do(r) //nolint:bodyclose // httpx.Drain closes it
	if err != nil {
		return fmt.Errorf("GET %s: %w", urlPath, err)
	}

	defer httpx.Drain(resp)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)

		return fmt.Errorf("course-service %s returned %d: %s", urlPath, resp.StatusCode, body)
	}

	err = json.NewDecoder(resp.Body).Decode(dest)
	if err != nil {
		return fmt.Errorf("decode %s: %w", urlPath, err)
	}

	return nil
}
