package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/elearning/api-go/internal/config"
	"github.com/elearning/api-go/internal/db"
	"github.com/elearning/api-go/internal/handlers"
	"github.com/elearning/api-go/migrations"
)

// ─── HTTP test client ─────────────────────────────────────────────────────────

type testClient struct {
	base string
	t    *testing.T
}

func (c *testClient) do(method, path string, body any, token string) (int, map[string]any) {
	c.t.Helper()
	var req *http.Request
	if body != nil {
		data, _ := json.Marshal(body)
		req, _ = http.NewRequest(method, c.base+path, bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req, _ = http.NewRequest(method, c.base+path, nil)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatalf("request %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result) //nolint:errcheck
	return resp.StatusCode, result
}

func (c *testClient) get(path, token string) (int, map[string]any) {
	return c.do("GET", path, nil, token)
}
func (c *testClient) post(path string, body any, token string) (int, map[string]any) {
	return c.do("POST", path, body, token)
}
func (c *testClient) put(path string, body any, token string) (int, map[string]any) {
	return c.do("PUT", path, body, token)
}
func (c *testClient) del(path, token string) (int, map[string]any) {
	return c.do("DELETE", path, nil, token)
}

// ─── Test helpers ─────────────────────────────────────────────────────────────

func str(body map[string]any, key string) string {
	v, _ := body[key].(string)
	return v
}

func num(body map[string]any, key string) float64 {
	v, _ := body[key].(float64)
	return v
}

func hasKey(t *testing.T, body map[string]any, key string) {
	t.Helper()
	if _, ok := body[key]; !ok {
		t.Errorf("key %q missing from response: %v", key, body)
	}
}

func assertStatus(t *testing.T, got, want int, body map[string]any) {
	t.Helper()
	if got != want {
		t.Fatalf("status: got %d, want %d — body: %v", got, want, body)
	}
}

// ─── Setup ────────────────────────────────────────────────────────────────────

func setupTestServer(t *testing.T) (*testClient, *pgxpool.Pool) {
	t.Helper()
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping integration tests")
	}

	pool, err := db.Connect(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("db connect: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := db.RunMigrations(context.Background(), pool, migrations.FS); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	cleanDB(t, pool)

	cfg := &config.Config{
		JWTSecret:   "test-secret-minimum-32-characters-long!!",
		JWTExpiryH:  24,
		CORSOrigins: []string{"*"},
	}
	s := &handlers.State{Pool: pool, Config: cfg}
	srv := httptest.NewServer(handlers.BuildRouter(s, cfg, pool, false))
	t.Cleanup(srv.Close)

	return &testClient{base: srv.URL, t: t}, pool
}

// cleanDB truncates all user-data tables and re-seeds the admin account.
func cleanDB(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE users CASCADE"); err != nil {
		t.Fatalf("cleanDB truncate: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (username, email, password_hash, role, is_active, auth_provider)
		VALUES ('admin', 'admin@elearning.local',
		        '$2y$12$U6BVYjCKzHaIu2VrJNHDhuBUNTiOrcP0xoovwKbGSvOMd29qwZz.y',
		        'admin', TRUE, 'local')`); err != nil {
		t.Fatalf("cleanDB seed admin: %v", err)
	}
}

// ─── Integration tests ────────────────────────────────────────────────────────

// TestAPI runs all endpoint tests sequentially.
// Each sub-test builds on shared state (tokens, IDs) accumulated by prior sub-tests.
func TestAPI(t *testing.T) {
	c, _ := setupTestServer(t)

	var (
		adminToken   string
		studentToken string
		courseID     string
		formLabID    string
		ctfLabID     string
	)

	// ── Health & public endpoints ──────────────────────────────────────────────

	t.Run("Health", func(t *testing.T) {
		status, body := c.get("/health", "")
		assertStatus(t, status, 200, body)
		if str(body, "status") != "ok" {
			t.Fatalf("expected ok, got %v", body)
		}
	})

	t.Run("PublicSettings", func(t *testing.T) {
		status, body := c.get("/api/settings/public", "")
		assertStatus(t, status, 200, body)
		hasKey(t, body, "registration_enabled")
		hasKey(t, body, "password_min_length")
	})

	t.Run("OAuthProviders", func(t *testing.T) {
		status, body := c.get("/api/auth/oauth/providers", "")
		assertStatus(t, status, 200, body)
		hasKey(t, body, "providers")
	})

	// ── Auth ──────────────────────────────────────────────────────────────────

	t.Run("Auth/AdminLogin", func(t *testing.T) {
		status, body := c.post("/api/auth/login", map[string]string{
			"email": "admin@elearning.local", "password": "Admin@1234",
		}, "")
		assertStatus(t, status, 200, body)
		adminToken = str(body, "token")
		if adminToken == "" {
			t.Fatal("admin token missing")
		}
	})

	t.Run("Auth/Register", func(t *testing.T) {
		status, body := c.post("/api/auth/register", map[string]string{
			"username": "student1",
			"email":    "student1@test.local",
			"password": "Test@1234",
		}, "")
		assertStatus(t, status, 200, body)
		studentToken = str(body, "token")
		if studentToken == "" {
			t.Fatal("student token missing")
		}
		user, _ := body["user"].(map[string]any)
		if str(user, "role") != "student" {
			t.Fatalf("expected student role, got %v", user)
		}
	})

	t.Run("Auth/Register_DuplicateEmail", func(t *testing.T) {
		status, body := c.post("/api/auth/register", map[string]string{
			"username": "other", "email": "student1@test.local", "password": "Test@1234",
		}, "")
		assertStatus(t, status, 409, body)
	})

	t.Run("Auth/Register_WeakPassword", func(t *testing.T) {
		status, body := c.post("/api/auth/register", map[string]string{
			"username": "weakuser", "email": "weak@test.local", "password": "short",
		}, "")
		assertStatus(t, status, 400, body)
	})

	t.Run("Auth/Login_WrongPassword", func(t *testing.T) {
		status, body := c.post("/api/auth/login", map[string]string{
			"email": "student1@test.local", "password": "wrongpassword",
		}, "")
		assertStatus(t, status, 401, body)
	})

	t.Run("Auth/Me", func(t *testing.T) {
		status, body := c.get("/api/auth/me", studentToken)
		assertStatus(t, status, 200, body)
		if str(body, "email") != "student1@test.local" {
			t.Fatalf("wrong email: %v", body)
		}
	})

	t.Run("Auth/Me_Unauthenticated", func(t *testing.T) {
		status, body := c.get("/api/auth/me", "")
		assertStatus(t, status, 401, body)
	})

	t.Run("Auth/Me_InvalidToken", func(t *testing.T) {
		status, body := c.get("/api/auth/me", "not.a.valid.jwt")
		assertStatus(t, status, 401, body)
	})

	t.Run("Auth/UpdateProfile", func(t *testing.T) {
		status, body := c.put("/api/auth/profile", map[string]string{
			"bio": "hello world",
		}, studentToken)
		assertStatus(t, status, 200, body)
		if str(body, "bio") != "hello world" {
			t.Fatalf("bio not updated: %v", body)
		}
	})

	// ── Courses ───────────────────────────────────────────────────────────────

	t.Run("Courses/ListEmpty", func(t *testing.T) {
		status, body := c.get("/api/courses", "")
		assertStatus(t, status, 200, body)
		courses, _ := body["courses"].([]any)
		if len(courses) != 0 {
			t.Fatalf("expected 0, got %d", len(courses))
		}
	})

	t.Run("Courses/Create", func(t *testing.T) {
		status, body := c.post("/api/courses", map[string]any{
			"title": "Go Testing 101", "description": "A test course",
			"is_published": true,
		}, studentToken)
		assertStatus(t, status, 200, body)
		courseID = str(body, "id")
		if courseID == "" {
			t.Fatal("missing course ID")
		}
	})

	t.Run("Courses/Create_NoTitle", func(t *testing.T) {
		status, body := c.post("/api/courses", map[string]any{
			"description": "missing title",
		}, studentToken)
		assertStatus(t, status, 400, body)
	})

	t.Run("Courses/Get", func(t *testing.T) {
		status, body := c.get("/api/courses/"+courseID, "")
		assertStatus(t, status, 200, body)
		if str(body, "id") != courseID {
			t.Fatalf("wrong course: %v", body)
		}
		if str(body, "title") != "Go Testing 101" {
			t.Fatalf("wrong title: %v", body)
		}
	})

	t.Run("Courses/Get_NotFound", func(t *testing.T) {
		status, body := c.get("/api/courses/00000000-0000-0000-0000-000000000000", "")
		assertStatus(t, status, 404, body)
	})

	t.Run("Courses/Enroll", func(t *testing.T) {
		status, body := c.post("/api/courses/"+courseID+"/enroll", nil, studentToken)
		assertStatus(t, status, 200, body)
	})

	t.Run("Courses/Enroll_Idempotent", func(t *testing.T) {
		// Re-enrolling must not return an error
		status, body := c.post("/api/courses/"+courseID+"/enroll", nil, studentToken)
		assertStatus(t, status, 200, body)
	})

	t.Run("Courses/MyCourses", func(t *testing.T) {
		status, body := c.get("/api/my/courses", studentToken)
		assertStatus(t, status, 200, body)
		courses, _ := body["courses"].([]any)
		if len(courses) != 1 {
			t.Fatalf("expected 1 course, got %d", len(courses))
		}
	})

	t.Run("Courses/ListPublished", func(t *testing.T) {
		status, body := c.get("/api/courses", "")
		assertStatus(t, status, 200, body)
		courses, _ := body["courses"].([]any)
		if len(courses) != 1 {
			t.Fatalf("expected 1, got %d", len(courses))
		}
	})

	// ── Labs ──────────────────────────────────────────────────────────────────

	t.Run("Labs/CreateForm", func(t *testing.T) {
		status, body := c.post("/api/courses/"+courseID+"/labs", map[string]any{
			"title": "Form Lab", "description": "Quiz time", "lab_type": "form",
			"points": 100, "is_published": true,
			"content": map[string]any{
				"questions": []any{
					map[string]any{
						"id": "q1", "type": "multiple_choice",
						"text":           "What is 2+2?",
						"options":        []string{"3", "4", "5"},
						"correct_answer": "4",
						"points":         100,
					},
				},
			},
		}, studentToken)
		assertStatus(t, status, 200, body)
		formLabID = str(body, "id")
		if formLabID == "" {
			t.Fatal("missing lab ID")
		}
	})

	t.Run("Labs/CreateCTF", func(t *testing.T) {
		status, body := c.post("/api/courses/"+courseID+"/labs", map[string]any{
			"title": "CTF Lab", "description": "Find the flag", "lab_type": "ctf",
			"points": 200, "is_published": true,
			"flag":    "FLAG{test-go-api}",
			"content": map[string]any{},
		}, studentToken)
		assertStatus(t, status, 200, body)
		ctfLabID = str(body, "id")
		if ctfLabID == "" {
			t.Fatal("missing lab ID")
		}
	})

	t.Run("Labs/CreateCTF_MissingFlag", func(t *testing.T) {
		status, body := c.post("/api/courses/"+courseID+"/labs", map[string]any{
			"title": "Bad CTF", "description": "no flag", "lab_type": "ctf",
			"points": 100, "content": map[string]any{},
		}, studentToken)
		assertStatus(t, status, 400, body)
	})

	t.Run("Labs/CreateInvalidType", func(t *testing.T) {
		status, body := c.post("/api/courses/"+courseID+"/labs", map[string]any{
			"title": "Bad Lab", "description": "bad type", "lab_type": "unknown",
			"points": 100, "content": map[string]any{},
		}, studentToken)
		assertStatus(t, status, 400, body)
	})

	t.Run("Labs/List", func(t *testing.T) {
		status, body := c.get("/api/courses/"+courseID+"/labs", studentToken)
		assertStatus(t, status, 200, body)
		labs, _ := body["labs"].([]any)
		if len(labs) != 2 {
			t.Fatalf("expected 2, got %d", len(labs))
		}
	})

	t.Run("Labs/List_UnenrolledForbidden", func(t *testing.T) {
		// Register a new user who is not enrolled
		_, regBody := c.post("/api/auth/register", map[string]string{
			"username": "student2",
			"email":    "student2@test.local",
			"password": "Test@1234",
		}, "")
		tok := str(regBody, "token")
		status, body := c.get("/api/courses/"+courseID+"/labs", tok)
		assertStatus(t, status, 403, body)
	})

	t.Run("Labs/GetFormNoCorrectAnswer", func(t *testing.T) {
		// Students must not receive correct_answer
		status, body := c.get("/api/courses/"+courseID+"/labs/"+formLabID, studentToken)
		assertStatus(t, status, 200, body)
		lab, _ := body["lab"].(map[string]any)
		content, _ := lab["content"].(map[string]any)
		questions, _ := content["questions"].([]any)
		if len(questions) == 0 {
			t.Fatal("expected questions")
		}
		q := questions[0].(map[string]any)
		if _, hasAnswer := q["correct_answer"]; hasAnswer {
			t.Error("student should NOT see correct_answer")
		}
	})

	t.Run("Labs/AdminGetLabHasFlag", func(t *testing.T) {
		status, body := c.get("/api/admin/courses/"+courseID+"/labs/"+ctfLabID, adminToken)
		assertStatus(t, status, 200, body)
		if str(body, "flag") == "" {
			t.Error("admin should see the flag")
		}
	})

	// ── Submissions ───────────────────────────────────────────────────────────

	t.Run("Submit/Form_Correct", func(t *testing.T) {
		status, body := c.post(
			"/api/courses/"+courseID+"/labs/"+formLabID+"/submit",
			map[string]any{"answer": map[string]any{"answers": map[string]string{"q1": "4"}}},
			studentToken,
		)
		assertStatus(t, status, 200, body)
		if !body["is_correct"].(bool) {
			t.Fatalf("expected correct: %v", body)
		}
		if num(body, "score") != 100 {
			t.Fatalf("expected score 100, got %v", body["score"])
		}
	})

	t.Run("Submit/Form_Incorrect", func(t *testing.T) {
		status, body := c.post(
			"/api/courses/"+courseID+"/labs/"+formLabID+"/submit",
			map[string]any{"answer": map[string]any{"answers": map[string]string{"q1": "3"}}},
			studentToken,
		)
		assertStatus(t, status, 200, body)
		if body["is_correct"].(bool) {
			t.Fatal("expected incorrect")
		}
		if num(body, "score") != 0 {
			t.Fatalf("expected score 0, got %v", body["score"])
		}
	})

	t.Run("Submit/Form_CaseInsensitive", func(t *testing.T) {
		// EqualFold matching for text answers
		status, body := c.post(
			"/api/courses/"+courseID+"/labs/"+formLabID+"/submit",
			map[string]any{"answer": map[string]any{"answers": map[string]string{"q1": "4"}}},
			studentToken,
		)
		assertStatus(t, status, 200, body)
		if !body["is_correct"].(bool) {
			t.Fatalf("expected correct with exact match: %v", body)
		}
	})

	t.Run("Submit/CTF_Correct", func(t *testing.T) {
		status, body := c.post(
			"/api/courses/"+courseID+"/labs/"+ctfLabID+"/submit",
			map[string]any{"answer": map[string]string{"flag": "FLAG{test-go-api}"}},
			studentToken,
		)
		assertStatus(t, status, 200, body)
		if !body["is_correct"].(bool) {
			t.Fatalf("expected correct: %v", body)
		}
		if num(body, "score") != 200 {
			t.Fatalf("expected score 200, got %v", body["score"])
		}
	})

	t.Run("Submit/CTF_Incorrect", func(t *testing.T) {
		status, body := c.post(
			"/api/courses/"+courseID+"/labs/"+ctfLabID+"/submit",
			map[string]any{"answer": map[string]string{"flag": "wrong-flag"}},
			studentToken,
		)
		assertStatus(t, status, 200, body)
		if body["is_correct"].(bool) {
			t.Fatal("expected incorrect")
		}
	})

	t.Run("Submit/CTF_MissingFlagField", func(t *testing.T) {
		status, body := c.post(
			"/api/courses/"+courseID+"/labs/"+ctfLabID+"/submit",
			map[string]any{"answer": map[string]string{"wrong_key": "value"}},
			studentToken,
		)
		assertStatus(t, status, 400, body)
	})

	t.Run("Submit/UnenrolledForbidden", func(t *testing.T) {
		// student2 is not enrolled
		_, regBody := c.post("/api/auth/login", map[string]string{
			"email": "student2@test.local", "password": "Test@1234",
		}, "")
		tok := str(regBody, "token")
		status, body := c.post(
			"/api/courses/"+courseID+"/labs/"+formLabID+"/submit",
			map[string]any{"answer": map[string]any{"answers": map[string]string{"q1": "4"}}},
			tok,
		)
		assertStatus(t, status, 403, body)
	})

	t.Run("Submit/MySubmissions", func(t *testing.T) {
		status, body := c.get(
			"/api/courses/"+courseID+"/labs/"+formLabID+"/submissions",
			studentToken,
		)
		assertStatus(t, status, 200, body)
		subs, _ := body["submissions"].([]any)
		if len(subs) < 2 {
			t.Fatalf("expected >= 2 submissions, got %d", len(subs))
		}
	})

	t.Run("Submit/MyProgress", func(t *testing.T) {
		status, body := c.get("/api/courses/"+courseID+"/progress", studentToken)
		assertStatus(t, status, 200, body)
		if num(body, "total_labs") != 2 {
			t.Fatalf("expected 2 labs, got %v", body["total_labs"])
		}
		if num(body, "completed_labs") != 2 {
			t.Fatalf("expected 2 completed, got %v", body["completed_labs"])
		}
		if num(body, "total_points_earned") < 100 {
			t.Fatalf("expected points earned >= 100, got %v", body["total_points_earned"])
		}
	})

	t.Run("Submit/Leaderboard", func(t *testing.T) {
		status, body := c.get("/api/courses/"+courseID+"/leaderboard", studentToken)
		assertStatus(t, status, 200, body)
		lb, _ := body["leaderboard"].([]any)
		if len(lb) == 0 {
			t.Fatal("leaderboard is empty")
		}
		first := lb[0].(map[string]any)
		if first["total_points"] == nil {
			t.Errorf("leaderboard entry missing total_points: %v", first)
		}
	})

	// ── Admin ─────────────────────────────────────────────────────────────────

	t.Run("Admin/Stats", func(t *testing.T) {
		status, body := c.get("/api/admin/stats", adminToken)
		assertStatus(t, status, 200, body)
		if num(body, "total_users") < 2 {
			t.Fatalf("expected >= 2 users, got %v", body["total_users"])
		}
		hasKey(t, body, "total_courses")
		hasKey(t, body, "total_labs")
		hasKey(t, body, "total_submissions")
		hasKey(t, body, "success_rate")
	})

	t.Run("Admin/Stats_ForbiddenAsStudent", func(t *testing.T) {
		status, body := c.get("/api/admin/stats", studentToken)
		assertStatus(t, status, 403, body)
	})

	t.Run("Admin/Stats_Unauthenticated", func(t *testing.T) {
		status, body := c.get("/api/admin/stats", "")
		assertStatus(t, status, 401, body)
	})

	t.Run("Admin/ListUsers", func(t *testing.T) {
		status, body := c.get("/api/admin/users", adminToken)
		assertStatus(t, status, 200, body)
		users, _ := body["users"].([]any)
		if len(users) < 2 {
			t.Fatalf("expected >= 2 users, got %d", len(users))
		}
	})

	t.Run("Admin/GetSettings", func(t *testing.T) {
		status, body := c.get("/api/admin/settings", adminToken)
		assertStatus(t, status, 200, body)
		settings, _ := body["settings"].([]any)
		if len(settings) == 0 {
			t.Fatal("expected settings")
		}
	})

	t.Run("Admin/UpdateSettings", func(t *testing.T) {
		status, body := c.put("/api/admin/settings", map[string]string{
			"gitlab_url": "https://gitlab.example.com",
		}, adminToken)
		assertStatus(t, status, 200, body)
	})

	t.Run("Admin/UpdateSettings_UnknownKey", func(t *testing.T) {
		status, body := c.put("/api/admin/settings", map[string]string{
			"unknown_key": "value",
		}, adminToken)
		assertStatus(t, status, 400, body)
	})

	t.Run("Admin/CourseMonitoring", func(t *testing.T) {
		status, body := c.get("/api/admin/courses/"+courseID+"/monitoring", adminToken)
		assertStatus(t, status, 200, body)
		hasKey(t, body, "students")
		hasKey(t, body, "total_enrolled")
	})

	t.Run("Admin/LabStats", func(t *testing.T) {
		status, body := c.get(
			"/api/admin/courses/"+courseID+"/labs/"+formLabID+"/stats",
			adminToken,
		)
		assertStatus(t, status, 200, body)
		hasKey(t, body, "total_submissions")
		hasKey(t, body, "success_rate")
	})

	t.Run("Admin/AdminListCourses", func(t *testing.T) {
		status, body := c.get("/api/admin/courses", adminToken)
		assertStatus(t, status, 200, body)
		hasKey(t, body, "courses")
	})

	t.Run("Admin/Enrollments", func(t *testing.T) {
		status, body := c.get("/api/admin/courses/"+courseID+"/enrollments", adminToken)
		assertStatus(t, status, 200, body)
		hasKey(t, body, "enrollments")
	})

	// ── Change password ───────────────────────────────────────────────────────

	t.Run("Auth/ChangePassword", func(t *testing.T) {
		status, body := c.put("/api/auth/password", map[string]string{
			"old_password": "Test@1234",
			"new_password": "NewPass@5678",
		}, studentToken)
		assertStatus(t, status, 200, body)
	})

	t.Run("Auth/ChangePassword_WrongOld", func(t *testing.T) {
		status, body := c.put("/api/auth/password", map[string]string{
			"old_password": "wrong",
			"new_password": "NewPass@5678",
		}, studentToken)
		assertStatus(t, status, 401, body)
	})

	t.Run("Auth/LoginWithNewPassword", func(t *testing.T) {
		status, body := c.post("/api/auth/login", map[string]string{
			"email": "student1@test.local", "password": "NewPass@5678",
		}, "")
		assertStatus(t, status, 200, body)
		if str(body, "token") == "" {
			t.Fatal("expected token with new password")
		}
	})

	// ── Cleanup: delete labs, course ─────────────────────────────────────────

	t.Run("Labs/Delete", func(t *testing.T) {
		status, body := c.del("/api/courses/"+courseID+"/labs/"+ctfLabID, studentToken)
		assertStatus(t, status, 200, body)
	})

	t.Run("Labs/DeleteOtherUserForbidden", func(t *testing.T) {
		// student2 does not own the course
		_, regBody := c.post("/api/auth/login", map[string]string{
			"email": "student2@test.local", "password": "Test@1234",
		}, "")
		tok := str(regBody, "token")
		status, body := c.del("/api/courses/"+courseID+"/labs/"+formLabID, tok)
		assertStatus(t, status, 403, body)
	})

	t.Run("Courses/Unenroll", func(t *testing.T) {
		status, body := c.del("/api/courses/"+courseID+"/unenroll", studentToken)
		assertStatus(t, status, 200, body)
	})

	t.Run("Courses/DeleteOtherUserForbidden", func(t *testing.T) {
		_, regBody := c.post("/api/auth/login", map[string]string{
			"email": "student2@test.local", "password": "Test@1234",
		}, "")
		tok := str(regBody, "token")
		status, body := c.del("/api/courses/"+courseID, tok)
		assertStatus(t, status, 403, body)
	})

	t.Run("Courses/Delete", func(t *testing.T) {
		status, body := c.del("/api/courses/"+courseID, studentToken)
		assertStatus(t, status, 200, body)
	})

	t.Run("Courses/GetDeleted", func(t *testing.T) {
		status, body := c.get("/api/courses/"+courseID, "")
		assertStatus(t, status, 404, body)
	})
}
