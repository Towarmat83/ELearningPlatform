//go:build e2e

package e2e_test

import (
	"net/http"
	"testing"
)

// TestCheckerServiceHealth checks the checker-service health endpoint.
func TestCheckerServiceHealth(t *testing.T) {
	skipIfNoCheckerService(t)

	resp := doJSON(t, http.MethodGet, globalCfg.CheckerURL+"/health", nil, "")
	mustStatus(t, resp, http.StatusOK)

	var out struct {
		Status string `json:"status"`
	}

	decodeBody(t, resp, &out)

	if out.Status != "ok" {
		t.Fatalf("health: expected status ok, got %q", out.Status)
	}
}

// TestEvaluateMissingFields verifies that /evaluate rejects empty payloads.
func TestEvaluateMissingFields(t *testing.T) {
	skipIfNoCheckerService(t)

	cases := []struct {
		name string
		body any
	}{
		{"empty_object", map[string]string{}},
		{"missing_policy", map[string]string{"projectId": "123"}},
		{"missing_project_id", map[string]string{"policy": "package p\ndefault allow = false"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := doJSON(t, http.MethodPost, globalCfg.CheckerURL+"/evaluate", tc.body, "")
			if resp.StatusCode < 400 {
				t.Fatalf("evaluate %s: expected 4xx error, got %d", tc.name, resp.StatusCode)
			}

			resp.Body.Close()
		})
	}
}

// TestCheckStepMissingFields verifies that /check-step rejects empty payloads.
func TestCheckStepMissingFields(t *testing.T) {
	skipIfNoCheckerService(t)

	resp := doJSON(t, http.MethodPost, globalCfg.CheckerURL+"/check-step", map[string]string{}, "")
	if resp.StatusCode < 400 {
		t.Fatalf("check-step: expected 4xx error, got %d", resp.StatusCode)
	}

	resp.Body.Close()
}

// TestEvaluateInvalidJSON verifies that /evaluate rejects malformed JSON.
func TestEvaluateInvalidJSON(t *testing.T) {
	skipIfNoCheckerService(t)

	resp := doJSON(t, http.MethodPost, globalCfg.CheckerURL+"/evaluate", "not-a-json-object", "")
	if resp.StatusCode < 400 {
		t.Fatalf("evaluate: expected 4xx for invalid JSON, got %d", resp.StatusCode)
	}

	resp.Body.Close()
}
