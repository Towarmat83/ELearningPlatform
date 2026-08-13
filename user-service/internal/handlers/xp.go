package handlers

import (
	"net/http"

	"go.uber.org/zap"

	"github.com/genesary/pupitre/user-service/internal/repository"
)

// xpLevel thresholds and names for the five learner tiers.
const (
	xpLevelNovice    = 1
	xpLevelApprenant = 2
	xpLevelConfirme  = 3
	xpLevelExpert    = 4
	xpLevelMaitre    = 5

	xpThresholdApprenant = 200
	xpThresholdConfirme  = 600
	xpThresholdExpert    = 1500
	xpThresholdMaitre    = 3500
)

// levelNames maps a level number to its display name.
//
//nolint:gochecknoglobals // static lookup table, read-only after init
var levelNames = [xpLevelMaitre + 1]string{
	"",
	"Novice",
	"Apprenant",
	"Confirmé",
	"Expert",
	"Maître",
}

// xpLevel returns the tier (1–5) corresponding to total XP.
func xpLevel(total int) int {
	switch {
	case total >= xpThresholdMaitre:
		return xpLevelMaitre
	case total >= xpThresholdExpert:
		return xpLevelExpert
	case total >= xpThresholdConfirme:
		return xpLevelConfirme
	case total >= xpThresholdApprenant:
		return xpLevelApprenant
	default:
		return xpLevelNovice
	}
}

// xpResponse is the payload returned by MyXP.
type xpResponse struct {
	Total     int                     `json:"total"`
	Level     int                     `json:"level"`
	LevelName string                  `json:"levelName"`
	History   []repository.XPEventRow `json:"history"`
}

// MyXP godoc
// @Summary  Get the current user's XP total, level and history
// @Tags     XP
// @Security BearerAuth
// @Produce  json
// @Success  200  {object}  xpResponse
// @Router   /api/my/xp [get].
func (s *State) MyXP(writer http.ResponseWriter, req *http.Request) {
	claims := s.claims(req)
	ctx := req.Context()

	total, err := s.Repos.XP.Total(ctx, claims.Subject)
	if err != nil {
		zap.L().Error("failed to get xp total", zap.String("userID", claims.Subject), zap.Error(err))
		s.Error(writer, http.StatusInternalServerError, "Failed to fetch XP")

		return
	}

	history, err := s.Repos.XP.History(ctx, claims.Subject)
	if err != nil {
		zap.L().Error("failed to get xp history", zap.String("userID", claims.Subject), zap.Error(err))
		s.Error(writer, http.StatusInternalServerError, "Failed to fetch XP history")

		return
	}

	if history == nil {
		history = []repository.XPEventRow{}
	}

	lvl := xpLevel(total)
	s.JSON(writer, http.StatusOK, xpResponse{
		Total:     total,
		Level:     lvl,
		LevelName: levelNames[lvl],
		History:   history,
	})
}

// awardXP records an XP gain for userID, logging any error without failing the
// caller — XP errors must never block progress tracking or enrollment.
func (s *State) awardXP(req *http.Request, userID, source, sourceSlug string, amount int) {
	err := s.Repos.XP.Award(req.Context(), userID, source, sourceSlug, amount)
	if err != nil {
		zap.L().Warn("failed to award xp",
			zap.String("userID", userID),
			zap.String("source", source),
			zap.String("sourceSlug", sourceSlug),
			zap.Error(err))
	}
}
