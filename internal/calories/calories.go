// Package calories is the Module for tracking what the Owner eats each Day.
//
// The Module is split in three layers: model declares its data and one
// repository interface per collection, service handles requests and
// validation (MealsService and EntriesService), and repository holds all
// database code. The services see only the MealsRepository,
// EntriesRepository and CoreService interfaces, never a database.
package calories

import (
	"context"
	"net/http"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/feriosch/nando-server/internal/calories/repository"
	"github.com/feriosch/nando-server/internal/calories/service"
	coremodel "github.com/feriosch/nando-server/internal/core/model"
)

// Module is the Calories Module. Its only access to data is its own database
// and Core's service.
type Module struct {
	meals   *service.MealsService
	entries *service.EntriesService
}

// Config is what the Module needs besides its database and Core.
type Config struct {
	// Now is the clock; nil means time.Now.
	Now func() time.Time
}

// NewCaloriesModule returns the Calories Module reading and writing its own
// data through db and asking Core for Core's data through core.
func NewCaloriesModule(ctx context.Context, db *mongo.Database, core coremodel.CoreService, cfg Config) (*Module, error) {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	entries, err := repository.NewEntries(ctx, db)
	if err != nil {
		return nil, err
	}
	meals := repository.NewMeals(db)
	return &Module{
		meals:   service.NewMeals(meals, entries, core, now),
		entries: service.NewEntries(entries, meals, core, now),
	}, nil
}

// Routes adds the Module's routes to mux, all under /api/calories/.
func (m *Module) Routes(mux *http.ServeMux) {
	m.meals.Routes(mux)
	m.entries.Routes(mux)
}
