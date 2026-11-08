package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"

	"github.com/feriosch/nando-server/internal/core/model"
)

// WeighInsService handles requests about Weigh-ins. It asks the
// SettingsService for Today, to know which weeks can still be changed.
type WeighInsService struct {
	weighIns model.WeighInsRepository
	settings *SettingsService
}

// NewWeighIns returns the WeighInsService.
func NewWeighIns(weighIns model.WeighInsRepository, settings *SettingsService) *WeighInsService {
	return &WeighInsService{weighIns: weighIns, settings: settings}
}

// Routes adds the Weigh-in routes to mux, under /api/weigh-ins.
func (s *WeighInsService) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/weigh-ins", s.listWeighIns)
	mux.HandleFunc("PUT /api/weigh-ins/{week}", s.putWeighIn)
	mux.HandleFunc("DELETE /api/weigh-ins/{week}", s.deleteWeighIn)
}

// weighInJSON is a Weigh-in as the API lists it.
type weighInJSON struct {
	Week model.Day `json:"week"`
	Kg   float64   `json:"kg"`
}

// listWeighIns lists every Weigh-in, newest first.
func (s *WeighInsService) listWeighIns(w http.ResponseWriter, r *http.Request) {
	weighIns, err := s.weighIns.List(r.Context())
	if err != nil {
		ServerError(w, err)
		return
	}
	list := make([]weighInJSON, len(weighIns))
	for i, weighIn := range weighIns {
		list[i] = weighInJSON{Week: weighIn.Week, Kg: weighIn.Kg}
	}
	WriteJSON(w, http.StatusOK, list)
}

// putWeighIn logs the Weigh-in for the week starting on the Monday in the
// path, replacing any earlier one for that week.
func (s *WeighInsService) putWeighIn(w http.ResponseWriter, r *http.Request) {
	week, ok := s.editableWeek(w, r)
	if !ok {
		return
	}
	var body struct {
		Kg *float64 `json:"kg"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Kg == nil {
		http.Error(w, `the body isn't {"kg": number}`, http.StatusBadRequest)
		return
	}
	kg, err := toTenths(*body.Kg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	weighIn := model.WeighIn{Week: week, Kg: kg}
	if err := s.weighIns.Save(r.Context(), weighIn); err != nil {
		ServerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// toTenths checks that kg is a weight to at most 1 decimal and returns it
// rounded to exactly that, as 72.4 may arrive as 72.40000000000001.
func toTenths(kg float64) (float64, error) {
	tenths := math.Round(kg * 10)
	if math.Abs(kg*10-tenths) > 1e-6 || tenths <= 0 || tenths > 5000 {
		return 0, errors.New("kg must be more than 0 and at most 500, to at most 1 decimal")
	}
	return tenths / 10, nil
}

// editableWeek returns the week in r's path, the Monday it starts on, if its
// Weigh-in can be changed: only this week's and last week's can. Otherwise it
// answers r and returns false.
func (s *WeighInsService) editableWeek(w http.ResponseWriter, r *http.Request) (model.Day, bool) {
	week, err := model.ParseDay(r.PathValue("week"))
	if err != nil || week.Week() != week {
		http.Error(w, "the week must be the Monday it starts on, as YYYY-MM-DD", http.StatusBadRequest)
		return "", false
	}
	today, err := s.settings.Today(r.Context())
	if err != nil {
		ServerError(w, err)
		return "", false
	}
	if thisWeek := today.Week(); week != thisWeek && week != thisWeek.AddDays(-7) {
		http.Error(w, "only this week's and last week's Weigh-ins can be changed", http.StatusConflict)
		return "", false
	}
	return week, true
}

// deleteWeighIn deletes the Weigh-in for the week starting on the Monday in
// the path. Deleting one that doesn't exist also succeeds.
func (s *WeighInsService) deleteWeighIn(w http.ResponseWriter, r *http.Request) {
	week, ok := s.editableWeek(w, r)
	if !ok {
		return
	}
	if err := s.weighIns.Delete(r.Context(), week); err != nil {
		ServerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LatestWeighIn is the Weigh-in for the newest week that has one, or
// ErrNotFound if there are none.
func (s *WeighInsService) LatestWeighIn(ctx context.Context) (model.WeighIn, error) {
	return s.weighIns.FindLatest(ctx)
}
