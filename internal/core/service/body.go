package service

import (
	"errors"
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

// getBody answers the Body Home Card: the latest Weigh-in, its BMI (once the
// height is set) and its change since the week before (if that week has one).
func (s *Service) getBody(w http.ResponseWriter, r *http.Request) {
	var card struct {
		Latest *weighInJSON `json:"latest"`
		BMI    *float64     `json:"bmi"`
		Change *float64     `json:"change"`
	}
	latest, err := s.repo.FindLatestWeighIn(r.Context())
	if errors.Is(err, model.ErrNotFound) {
		writeJSON(w, card)
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	card.Latest = &weighInJSON{Week: latest.Week, Kg: latest.Kg}

	settings, err := s.settings(r.Context())
	if err != nil {
		serverError(w, err)
		return
	}
	if settings.HeightCm != 0 {
		b := bmi(latest.Kg, settings.HeightCm)
		card.BMI = &b
	}

	previous, err := s.repo.FindWeighIn(r.Context(), latest.Week.AddDays(-7))
	switch {
	case err == nil:
		change := round1(latest.Kg - previous.Kg)
		card.Change = &change
	case !errors.Is(err, model.ErrNotFound):
		serverError(w, err)
		return
	}
	writeJSON(w, card)
}
