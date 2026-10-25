package service

import (
	"context"
	"math"
	"net/http"

	"github.com/feriosch/nando-server/internal/core/model"
)

// round1 rounds x to 1 decimal.
func round1(x float64) float64 {
	return math.Round(x*10) / 10
}

// bmi is the body mass index for kg at heightCm, to 1 decimal. It is always
// derived, never stored.
func bmi(kg float64, heightCm int) float64 {
	m := float64(heightCm) / 100
	return round1(kg / (m * m))
}

// BMI is the latest Weigh-in's body mass index to 1 decimal, or ErrNotFound
// until there is a Weigh-in and the height is set.
func (s *Service) BMI(ctx context.Context) (float64, error) {
	latest, err := s.LatestWeighIn(ctx)
	if err != nil {
		return 0, err
	}
	settings, err := s.settings(ctx)
	if err != nil {
		return 0, err
	}
	if settings.HeightCm == 0 {
		return 0, model.ErrNotFound
	}
	return bmi(latest.Kg, settings.HeightCm), nil
}

// getBody answers the Body Home Card: the latest Weigh-in, its BMI (once the
// height is set) and its change since the Weigh-in before it, however many
// weeks earlier that was.
func (s *Service) getBody(w http.ResponseWriter, r *http.Request) {
	var card struct {
		Latest *weighInJSON `json:"latest"`
		BMI    *float64     `json:"bmi"`
		Change *float64     `json:"change"`
	}
	weighIns, err := s.repo.ListWeighIns(r.Context(), 2)
	if err != nil {
		serverError(w, err)
		return
	}
	if len(weighIns) == 0 {
		writeJSON(w, card)
		return
	}
	latest := weighIns[0]
	latestJSON := toWeighInJSON(latest)
	card.Latest = &latestJSON

	settings, err := s.settings(r.Context())
	if err != nil {
		serverError(w, err)
		return
	}
	if settings.HeightCm != 0 {
		b := bmi(latest.Kg, settings.HeightCm)
		card.BMI = &b
	}
	if len(weighIns) == 2 {
		change := round1(latest.Kg - weighIns[1].Kg)
		card.Change = &change
	}
	writeJSON(w, card)
}
