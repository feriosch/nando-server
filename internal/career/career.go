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
	"errors"
	"net/http"
	"slices"

	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/feriosch/nando-server/internal/career/repository"
	"github.com/feriosch/nando-server/internal/career/service"
	coremodel "github.com/feriosch/nando-server/internal/core/model"
	coreutils "github.com/feriosch/nando-server/internal/core/utils"
)

// Module is the Career Module. Its only access to data is its own database
// and Core's service.
type Module struct {
	settings *service.SettingsService
	syncs    *service.SyncsService
	work     *service.WorkService
	// notifications is the Module's Notification provider.
	notifications *service.NotificationsService
	// tickRules apply the rules that need no model at every tick.
	tickRules *service.TickRulesService
	// conversations sorts Conversations: ignores or files them.
	conversations *service.ConversationsService
	opportunities *service.OpportunitiesService
	companies     *service.CompaniesService
	search        *service.SearchService
	profile       *service.ProfileService
	recruiters    *service.RecruitersService
	overview      *service.OverviewService
	gmailSync     *service.GmailCareerSyncService
}

// Config is what the Module needs besides its database and Core.
type Config struct {
	// SyncTriggerURL and SyncTriggerToken are the API trigger of the
	// scheduled task whose runs work through what Syncs bring, and its
	// bearer token. An empty one means none: Claude is never woken.
	SyncTriggerURL   string
	SyncTriggerToken string
}

// NewCareerModule returns the Career Module reading and writing its own data
// through db and asking Core for Core's data through core.
func NewCareerModule(ctx context.Context, db *mongo.Database, core coremodel.CoreService, cfg Config) (*Module, error) {
	conversations, err := repository.NewConversations(ctx, db)
	if err != nil {
		return nil, err
	}
	work, err := repository.NewWork(ctx, db)
	if err != nil {
		return nil, err
	}
	settings, syncs, recruiters := repository.NewSettings(db), repository.NewSyncs(db), repository.NewRecruiters(db)
	opportunities, companies := repository.NewOpportunities(db), repository.NewCompanies(db)
	profile := repository.NewProfile(db)
	scheduledTask := coreutils.NewScheduledTask(repository.NewScheduledTask(db), service.WorkQueue(work),
		coreutils.Trigger{URL: cfg.SyncTriggerURL, Token: cfg.SyncTriggerToken, Env: "CAREER_SYNC_TRIGGER"}, core.Now)
	settingsService := service.NewSettings(settings)
	opportunitiesService := service.NewOpportunities(opportunities, companies, recruiters, core)
	return &Module{
		settings:      settingsService,
		syncs:         service.NewSyncs(scheduledTask, syncs, conversations, recruiters, work, core),
		notifications: service.NewNotifications(settingsService, conversations, opportunities, companies, syncs, work, core.Now),
		tickRules:     service.NewTickRules(opportunities, conversations, work, core.Now),
		work:          service.NewWork(scheduledTask, work, conversations, recruiters, opportunitiesService, settingsService, profile, syncs, core.Now),
		conversations: service.NewConversations(scheduledTask, conversations, recruiters, work, core.Now),
		opportunities: opportunitiesService,
		companies:     service.NewCompanies(companies, opportunities, opportunitiesService),
		search:        service.NewSearch(recruiters, conversations, companies, opportunities, opportunitiesService),
		profile:       service.NewProfile(profile),
		recruiters:    service.NewRecruiters(recruiters, conversations),
		overview:      service.NewOverview(conversations, recruiters, opportunitiesService, settingsService),
		gmailSync:     service.NewGmailCareerSync(repository.NewGmailPasses(db), core.Now),
	}, nil
}

// Routes adds the Module's routes to mux, all under /api/career/.
func (m *Module) Routes(mux *http.ServeMux) {
	m.settings.Routes(mux)
	m.opportunities.Routes(mux)
	m.recruiters.Routes(mux)
	m.conversations.Routes(mux)
	m.overview.Routes(mux)
}

// NotificationProvider is the Module's Notification provider, for
// Core.AddNotificationProvider.
func (m *Module) NotificationProvider() coremodel.NotificationProvider {
	return m.notifications
}

// TickJob is what the Module does at every tick, for Core.AddTickJob: it
// applies the rules that need no model, such as closing faded Opportunities,
// sets aside the Items no run could do, and wakes Claude for those still
// waiting, including any the rules just queued.
func (m *Module) TickJob() coremodel.TickJob {
	return tickJob{m}
}

// tickJob is the Module's work at every tick.
type tickJob struct{ m *Module }

// RunAtTick applies the rules first, so the run is woken for what they
// queue. A rule that fails doesn't stop the run's work, nor it the rules.
func (j tickJob) RunAtTick(ctx context.Context) error {
	return errors.Join(j.m.tickRules.Apply(ctx), j.m.work.RunAtTick(ctx))
}

// Assistant is the Module's Assistant, for Core to receive the Owner's
// messages to it in the Messenger (ADR 0008).
func (m *Module) Assistant() coremodel.Assistant {
	return coremodel.Assistant{Name: "career"}
}

// ToolProvider gathers the Module's Tools, for Core to serve to Connectors.
func (m *Module) ToolProvider() coremodel.ToolProvider {
	return toolProvider{m}
}

// toolProvider is the Module's Tools, from each of its services.
type toolProvider struct{ m *Module }

func (p toolProvider) Tools() []coremodel.Tool {
	return slices.Concat(p.m.settings.Tools(), p.m.syncs.Tools(), p.m.work.Tools(), p.m.conversations.Tools(), p.m.opportunities.Tools(), p.m.companies.Tools(), p.m.search.Tools(), p.m.profile.Tools(), p.m.gmailSync.Tools())
}

func (p toolProvider) ToolsDescription() string {
	return "see whether you are Looking for a new role, record the LinkedIn messages your Mac's Sync reads, " +
		"sort your Conversations under Recruiters, keep your Opportunities, their Steps and Companies, " +
		"write Suggested Replies, read and change your Career Profile, file the Gmail threads you labeled Career, " +
		"and delete Steps and Opportunities, which can't be undone"
}
