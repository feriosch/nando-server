// Package money is the Module that keeps the Owner's money across Japan and
// Mexico, from the Statements the Owner sends the Money Assistant (ADR 0014).
//
// The Module is split in three layers: model declares its data and one
// repository interface per collection, service handles Tools and validation,
// and repository holds all database code. The services see only the
// repository and CoreService interfaces, never a database.
package money

import (
	"context"
	"net/http"
	"slices"

	"go.mongodb.org/mongo-driver/v2/mongo"

	coremodel "github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/money/repository"
	"github.com/feriosch/nando-server/internal/money/service"
)

// Module is the Money Module. Its only access to data is its own database
// and Core's service.
type Module struct {
	settings      *service.SettingsService
	accounts      *service.AccountsService
	statements    *service.StatementsService
	categories    *service.CategoriesService
	merchants     *service.MerchantsService
	subscriptions *service.SubscriptionsService
}

// Config is what the Module needs besides its database and Core.
type Config struct {
	// StatementsBucket is the Cloud Storage bucket every Statement's file is
	// kept in (ADR 0014); empty keeps no files, as locally.
	StatementsBucket string
}

// NewMoneyModule returns the Money Module reading and writing its own data
// through db and asking Core for Core's data through core.
func NewMoneyModule(ctx context.Context, db *mongo.Database, core coremodel.CoreService, _ Config) (*Module, error) {
	statements, err := repository.NewStatements(ctx, db)
	if err != nil {
		return nil, err
	}
	merchantsRepo, err := repository.NewMerchants(ctx, db)
	if err != nil {
		return nil, err
	}
	subscriptionsRepo := repository.NewSubscriptions(db)
	accounts := service.NewAccounts(repository.NewAccounts(db), statements)
	categories := service.NewCategories(repository.NewSubcategories(db), statements, merchantsRepo, subscriptionsRepo)
	merchants := service.NewMerchants(merchantsRepo, categories)
	return &Module{
		settings:      service.NewSettings(repository.NewSettings(db)),
		accounts:      accounts,
		statements:    service.NewStatements(statements, accounts, categories, merchants),
		categories:    categories,
		merchants:     merchants,
		subscriptions: service.NewSubscriptions(subscriptionsRepo, merchants, categories, core),
	}, nil
}

// Routes adds the Module's routes to mux, all under /api/money/: the web
// app's reads. The Money Assistant writes, through its Tools.
func (m *Module) Routes(*http.ServeMux) {}

// Assistant is the Module's Assistant, for Core to receive the Owner's
// messages to it in the Messenger (ADR 0008, ADR 0014).
func (m *Module) Assistant() coremodel.Assistant {
	return coremodel.Assistant{Name: "money"}
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
	return "read and change your Money settings and Accounts, save, read and delete your Statements and their Transactions, and keep what each Transaction is: its Category and Subcategory, its Merchant, and your Subscriptions"
}

func (p toolProvider) Tools() []coremodel.Tool {
	return slices.Concat(p.m.settings.Tools(), p.m.accounts.Tools(), p.m.statements.Tools(), p.m.categories.Tools(),
		p.m.merchants.Tools(), p.m.subscriptions.Tools())
}
