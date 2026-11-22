package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/feriosch/nando-server/internal/calories/model"
	"github.com/feriosch/nando-server/internal/calories/utils"
	coremodel "github.com/feriosch/nando-server/internal/core/model"
	coreutils "github.com/feriosch/nando-server/internal/core/utils"
)

// DailyLimitsService handles requests about the Daily Limit.
type DailyLimitsService struct {
	dailyLimits model.DailyLimitsRepository
	core        coremodel.CoreService
}

// NewDailyLimits returns the DailyLimitsService.
func NewDailyLimits(dailyLimits model.DailyLimitsRepository, core coremodel.CoreService) *DailyLimitsService {
	return &DailyLimitsService{dailyLimits: dailyLimits, core: core}
}

// Routes adds the Daily Limit route to mux, /api/calories/daily-limit. The
// web app only reads it: the Calories Assistant sets it, through
// calories_set_daily_limit.
func (s *DailyLimitsService) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/calories/daily-limit", s.getDailyLimit)
}

// getDailyLimit answers the Daily Limit in force now, or null if none was
// ever set.
func (s *DailyLimitsService) getDailyLimit(w http.ResponseWriter, r *http.Request) {
	limits, err := s.dailyLimits.List(r.Context())
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	latest, ok := limits.Latest()
	if !ok {
		coreutils.WriteJSON(w, http.StatusOK, nil)
		return
	}
	coreutils.WriteJSON(w, http.StatusOK, model.DailyLimitJSON{Kcal: latest.Kcal, EffectiveFrom: latest.EffectiveFrom})
}

// Tools are what the DailyLimitsService offers Connectors.
func (s *DailyLimitsService) Tools() []coremodel.Tool {
	return []coremodel.Tool{{
		Name: "calories_get_daily_limit",
		Description: "The Daily Limit in force now: the most kcal the Owner intends to consume in a Day, a ceiling to stay under, " +
			"and the Day it applies from. null if the Owner never set one.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		ReadOnly:    true,
		Handle:      s.getDailyLimitTool,
	}, {
		Name: "calories_set_daily_limit",
		Description: "Sets the Daily Limit from today, in kcal (1 to 10,000): Days before today keep the limit they had, " +
			"and a second change on the same Day replaces the first. Answers the Daily Limit in force.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"kcal":{"type":"integer","minimum":1,"maximum":10000,` +
			`"description":"The most kcal the Owner means to consume in a Day."}},"required":["kcal"]}`),
		Handle: s.setDailyLimitTool,
	}}
}

func (s *DailyLimitsService) setDailyLimitTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		Kcal *int `json:"kcal"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, errors.New("kcal must be a whole number of kcal")
	}
	kcal, err := utils.CheckDailyLimit("kcal", args.Kcal)
	if err != nil {
		return nil, err
	}
	today, err := s.core.Today(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.dailyLimits.Save(ctx, model.DailyLimit{EffectiveFrom: today, Kcal: kcal}); err != nil {
		return nil, err
	}
	return s.getDailyLimitTool(ctx, nil)
}

func (s *DailyLimitsService) getDailyLimitTool(ctx context.Context, _ json.RawMessage) (any, error) {
	limits, err := s.dailyLimits.List(ctx)
	if err != nil {
		return nil, err
	}
	var answer struct {
		DailyLimit *model.DailyLimitJSON `json:"dailyLimit"`
	}
	if latest, ok := limits.Latest(); ok {
		answer.DailyLimit = &model.DailyLimitJSON{Kcal: latest.Kcal, EffectiveFrom: latest.EffectiveFrom}
	}
	return answer, nil
}
