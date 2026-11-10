package service

import (
	"errors"
	"math"
	"net/http"

	"github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/core/utils"
)

// BodyService answers the Body Home Card, which shows Weigh-ins with the
// Owner's height from their settings.
type BodyService struct {
	weighIns model.WeighInsRepository
	settings *SettingsService
}

// NewBody returns the BodyService.
func NewBody(weighIns model.WeighInsRepository, settings *SettingsService) *BodyService {
	return &BodyService{weighIns: weighIns, settings: settings}
}

// Routes adds the Body Home Card's route to mux, /api/body.
func (s *BodyService) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/body", s.getBody)
}

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
func (s *BodyService) getBody(w http.ResponseWriter, r *http.Request) {
	var card model.BodyCardJSON
	latest, err := s.weighIns.FindLatest(r.Context())
	if errors.Is(err, utils.ErrNotFound) {
		utils.WriteJSON(w, http.StatusOK, card)
		return
	}
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	card.Latest = &model.WeighInJSON{Week: latest.Week, Kg: latest.Kg}

	settings, err := s.settings.Settings(r.Context())
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	if settings.HeightCm != 0 {
		b := bmi(latest.Kg, settings.HeightCm)
		card.BMI = &b
	}

	previous, err := s.weighIns.Find(r.Context(), latest.Week.AddDays(-7))
	switch {
	case err == nil:
		change := round1(latest.Kg - previous.Kg)
		card.Change = &change
	case !errors.Is(err, utils.ErrNotFound):
		utils.ServerError(w, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, card)
}
