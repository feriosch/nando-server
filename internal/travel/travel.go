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
	verdicts *service.PriceVerdictsService
	overview *service.OverviewService
	// notifications is the Module's Notification provider.
	notifications *service.NotificationsService
}

// Config is what the Module needs besides its database and Core.
type Config struct {
	// SerpAPIURL is SerpApi, whose Google Flights search is at /search.json;
	// empty means https://serpapi.com.
	SerpAPIURL string
	// SerpAPIKey is SerpApi's key; empty turns Price Verdicts off.
	SerpAPIKey string
}

// NewTravelModule returns the Travel Module reading and writing its own data
// through db and asking Core for Core's data through core.
func NewTravelModule(ctx context.Context, db *mongo.Database, core coremodel.CoreService, cfg Config) (*Module, error) {
	if cfg.SerpAPIURL == "" {
		cfg.SerpAPIURL = "https://serpapi.com"
	}
	fares, err := repository.NewFares(ctx, db)
	if err != nil {
		return nil, err
	}
	settings := service.NewSettings(repository.NewSettings(db))
	trips := repository.NewTrips(db)
	alerts := repository.NewFareAlerts(db)
	verdicts, err := repository.NewPriceVerdicts(ctx, db)
	if err != nil {
		return nil, err
	}
	calls, err := repository.NewSerpAPICalls(ctx, db)
	if err != nil {
		return nil, err
	}
	faresService := service.NewFares(repository.NewFareChecks(db), fares, alerts, trips, core)
	return &Module{
		settings:      settings,
		trips:         service.NewTrips(trips, fares, verdicts, alerts, settings, core),
		fares:         faresService,
		overview:      service.NewOverview(trips, verdicts, faresService, settings),
		notifications: service.NewNotifications(settings, trips),
		verdicts:      service.NewPriceVerdicts(verdicts, calls, trips, settings, core, cfg.SerpAPIURL, cfg.SerpAPIKey),
	}, nil
}

// Routes adds the Module's routes to mux, all under /api/travel/: the web
// app's reads. The Travel Assistant writes, through its Tools.
func (m *Module) Routes(mux *http.ServeMux) {
	m.overview.Routes(mux)
}

// NotificationProvider is the Module's Notification provider, for Core to ask
// at every tick: it asks the Travel Assistant for Fare Checks.
func (m *Module) NotificationProvider() coremodel.NotificationProvider {
	return m.notifications
}

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
	return "read and change your Travel settings, set up, change, close and delete your Trips, record and read the Fares found for them and the Fare Alerts sent, set Target Fares, and fetch Google's Price Verdicts on them"
}

func (p toolProvider) Tools() []coremodel.Tool {
	return slices.Concat(p.m.settings.Tools(), p.m.trips.Tools(), p.m.fares.Tools(), p.m.verdicts.Tools())
}
