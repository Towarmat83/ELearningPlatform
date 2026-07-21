package handlers

import (
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/google/uuid"

	"github.com/genesary/pupitre/user-service/internal/repository"
)

// badgeResponseKey is the JSON key used for badge arrays in responses.
const badgeResponseKey = "badges"

// badgeDetail enriches a BadgeRow with the course-service badge metadata.
type badgeDetail struct {
	CourseSlug  string    `json:"courseSlug"`
	CourseTitle string    `json:"courseTitle"`
	BadgeName   string    `json:"name"`
	BadgeIcon   string    `json:"icon,omitempty"`
	EarnedAt    time.Time `json:"earnedAt"`
	EarnedBy    int64     `json:"earnedBy"`
}

// courseBadgeInfo is the badge metadata returned by course-service.
type courseBadgeInfo struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
	Badge *struct {
		Name string `json:"name"`
		Icon string `json:"icon,omitempty"`
	} `json:"badge,omitempty"`
}

// fetchCourseBadgeInfo fetches badge metadata for a course from course-service.
func (s *State) fetchCourseBadgeInfo(req *http.Request, courseSlug string) (*courseBadgeInfo, error) {
	if !slugRE.MatchString(courseSlug) {
		return nil, fmt.Errorf("invalid course slug: %q", courseSlug)
	}

	var info courseBadgeInfo

	err := s.fetchCourseServiceJSON(req, "/api/courses/"+courseSlug, &info)
	if err != nil {
		return nil, err
	}

	return &info, nil
}

// enrichBadgeRows converts BadgeRows into badgeDetails by fetching metadata.
func (s *State) enrichBadgeRows(req *http.Request, rows []repository.BadgeRow) []badgeDetail {
	details := make([]badgeDetail, 0, len(rows))

	for _, row := range rows {
		detail := badgeDetail{
			CourseSlug: row.CourseSlug,
			EarnedAt:   row.EarnedAt,
		}

		info, infoErr := s.fetchCourseBadgeInfo(req, row.CourseSlug)
		if infoErr != nil {
			zap.L().Warn("failed to fetch course badge info", zap.String("courseSlug", row.CourseSlug), zap.Error(infoErr))
		} else {
			detail.CourseTitle = info.Title
			if info.Badge != nil {
				detail.BadgeName = info.Badge.Name
				detail.BadgeIcon = info.Badge.Icon
			}
		}

		count, countErr := s.Repos.Badges.EarnedCount(req.Context(), row.CourseSlug)
		if countErr != nil {
			zap.L().Warn("failed to count badge earners", zap.String("courseSlug", row.CourseSlug), zap.Error(countErr))
		} else {
			detail.EarnedBy = count
		}

		details = append(details, detail)
	}

	return details
}

// MyBadges godoc
// @Summary  List badges earned by the current user
// @Tags     Badges
// @Security BearerAuth
// @Produce  json
// @Success  200  {object}  map[string]interface{}
// @Router   /api/my/badges [get].
func (s *State) MyBadges(writer http.ResponseWriter, request *http.Request) {
	claims := s.claims(request)

	rows, err := s.Repos.Badges.MyBadges(request.Context(), claims.Subject)
	if err != nil {
		zap.L().Error("failed to query my badges", zap.String("userID", claims.Subject), zap.Error(err))
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	s.JSON(writer, http.StatusOK, map[string]any{badgeResponseKey: s.enrichBadgeRows(request, rows)})
}

// PublicUser godoc
// @Summary  Get public profile of a user
// @Tags     Users
// @Produce  json
// @Param    id  path  string  true  "User UUID"
// @Success  200  {object}  map[string]interface{}
// @Router   /api/users/{id} [get].
func (s *State) PublicUser(writer http.ResponseWriter, request *http.Request) {
	userID, err := uuid.Parse(param(request, "id"))
	if err != nil {
		s.Error(writer, http.StatusBadRequest, "Invalid user ID")

		return
	}

	found, err := s.Repos.Users.FindByID(request.Context(), userID)
	if err != nil {
		s.Error(writer, http.StatusNotFound, "User not found")

		return
	}

	s.JSON(writer, http.StatusOK, map[string]any{
		"id":        found.ID,
		"username":  found.Username,
		"avatarUrl": found.AvatarURL,
	})
}

// UserBadges godoc
// @Summary  List badges earned by a user (public)
// @Tags     Badges
// @Produce  json
// @Param    id  path  string  true  "User UUID"
// @Success  200  {object}  map[string]interface{}
// @Router   /api/users/{id}/badges [get].
func (s *State) UserBadges(writer http.ResponseWriter, request *http.Request) {
	userID := param(request, "id")

	rows, err := s.Repos.Badges.UserBadges(request.Context(), userID)
	if err != nil {
		zap.L().Error("failed to query user badges", zap.String("userID", userID), zap.Error(err))
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	s.JSON(writer, http.StatusOK, map[string]any{badgeResponseKey: s.enrichBadgeRows(request, rows)})
}

// BadgeStats godoc
// @Summary  Get global stats for a course badge
// @Tags     Badges
// @Produce  json
// @Param    slug  path  string  true  "Course slug"
// @Success  200  {object}  map[string]interface{}
// @Router   /api/badges/{slug} [get].
func (s *State) BadgeStats(writer http.ResponseWriter, request *http.Request) {
	courseSlug := param(request, "slug")

	count, err := s.Repos.Badges.EarnedCount(request.Context(), courseSlug)
	if err != nil {
		zap.L().Error("failed to count badge earners", zap.String("courseSlug", courseSlug), zap.Error(err))
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	s.JSON(writer, http.StatusOK, map[string]any{"courseSlug": courseSlug, "earnedBy": count})
}
