// Package travel is the Module that watches flight prices for the Owner's
// Trips, through the Travel Assistant (ADR 0013).
//
// The Module is split in three layers: model declares its data and one
// repository interface per collection, service handles Tools and validation,
// and repository holds all database code. The services see only the
// repository and CoreService interfaces, never a database.
package travel

import (
	"context"
	"net/http"
	"slices"

	"go.mongodb.org/mongo-driver/v2/mongo"

	coremodel "github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/travel/repository"
	"github.com/feriosch/nando-server/internal/travel/service"
)

// Module is the Travel Module. Its only access to data is its own database
// and Core's service.
type Module struct {
	settings *service.SettingsService
	trips    *service.TripsService
	fares    *service.FaresService
}

// Config is what the Module needs besides its database and Core.
type Config struct{}

// NewTravelModule returns the Travel Module reading and writing its own data
// through db and asking Core for Core's data through core.
func NewTravelModule(ctx context.Context, db *mongo.Database, core coremodel.CoreService, _ Config) (*Module, error) {
	fares, err := repository.NewFares(ctx, db)
	if err != nil {
		return nil, err
	}
	settings := service.NewSettings(repository.NewSettings(db))
	trips := repository.NewTrips(db)
	return &Module{
		settings: settings,
		trips:    service.NewTrips(trips, fares, settings, core),
		fares:    service.NewFares(repository.NewFareChecks(db), fares, trips, core),
	}, nil
}

// Routes adds the Module's routes to mux, all under /api/travel/. It has none
// yet: the Travel Assistant works through its Tools.
func (m *Module) Routes(*http.ServeMux) {}

// Assistant is the Module's Assistant, for Core to receive the Owner's
// messages to it in the Messenger (ADR 0008, ADR 0013).
func (m *Module) Assistant() coremodel.Assistant {
	return coremodel.Assistant{Name: "travel"}
}

// ToolProvider is what the Module offers Connectors: its Tools, for
// Core.AddToolProvider.
func (m *Module) ToolProvider() coremodel.ToolProvider {
	return toolProvider{m}
}

// toolProvider gathers the Tools of the Module's services.
type toolProvider struct{ m *Module }

// ToolsDescription says what the Module's Tools let a Connector do, for the
// consent page.
func (p toolProvider) ToolsDescription() string {
	return "read and change your Travel settings, set up, change, close and delete your Trips, and record and read the Fares found for them"
}

func (p toolProvider) Tools() []coremodel.Tool {
	return slices.Concat(p.m.settings.Tools(), p.m.trips.Tools(), p.m.fares.Tools())
}
