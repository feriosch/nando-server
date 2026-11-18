package service

import (
	"context"
	"encoding/json"
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

// Routes adds the Daily Limit routes to mux, under /api/calories/daily-limit.
func (s *DailyLimitsService) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/calories/daily-limit", s.getDailyLimit)
	mux.HandleFunc("PUT /api/calories/daily-limit", s.putDailyLimit)
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

// putDailyLimit changes the Daily Limit from today.
func (s *DailyLimitsService) putDailyLimit(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Kcal *int `json:"kcal"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "the body isn't a Daily Limit", http.StatusBadRequest)
		return
	}
	kcal, err := utils.CheckDailyLimit("kcal", body.Kcal)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	today, err := s.core.Today(r.Context())
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	if err := s.dailyLimits.Save(r.Context(), model.DailyLimit{EffectiveFrom: today, Kcal: kcal}); err != nil {
		coreutils.ServerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
	}}
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
