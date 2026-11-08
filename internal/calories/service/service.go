// Package service handles the Calories Module's requests and validates them.
// It never touches the database; it reaches its own data only through the
// MealsRepository and EntriesRepository, and asks Core for Core's data
// through CoreService.
package service

import (
	"net/http"
	"time"

	"github.com/feriosch/nando-server/internal/calories/model"
	coremodel "github.com/feriosch/nando-server/internal/core/model"
	coreservice "github.com/feriosch/nando-server/internal/core/service"
)

// Service handles the Calories Module's requests.
type Service struct {
	meals   model.MealsRepository
	entries model.EntriesRepository
	core    coremodel.CoreService
	now     func() time.Time
}

// New returns a Service reaching its own data through one repository per
// collection and asking Core through core, with now as its clock (nil means
// time.Now).
func New(meals model.MealsRepository, entries model.EntriesRepository, core coremodel.CoreService, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{meals: meals, entries: entries, core: core, now: now}
}

// Routes adds the Module's routes to mux, all under /api/calories/.
func (s *Service) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/calories/meals", s.listMeals)
	mux.HandleFunc("POST /api/calories/meals", s.postMeal)
	mux.HandleFunc("GET /api/calories/meals/quick", s.quickMeals)
	mux.HandleFunc("PUT /api/calories/meals/{id}", s.putMeal)
	mux.HandleFunc("DELETE /api/calories/meals/{id}", s.deleteMeal)
	mux.HandleFunc("POST /api/calories/meals/{id}/restore", s.restoreMeal)

	mux.HandleFunc("GET /api/calories/days/{day}/entries", s.listEntries)
	mux.HandleFunc("POST /api/calories/days/{day}/entries", s.postEntry)
	mux.HandleFunc("PUT /api/calories/entries/{id}", s.putEntry)
	mux.HandleFunc("DELETE /api/calories/entries/{id}", s.deleteEntry)
}

// writeCreated answers 201 with the new Meal's or Entry's ID.
func writeCreated(w http.ResponseWriter, id string) {
	coreservice.WriteJSON(w, http.StatusCreated, struct {
		ID string `json:"id"`
	}{id})
}
