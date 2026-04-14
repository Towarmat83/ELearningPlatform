package handlers

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/elearning/api-go/internal/metrics"
)

type submissionResult struct {
	IsCorrect       bool             `json:"is_correct"`
	Score           int32            `json:"score"`
	MaxScore        int32            `json:"max_score"`
	Feedback        *string          `json:"feedback"`
	QuestionResults []questionResult `json:"question_results,omitempty"`
	FlagResults     []flagResult     `json:"flag_results,omitempty"`
}

type questionResult struct {
	QuestionID    string  `json:"question_id"`
	IsCorrect     bool    `json:"is_correct"`
	PointsEarned  int32   `json:"points_earned"`
	CorrectAnswer *string `json:"correct_answer"`
	Explanation   *string `json:"explanation"`
}

type flagResult struct {
	FlagID       string `json:"flag_id"`
	Name         string `json:"name"`
	IsCorrect    bool   `json:"is_correct"`
	PointsEarned int32  `json:"points_earned"`
}

func strPtr(s string) *string { return &s }

type appError struct {
	Status  int
	Message string
}

func (e *appError) Error() string { return e.Message }

func gradeCTF(labPoints int32, flag *string, content json.RawMessage, answer json.RawMessage) (submissionResult, error) {
	// Check for multi-flag mode
	var contentMap map[string]json.RawMessage
	if err := json.Unmarshal(content, &contentMap); err == nil {
		if rawFlags, ok := contentMap["flags"]; ok {
			var flagsMeta []map[string]json.RawMessage
			if err := json.Unmarshal(rawFlags, &flagsMeta); err == nil && len(flagsMeta) > 0 {
				return gradeCTFMulti(labPoints, flag, flagsMeta, answer), nil
			}
		}
	}

	// Single flag mode
	var answerMap map[string]string
	if err := json.Unmarshal(answer, &answerMap); err != nil || answerMap["flag"] == "" {
		return submissionResult{}, &appError{400, "Missing 'flag' field in answer"}
	}
	submitted := strings.TrimFunc(answerMap["flag"], unicode.IsControl)
	expected := ""
	if flag != nil {
		expected = strings.TrimSpace(*flag)
	}
	isCorrect := submitted == expected
	score := int32(0)
	if isCorrect {
		score = labPoints
	}
	fb := "Incorrect flag. Keep trying!"
	if isCorrect {
		fb = "Correct flag! Well done!"
	}
	return submissionResult{
		IsCorrect: isCorrect, Score: score, MaxScore: labPoints, Feedback: strPtr(fb),
	}, nil
}

func gradeCTFMulti(labPoints int32, flag *string, flagsMeta []map[string]json.RawMessage, answer json.RawMessage) submissionResult {
	flagMap := make(map[string]string)
	if flag != nil {
		json.Unmarshal([]byte(*flag), &flagMap) //nolint:errcheck
	}

	var answerOuter map[string]json.RawMessage
	var submittedFlags map[string]string
	if err := json.Unmarshal(answer, &answerOuter); err == nil {
		json.Unmarshal(answerOuter["flags"], &submittedFlags) //nolint:errcheck
	}

	var flagResults []flagResult
	var totalFlagPts, earnedPts int32
	for _, meta := range flagsMeta {
		var flagID, flagName string
		var flagPts int32
		json.Unmarshal(meta["id"], &flagID)      //nolint:errcheck
		json.Unmarshal(meta["name"], &flagName)  //nolint:errcheck
		json.Unmarshal(meta["points"], &flagPts) //nolint:errcheck

		expected := flagMap[flagID]
		submitted := strings.TrimSpace(submittedFlags[flagID])
		isCorrect := submitted != "" && submitted == expected
		pts := int32(0)
		if isCorrect {
			pts = flagPts
		}
		totalFlagPts += flagPts
		earnedPts += pts
		flagResults = append(flagResults, flagResult{
			FlagID: flagID, Name: flagName, IsCorrect: isCorrect, PointsEarned: pts,
		})
	}

	score := int32(0)
	if totalFlagPts > 0 {
		score = int32(math.Round(float64(earnedPts) / float64(totalFlagPts) * float64(labPoints)))
	}
	found := 0
	for _, fr := range flagResults {
		if fr.IsCorrect {
			found++
		}
	}
	isCorrect := found == len(flagResults) && len(flagResults) > 0
	fb := fmt.Sprintf("%d/%d flags captured", found, len(flagResults))
	return submissionResult{
		IsCorrect: isCorrect, Score: score, MaxScore: labPoints,
		Feedback: strPtr(fb), FlagResults: flagResults,
	}
}

func gradeForm(labPoints int32, content json.RawMessage, answer json.RawMessage) (submissionResult, error) {
	var contentMap map[string]json.RawMessage
	if err := json.Unmarshal(content, &contentMap); err != nil {
		return submissionResult{}, &appError{500, "Invalid form lab content"}
	}
	var questions []map[string]json.RawMessage
	if err := json.Unmarshal(contentMap["questions"], &questions); err != nil {
		return submissionResult{}, &appError{500, "Invalid form lab questions"}
	}

	var answerMap map[string]json.RawMessage
	if err := json.Unmarshal(answer, &answerMap); err != nil {
		return submissionResult{}, &appError{400, "Missing 'answers' object in answer"}
	}
	var submittedAnswers map[string]string
	if err := json.Unmarshal(answerMap["answers"], &submittedAnswers); err != nil {
		return submissionResult{}, &appError{400, "Missing 'answers' object in answer"}
	}

	var totalPts, earnedPts int32
	var results []questionResult
	for _, q := range questions {
		var qID, correctAnswer string
		var qPts int32
		json.Unmarshal(q["id"], &qID)                       //nolint:errcheck
		json.Unmarshal(q["correct_answer"], &correctAnswer) //nolint:errcheck
		json.Unmarshal(q["points"], &qPts)                  //nolint:errcheck
		var explanation *string
		if raw, ok := q["explanation"]; ok {
			var exp string
			if json.Unmarshal(raw, &exp) == nil {
				explanation = &exp
			}
		}
		totalPts += qPts
		submitted := strings.TrimSpace(submittedAnswers[qID])
		isCorrect := strings.EqualFold(submitted, strings.TrimSpace(correctAnswer))
		pts := int32(0)
		if isCorrect {
			pts = qPts
		}
		earnedPts += pts
		ca := correctAnswer
		results = append(results, questionResult{
			QuestionID: qID, IsCorrect: isCorrect, PointsEarned: pts,
			CorrectAnswer: &ca, Explanation: explanation,
		})
	}

	score := int32(0)
	if totalPts > 0 {
		score = int32(math.Round(float64(earnedPts) / float64(totalPts) * float64(labPoints)))
	}
	pct := 0.0
	if totalPts > 0 {
		pct = float64(earnedPts) / float64(totalPts) * 100
	}
	fb := fmt.Sprintf("You got %d/%d points (%.0f%%)", earnedPts, totalPts, pct)
	return submissionResult{
		IsCorrect:       earnedPts == totalPts && totalPts > 0,
		Score:           score,
		MaxScore:        labPoints,
		Feedback:        strPtr(fb),
		QuestionResults: results,
	}, nil
}

// POST /api/courses/{course_id}/labs/{lab_id}/submit
func (s *State) SubmitLab(w http.ResponseWriter, r *http.Request) {
	courseID := param(r, "course_id")
	labID := param(r, "lab_id")
	c := s.claims(r)
	ctx := r.Context()

	var enrolled int64
	s.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM enrollments WHERE user_id = $1::uuid AND course_id = $2::uuid",
		c.Subject, courseID).Scan(&enrolled)
	if enrolled == 0 {
		s.Error(w, http.StatusForbidden, "You must enroll first")
		return
	}

	var labType, contentStr string
	var flag *string
	var labPoints int32
	err := s.Pool.QueryRow(ctx, `
		SELECT lab_type, content::text, flag, points
		FROM labs WHERE id = $1::uuid AND course_id = $2::uuid AND is_published = TRUE`,
		labID, courseID).Scan(&labType, &contentStr, &flag, &labPoints)
	if err != nil {
		s.Error(w, http.StatusNotFound, "Lab not found")
		return
	}

	var reqBody struct {
		Answer json.RawMessage `json:"answer"`
	}
	if err := decode(r, &reqBody); err != nil {
		s.Error(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	var result submissionResult
	switch labType {
	case "ctf":
		result, err = gradeCTF(labPoints, flag, json.RawMessage(contentStr), reqBody.Answer)
	case "form":
		result, err = gradeForm(labPoints, json.RawMessage(contentStr), reqBody.Answer)
	default:
		s.Error(w, http.StatusBadRequest, "This lab type does not accept submissions")
		return
	}
	if err != nil {
		if ae, ok := err.(*appError); ok {
			s.Error(w, ae.Status, ae.Message)
		} else {
			s.Error(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	label := "incorrect"
	if result.IsCorrect {
		label = "correct"
	}
	metrics.LabSubmissionsTotal.WithLabelValues(labType, label).Inc()

	var currentAttempts, currentBest int32
	s.Pool.QueryRow(ctx,
		"SELECT total_attempts, best_score FROM lab_progress WHERE user_id = $1::uuid AND lab_id = $2::uuid",
		c.Subject, labID).Scan(&currentAttempts, &currentBest)

	totalAttempts := currentAttempts + 1
	bestScore := currentBest
	if result.Score > bestScore {
		bestScore = result.Score
	}

	answerBytes, _ := json.Marshal(reqBody.Answer)
	if _, err = s.Pool.Exec(ctx, `
		INSERT INTO lab_submissions (lab_id, user_id, answer, is_correct, score, attempts)
		VALUES ($1::uuid, $2::uuid, $3::jsonb, $4, $5, $6)`,
		labID, c.Subject, string(answerBytes), result.IsCorrect, result.Score, totalAttempts); err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}

	var completedAt *time.Time
	if result.IsCorrect {
		now := time.Now()
		completedAt = &now
	}
	if _, err = s.Pool.Exec(ctx, `
		INSERT INTO lab_progress (user_id, lab_id, course_id, completed, best_score, total_attempts, completed_at, last_attempt_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7, NOW())
		ON CONFLICT (user_id, lab_id) DO UPDATE SET
			completed       = CASE WHEN lab_progress.completed THEN TRUE ELSE $4 END,
			best_score      = $5,
			total_attempts  = $6,
			completed_at    = CASE WHEN lab_progress.completed_at IS NOT NULL THEN lab_progress.completed_at ELSE $7 END,
			last_attempt_at = NOW()`,
		c.Subject, labID, courseID, result.IsCorrect, bestScore, totalAttempts, completedAt); err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}

	s.JSON(w, http.StatusOK, result)
}

// GET /api/courses/{course_id}/labs/{lab_id}/submissions
func (s *State) MySubmissions(w http.ResponseWriter, r *http.Request) {
	labID := param(r, "lab_id")
	c := s.claims(r)

	rows, err := s.Pool.Query(r.Context(), `
		SELECT id::text, answer::text, is_correct, score, attempts, submitted_at::text
		FROM lab_submissions
		WHERE user_id = $1::uuid AND lab_id = $2::uuid
		ORDER BY submitted_at DESC LIMIT 20`, c.Subject, labID)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer rows.Close()

	type sub struct {
		ID          string          `json:"id"`
		Answer      json.RawMessage `json:"answer"`
		IsCorrect   bool            `json:"is_correct"`
		Score       int32           `json:"score"`
		Attempts    int32           `json:"attempts"`
		SubmittedAt string          `json:"submitted_at"`
	}
	submissions := make([]sub, 0)
	for rows.Next() {
		var sb sub
		var answerStr string
		if err := rows.Scan(&sb.ID, &answerStr, &sb.IsCorrect, &sb.Score, &sb.Attempts, &sb.SubmittedAt); err != nil {
			s.Error(w, http.StatusInternalServerError, "Scan error")
			return
		}
		sb.Answer = json.RawMessage(answerStr)
		submissions = append(submissions, sb)
	}
	s.JSON(w, http.StatusOK, map[string]any{"submissions": submissions})
}

// GET /api/courses/{course_id}/progress
func (s *State) MyProgress(w http.ResponseWriter, r *http.Request) {
	courseID := param(r, "course_id")
	c := s.claims(r)

	rows, err := s.Pool.Query(r.Context(), `
		SELECT l.id::text, l.title, l.lab_type, l.points,
		       COALESCE(lp.completed, FALSE),
		       COALESCE(lp.best_score, 0),
		       COALESCE(lp.total_attempts, 0),
		       lp.completed_at::text
		FROM labs l
		LEFT JOIN lab_progress lp ON lp.lab_id = l.id AND lp.user_id = $1::uuid
		WHERE l.course_id = $2::uuid AND l.is_published = TRUE
		ORDER BY l.order_index ASC`, c.Subject, courseID)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer rows.Close()

	type labProg struct {
		LabID         string  `json:"lab_id"`
		LabTitle      string  `json:"lab_title"`
		LabType       string  `json:"lab_type"`
		Points        int32   `json:"points"`
		Completed     bool    `json:"completed"`
		BestScore     int32   `json:"best_score"`
		TotalAttempts int32   `json:"total_attempts"`
		CompletedAt   *string `json:"completed_at"`
	}
	progList := make([]labProg, 0)
	for rows.Next() {
		var lp labProg
		if err := rows.Scan(&lp.LabID, &lp.LabTitle, &lp.LabType, &lp.Points,
			&lp.Completed, &lp.BestScore, &lp.TotalAttempts, &lp.CompletedAt); err != nil {
			s.Error(w, http.StatusInternalServerError, "Scan error")
			return
		}
		progList = append(progList, lp)
	}

	totalLabs := int64(len(progList))
	var completedLabs, totalPossible, totalEarned int64
	for _, lp := range progList {
		if lp.Completed {
			completedLabs++
		}
		totalPossible += int64(lp.Points)
		totalEarned += int64(lp.BestScore)
	}
	pct := 0.0
	if totalLabs > 0 {
		pct = float64(completedLabs) / float64(totalLabs) * 100.0
	}
	s.JSON(w, http.StatusOK, map[string]any{
		"course_id": courseID, "user_id": c.Subject,
		"total_labs": totalLabs, "completed_labs": completedLabs,
		"total_points_possible": totalPossible, "total_points_earned": totalEarned,
		"completion_percentage": pct, "lab_progress": progList,
	})
}

// GET /api/admin/courses/{course_id}/monitoring
func (s *State) AdminCourseMonitoring(w http.ResponseWriter, r *http.Request) {
	courseID := param(r, "course_id")
	ctx := r.Context()

	var courseTitle string
	if err := s.Pool.QueryRow(ctx, "SELECT title FROM courses WHERE id = $1::uuid", courseID).
		Scan(&courseTitle); err != nil {
		s.Error(w, http.StatusNotFound, "Course not found")
		return
	}
	var totalEnrolled int64
	s.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM enrollments WHERE course_id = $1::uuid", courseID).Scan(&totalEnrolled)

	rows, err := s.Pool.Query(ctx, `
		SELECT u.id::text, u.username, u.email,
		       COUNT(DISTINCT lp.lab_id) FILTER (WHERE lp.completed = TRUE)::bigint,
		       COALESCE(SUM(lp.best_score), 0)::bigint,
		       MAX(lp.last_attempt_at)::text
		FROM enrollments e
		JOIN users u ON u.id = e.user_id
		LEFT JOIN lab_progress lp ON lp.user_id = u.id AND lp.course_id = $1::uuid
		WHERE e.course_id = $1::uuid
		GROUP BY u.id, u.username, u.email
		ORDER BY COALESCE(SUM(lp.best_score), 0) DESC`, courseID)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer rows.Close()

	type studentEntry struct {
		UserID        string  `json:"user_id"`
		Username      string  `json:"username"`
		Email         string  `json:"email"`
		CompletedLabs int64   `json:"completed_labs"`
		TotalPoints   int64   `json:"total_points"`
		LastActivity  *string `json:"last_activity"`
	}
	students := make([]studentEntry, 0)
	for rows.Next() {
		var e studentEntry
		if err := rows.Scan(&e.UserID, &e.Username, &e.Email, &e.CompletedLabs, &e.TotalPoints, &e.LastActivity); err != nil {
			s.Error(w, http.StatusInternalServerError, "Scan error")
			return
		}
		students = append(students, e)
	}
	s.JSON(w, http.StatusOK, map[string]any{
		"course_id": courseID, "course_title": courseTitle,
		"total_enrolled": totalEnrolled, "student_progress": students,
	})
}

// GET /api/admin/courses/{course_id}/labs/{lab_id}/submissions
func (s *State) AdminLabSubmissions(w http.ResponseWriter, r *http.Request) {
	labID := param(r, "lab_id")
	rows, err := s.Pool.Query(r.Context(), `
		SELECT ls.id::text, ls.user_id::text, u.username, ls.is_correct, ls.score, ls.attempts, ls.submitted_at::text
		FROM lab_submissions ls
		JOIN users u ON u.id = ls.user_id
		WHERE ls.lab_id = $1::uuid
		ORDER BY ls.submitted_at DESC`, labID)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer rows.Close()

	type subRow struct {
		ID          string `json:"id"`
		UserID      string `json:"user_id"`
		Username    string `json:"username"`
		IsCorrect   bool   `json:"is_correct"`
		Score       int32  `json:"score"`
		Attempts    int32  `json:"attempts"`
		SubmittedAt string `json:"submitted_at"`
	}
	submissions := make([]subRow, 0)
	for rows.Next() {
		var sb subRow
		if err := rows.Scan(&sb.ID, &sb.UserID, &sb.Username, &sb.IsCorrect, &sb.Score, &sb.Attempts, &sb.SubmittedAt); err != nil {
			s.Error(w, http.StatusInternalServerError, "Scan error")
			return
		}
		submissions = append(submissions, sb)
	}
	s.JSON(w, http.StatusOK, map[string]any{"submissions": submissions})
}

// GET /api/admin/courses/{course_id}/labs/{lab_id}/stats
func (s *State) AdminLabStats(w http.ResponseWriter, r *http.Request) {
	labID := param(r, "lab_id")
	ctx := r.Context()

	var totalSubs, uniqueStudents, maxScore int64
	var successRate, avgScore float64
	s.Pool.QueryRow(ctx, `
		SELECT COUNT(*)::bigint, COUNT(DISTINCT user_id)::bigint,
		       COALESCE(AVG(CASE WHEN is_correct THEN 100.0 ELSE 0.0 END), 0),
		       COALESCE(AVG(score::float8), 0),
		       COALESCE(MAX(score), 0)::bigint
		FROM lab_submissions WHERE lab_id = $1::uuid`, labID).
		Scan(&totalSubs, &uniqueStudents, &successRate, &avgScore, &maxScore)

	var completedCount int64
	var avgAttempts float64
	s.Pool.QueryRow(ctx, `
		SELECT COUNT(*)::bigint, COALESCE(AVG(total_attempts::float8), 0)
		FROM lab_progress WHERE lab_id = $1::uuid AND completed = TRUE`, labID).
		Scan(&completedCount, &avgAttempts)

	s.JSON(w, http.StatusOK, map[string]any{
		"total_submissions":       totalSubs,
		"unique_students":          uniqueStudents,
		"success_rate":             math.Round(successRate*10) / 10,
		"avg_score":                math.Round(avgScore*10) / 10,
		"max_score_achieved":       maxScore,
		"completed_count":          completedCount,
		"avg_attempts_to_complete": math.Round(avgAttempts*10) / 10,
	})
}

// GET /api/admin/stats
func (s *State) AdminStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var totalUsers, totalCourses, totalLabs, totalSubs, totalEnroll int64
	var successRate float64

	s.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&totalUsers)
	s.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM courses").Scan(&totalCourses)
	s.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM labs").Scan(&totalLabs)
	s.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM lab_submissions").Scan(&totalSubs)
	s.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM enrollments").Scan(&totalEnroll)
	s.Pool.QueryRow(ctx,
		"SELECT COALESCE(AVG(CASE WHEN is_correct THEN 1.0 ELSE 0.0 END)*100, 0)::float8 FROM lab_submissions").
		Scan(&successRate)

	metrics.ActiveUsers.Set(float64(totalUsers))
	metrics.ActiveCourses.Set(float64(totalCourses))
	metrics.EnrollmentsTotal.Set(float64(totalEnroll))

	s.JSON(w, http.StatusOK, map[string]any{
		"total_users":       totalUsers,
		"total_courses":     totalCourses,
		"total_labs":        totalLabs,
		"total_submissions": totalSubs,
		"total_enrollments": totalEnroll,
		"success_rate":      fmt.Sprintf("%.1f", successRate),
	})
}
