package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"

	"github.com/feriosch/nando-server/internal/core/model"
)

// weighInJSON is a Weigh-in as the API lists it.
type weighInJSON struct {
	Week model.Day `json:"week"`
	Kg   float64   `json:"kg"`
}

// listWeighIns lists every Weigh-in, newest first.
func (s *Service) listWeighIns(w http.ResponseWriter, r *http.Request) {
	weighIns, err := s.repo.ListWeighIns(r.Context())
	if err != nil {
		serverError(w, err)
		return
	}
	list := make([]weighInJSON, len(weighIns))
	for i, weighIn := range weighIns {
		list[i] = weighInJSON{Week: weighIn.Week, Kg: weighIn.Kg}
	}
	writeJSON(w, list)
}

// putWeighIn logs the Weigh-in for the week starting on the Monday in the
// path, replacing any earlier one for that week.
func (s *Service) putWeighIn(w http.ResponseWriter, r *http.Request) {
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
	if err := s.repo.SaveWeighIn(r.Context(), weighIn); err != nil {
		serverError(w, err)
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
func (s *Service) editableWeek(w http.ResponseWriter, r *http.Request) (model.Day, bool) {
	week, err := model.ParseDay(r.PathValue("week"))
	if err != nil || week.Week() != week {
		http.Error(w, "the week must be the Monday it starts on, as YYYY-MM-DD", http.StatusBadRequest)
		return "", false
	}
	today, err := s.Today(r.Context())
	if err != nil {
		serverError(w, err)
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
func (s *Service) deleteWeighIn(w http.ResponseWriter, r *http.Request) {
	week, ok := s.editableWeek(w, r)
	if !ok {
		return
	}
	if err := s.repo.DeleteWeighIn(r.Context(), week); err != nil {
		serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LatestWeighIn is the Weigh-in for the newest week that has one, or
// ErrNotFound if there are none.
func (s *Service) LatestWeighIn(ctx context.Context) (model.WeighIn, error) {
	return s.repo.FindLatestWeighIn(ctx)
}
