//go:build e2e

// Package e2e_test contains end-to-end tests for the pupitre platform.
// Run with:
//
//	go test -tags e2e ./...
//
// Required env vars:
//
//	USER_SERVICE_URL    (default: http://localhost:8081)
//	CHECKER_SERVICE_URL (default: http://localhost:8083)
//	ADMIN_EMAIL         (default: admin@pupitre.local)
//	ADMIN_PASSWORD      (required — no default)
//
// Optional:
//
//	COURSE_SERVICE_URL  (course tests are skipped when unset)
package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
)

// global config populated by TestMain, readable by all test files.
var globalCfg testCfg

// TestMain waits for all configured services to be ready, performs the
// admin login to obtain a shared token, then runs the test suite.
func TestMain(m *testing.M) {
	globalCfg = loadConfig()

	if err := awaitServices(); err != nil {
		fmt.Fprintln(os.Stderr, "e2e: service readiness check failed:", err)
		os.Exit(1)
	}

	if globalCfg.UserURL != "" {
		tok, err := adminLogin(globalCfg)
		if err != nil {
			fmt.Fprintln(os.Stderr, "e2e: admin login failed:", err)
			os.Exit(1)
		}

		globalCfg.adminToken = tok
	}

	os.Exit(m.Run())
}

// awaitServices waits up to 30 s for each configured service health endpoint.
func awaitServices() error {
	const timeout = 30 * time.Second

	for _, entry := range []struct{ name, url string }{
		{"user-service", globalCfg.UserURL + "/health"},
		{"course-service", globalCfg.CourseURL + "/health"},
		{"checker-service", globalCfg.CheckerURL + "/health"},
	} {
		if entry.url == "/health" {
			continue // URL was empty; service not configured
		}

		if err := waitReady(entry.url, timeout); err != nil {
			return fmt.Errorf("%s: %w", entry.name, err)
		}
	}

	return nil
}

// adminLogin posts credentials to user-service and returns the JWT.
func adminLogin(cfg testCfg) (string, error) {
	body := map[string]string{
		"email":    cfg.AdminEmail,
		"password": cfg.AdminPass,
	}

	b, _ := json.Marshal(body)

	resp, err := http.Post( //nolint:gosec,noctx // test helper — URLs come from env, no context needed
		cfg.UserURL+"/api/auth/login",
		"application/json",
		bytes.NewReader(b),
	)
	if err != nil {
		return "", fmt.Errorf("login request: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("login returned %d", resp.StatusCode)
	}

	var out struct {
		Token string `json:"token"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode login response: %w", err)
	}

	if out.Token == "" {
		return "", fmt.Errorf("login response contained no token")
	}

	return out.Token, nil
}

// skipIfNoUserService skips the calling test when user-service is not configured.
func skipIfNoUserService(t *testing.T) {
	t.Helper()

	if globalCfg.UserURL == "" {
		t.Skip("USER_SERVICE_URL not set")
	}
}

// skipIfNoCourseService skips the calling test when course-service is not configured.
func skipIfNoCourseService(t *testing.T) {
	t.Helper()

	if globalCfg.CourseURL == "" {
		t.Skip("COURSE_SERVICE_URL not set — course-service tests require a running K8s cluster")
	}
}

// skipIfNoCheckerService skips the calling test when checker-service is not configured.
func skipIfNoCheckerService(t *testing.T) {
	t.Helper()

	if globalCfg.CheckerURL == "" {
		t.Skip("CHECKER_SERVICE_URL not set")
	}
}
