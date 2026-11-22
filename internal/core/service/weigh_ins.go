package service

import (
	"context"

	"github.com/feriosch/nando-server/internal/core/model"
)

// WeighInsService answers what Core kept of Weigh-ins until they moved to
// Calories (ADR 0012), for Calories to take once. It serves no routes.
type WeighInsService struct {
	weighIns model.WeighInsRepository
	settings *SettingsService
}

// NewWeighIns returns the WeighInsService.
func NewWeighIns(weighIns model.WeighInsRepository, settings *SettingsService) *WeighInsService {
	return &WeighInsService{weighIns: weighIns, settings: settings}
}

// WeighInsLeft is every Weigh-in Core kept, newest first, and the Weigh-in
// reminder in the Owner's settings, if they ever saved one.
func (s *WeighInsService) WeighInsLeft(ctx context.Context) (model.WeighInsLeft, error) {
	weighIns, err := s.weighIns.List(ctx)
	if err != nil {
		return model.WeighInsLeft{}, err
	}
	settings, err := s.settings.Settings(ctx)
	if err != nil {
		return model.WeighInsLeft{}, err
	}
	left := model.WeighInsLeft{WeighIns: weighIns}
	if settings.WeighInReminder.Time != "" {
		left.Reminder = &settings.WeighInReminder
	}
	return left, nil
}
