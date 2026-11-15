// Package career is the Module for keeping up with the Opportunities that
// Recruiters bring the Owner, from the first message until each one closes.
//
// The Module is split in three layers: model declares its data and one
// repository interface per collection, service handles requests, Tools and
// validation, and repository holds all database code. The services see only
// the repository and CoreService interfaces, never a database.
package career

import (
	"context"
	"net/http"
	"slices"

	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/feriosch/nando-server/internal/career/repository"
	"github.com/feriosch/nando-server/internal/career/service"
	coremodel "github.com/feriosch/nando-server/internal/core/model"
)

// Module is the Career Module. Its only access to data is its own database
// and Core's service.
type Module struct {
	settings *service.SettingsService
	syncs    *service.SyncsService
}

// Config is what the Module needs besides its database and Core.
type Config struct{}

// NewCareerModule returns the Career Module reading and writing its own data
// through db and asking Core for Core's data through core.
func NewCareerModule(ctx context.Context, db *mongo.Database, core coremodel.CoreService, cfg Config) (*Module, error) {
	return &Module{
		settings: service.NewSettings(repository.NewSettings(db)),
		syncs:    service.NewSyncs(core),
	}, nil
}

// Routes adds the Module's routes to mux, all under /api/career/.
func (m *Module) Routes(mux *http.ServeMux) {
	m.settings.Routes(mux)
}

// ToolProvider gathers the Module's Tools, for Core to serve to Connectors.
func (m *Module) ToolProvider() coremodel.ToolProvider {
	return toolProvider{m}
}

// toolProvider is the Module's Tools, from each of its services.
type toolProvider struct{ m *Module }

func (p toolProvider) Tools() []coremodel.Tool {
	return slices.Concat(p.m.settings.Tools(), p.m.syncs.Tools())
}

func (p toolProvider) ToolsDescription() string {
	return "see whether you are Looking for a new role"
}
