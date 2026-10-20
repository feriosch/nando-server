// Package service handles the Calories Module's requests and validates them.
// It never touches the database; it reaches its own data only through
// CaloriesRepository, and asks Core for Core's data through CoreService.
package service

import (
	"github.com/feriosch/nando-server/internal/calories/model"
	coremodel "github.com/feriosch/nando-server/internal/core/model"
)

// Service handles the Calories Module's requests.
type Service struct {
	repo model.CaloriesRepository
	core coremodel.CoreService
}

// New returns a Service reaching its own data through repo and asking Core
// through core.
func New(repo model.CaloriesRepository, core coremodel.CoreService) *Service {
	return &Service{repo: repo, core: core}
}
