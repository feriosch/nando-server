// Package core is the shared foundation every Module relies on.
//
// Core is split like a Module: model declares its data and one repository
// interface per collection, service handles requests and validation in one
// service per collection it serves, and repository holds all database code.
// Unlike a Module, Core also hands every Module's service model.CoreService,
// answered by its Settings and Weigh-ins services.
package core

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/api/idtoken"

	"github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/core/repository"
	"github.com/feriosch/nando-server/internal/core/service"
	"github.com/feriosch/nando-server/internal/core/utils"
)

// Config is what Core needs to know about where Nando is served.
type Config struct {
	// Domain is the parent domain of the web app and the API, such as
	// "nanders.app": the Passkeys' relying-party ID and the Session cookie's domain.
	Domain string
	// Origins are the web app origins Passkeys are used from.
	Origins []string
	// Now is the clock; nil means time.Now.
	Now func() time.Time
	// VerifySchedulerToken checks the bearer token of a call to
	// /internal/tick, returning an error unless it is a Google-signed OIDC
	// token from the scheduler's service account. In production it is
	// VerifyGoogleSchedulerToken.
	VerifySchedulerToken func(ctx context.Context, token string) error
	// Messenger is how Nando reaches the Owner in Telegram (ADR 0008).
	Messenger MessengerConfig
}

// MessengerConfig is how Nando reaches the Owner in Telegram: the Owner's
// Telegram user ID, the webhook secret and each Assistant's bot token.
type MessengerConfig = service.MessengerConfig

// VerifyGoogleSchedulerToken returns the production VerifySchedulerToken: it
// accepts only a Google-signed OIDC token issued for audience (as the Cloud
// Scheduler job sets it) to the scheduler's service account.
func VerifyGoogleSchedulerToken(audience, serviceAccount string) func(ctx context.Context, token string) error {
	return func(ctx context.Context, token string) error {
		payload, err := idtoken.Validate(ctx, token, audience)
		if err != nil {
			return err
		}
		email, _ := payload.Claims["email"].(string)
		verified, _ := payload.Claims["email_verified"].(bool)
		if email != serviceAccount || !verified {
			return fmt.Errorf("token is for %q, not the scheduler's service account", email)
		}
		return nil
	}
}

// Core is Nando's Core. Its only access to data is its own database.
type Core struct {
	identity      *service.IdentityService
	sessions      *service.SessionsService
	passkeys      *service.PasskeysService
	settings      *service.SettingsService
	notifications *service.NotificationsService
	mcp           *service.MCPService
	connectors    *service.ConnectorsService
	oauth         *service.OAuthService
	messenger     *service.MessengerService
	gmailFilings  *service.GmailFilingsService
	syncKeys      *service.SyncKeysService
	// coreService is what Modules' services are handed.
	coreService coreService
}

// New returns Core reading and writing through db, Core's own database.
func New(ctx context.Context, db *mongo.Database, cfg Config) (*Core, error) {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	enrollmentCodes, err := repository.NewEnrollmentCodes(ctx, db)
	if err != nil {
		return nil, err
	}
	ceremonies, err := repository.NewCeremonies(ctx, db)
	if err != nil {
		return nil, err
	}
	sessionsRepo, err := repository.NewSessions(ctx, db)
	if err != nil {
		return nil, err
	}
	sentNotifications, err := repository.NewSentNotifications(ctx, db)
	if err != nil {
		return nil, err
	}
	oauthClients, err := repository.NewOAuthClients(ctx, db)
	if err != nil {
		return nil, err
	}
	authorizationRequests, err := repository.NewAuthorizationRequests(ctx, db)
	if err != nil {
		return nil, err
	}
	authorizationCodes, err := repository.NewAuthorizationCodes(ctx, db)
	if err != nil {
		return nil, err
	}
	connectors, err := repository.NewConnectors(ctx, db)
	if err != nil {
		return nil, err
	}
	talks, err := repository.NewTalks(ctx, db)
	if err != nil {
		return nil, err
	}
	gmailFilings, err := repository.NewGmailFilings(ctx, db)
	if err != nil {
		return nil, err
	}
	syncKeys, err := repository.NewSyncKeys(ctx, db)
	if err != nil {
		return nil, err
	}
	passkeys := repository.NewPasskeys(db)

	// apiURL is the API's own URL, which Connectors reach it at, and webURL
	// the web app's.
	apiURL, webURL := "https://api."+cfg.Domain, "https://web."+cfg.Domain
	mcp := service.NewMCP(connectors, apiURL, now)
	c := &Core{
		sessions:   service.NewSessions(sessionsRepo, cfg.Domain, now),
		passkeys:   service.NewPasskeys(passkeys),
		settings:   service.NewSettings(repository.NewSettings(db), now),
		mcp:        mcp,
		connectors: service.NewConnectors(connectors, oauthClients, now),
		syncKeys:   service.NewSyncKeys(syncKeys, now),
		messenger: service.NewMessenger(talks, func(assistant string) utils.FireRecord {
			return repository.NewMessengerFires(db, assistant)
		}, cfg.Messenger, apiURL, now),
		oauth: service.NewOAuth(oauthClients, authorizationRequests, connectors, authorizationCodes, apiURL, webURL, mcp.ToolsDescriptions, now),
	}
	// Without a bot, no Assistant can be answered, so no Connector is
	// offered the Messenger's Tools or told of them.
	// The Filing Record serves the Core Assistant's run alone, so it goes
	// with the Messenger.
	c.gmailFilings = service.NewGmailFilings(gmailFilings, c.settings, now)
	if len(cfg.Messenger.BotTokens) > 0 {
		mcp.AddToolProvider(c.messenger)
		mcp.AddToolProvider(c.gmailFilings)
	}
	c.coreService = coreService{settings: c.settings, messenger: c.messenger, now: now}
	c.identity, err = service.NewIdentity(enrollmentCodes, ceremonies, passkeys, c.sessions, c.oauth, c.syncKeys, cfg.Domain, cfg.Origins, now)
	if err != nil {
		return nil, err
	}
	c.notifications, err = service.NewNotifications(sentNotifications, c.settings, cfg.VerifySchedulerToken, now)
	if err != nil {
		return nil, err
	}
	c.notifications.AddTickJob(c.messenger)
	c.notifications.AddTickJob(c.gmailFilings)
	c.notifications.SetMessenger(c.messenger)
	return c, nil
}

// IssueEnrollmentCode stores and returns a new Enrollment Code, valid once
// within the next 15 minutes. It is what `nando-server enroll` prints.
func (c *Core) IssueEnrollmentCode(ctx context.Context) (string, error) {
	return c.identity.IssueEnrollmentCode(ctx)
}

// Routes adds Core's routes to mux: signing in under /auth/, the tick under
// /internal/, the Tools for Connectors at /mcp with OAuth under /oauth/ and
// /.well-known/, and everything else under /api/, which must be served
// behind RequireSession.
func (c *Core) Routes(mux *http.ServeMux) {
	c.identity.Routes(mux)
	c.sessions.Routes(mux)
	c.passkeys.Routes(mux)
	c.settings.Routes(mux)
	c.notifications.Routes(mux)
	c.mcp.Routes(mux)
	c.connectors.Routes(mux)
	c.oauth.Routes(mux)
	c.messenger.Routes(mux)
	c.syncKeys.Routes(mux)
}

// Service is Core's service, which Modules' services call for Core's data.
func (c *Core) Service() model.CoreService {
	return c.coreService
}

// coreService is model.CoreService: what Modules may ask of Core, answered by
// the services whose data it is.
type coreService struct {
	settings  *service.SettingsService
	messenger *service.MessengerService
	now       func() time.Time
}

func (s coreService) Now() time.Time {
	return s.now()
}

func (s coreService) OwnerTimeZone(ctx context.Context) (*time.Location, error) {
	return s.settings.OwnerTimeZone(ctx)
}

func (s coreService) Today(ctx context.Context) (model.Day, error) {
	return s.settings.Today(ctx)
}

func (s coreService) MessengerFile(ctx context.Context, assistant, id string) ([]byte, error) {
	return s.messenger.Download(ctx, assistant, id)
}

func (s coreService) ReadFile(ctx context.Context, data []byte, pages, lines string) (any, error) {
	return s.messenger.ReadFile(ctx, data, pages, lines)
}

func (s coreService) HeightCm(ctx context.Context) (int, error) {
	settings, err := s.settings.Settings(ctx)
	return settings.HeightCm, err
}

// AddNotificationProvider has Core ask p for its due Notifications at every
// tick. Each Module with Notifications adds its provider once, at startup,
// before serving.
func (c *Core) AddNotificationProvider(p model.NotificationProvider) {
	c.notifications.AddNotificationProvider(p)
}

// AddTickJob has Core run j at every tick, after sending the due
// Notifications. Each Module with work to do at every tick adds its job
// once, at startup, before serving.
func (c *Core) AddTickJob(j model.TickJob) {
	c.notifications.AddTickJob(j)
}

// AddToolProvider has Core serve p's Tools to every Connector at /mcp, and
// tell the consent page what p says they let a Connector do. Each Module with
// Tools adds its provider once, at startup, before serving. It panics if p
// has Tools but no description of them, or if a Tool can't be served: one
// without a name, a handler or an object schema, or whose name another Tool
// already has.
func (c *Core) AddToolProvider(p model.ToolProvider) {
	c.mcp.AddToolProvider(p)
}

// AddAssistant has Core receive the Owner's messages to a, in a's own chat
// in the Messenger. Each Module with an Assistant adds it once, at startup,
// before serving. Core adds the Core Assistant itself.
func (c *Core) AddAssistant(a model.Assistant) {
	c.messenger.AddAssistant(a)
}

// RegisterWebhooks points every Assistant's bot at its webhook. main calls
// it once, after every Module has added its Assistant.
func (c *Core) RegisterWebhooks(ctx context.Context) error {
	return c.messenger.RegisterWebhooks(ctx)
}

// Talk is assistant's open Talk: an empty one if it has none yet.
func (c *Core) Talk(ctx context.Context, assistant string) (model.Talk, error) {
	return c.messenger.Talk(ctx, assistant)
}

// RequireSession lets a request through to next only if it carries a valid
// Session, or, on a route opened with OpenToSyncKeys, a Sync Key; otherwise
// it answers 401.
func (c *Core) RequireSession(next http.Handler) http.Handler {
	return c.syncKeys.Guard(c.sessions.RequireSession(next), next)
}

// OpenToSyncKeys lets a Sync Key call route, written "METHOD /path" with no
// wildcards, such as "POST /api/calories/health-sync" (ADR 0015). Each
// Module opens its routes once, at startup, before serving. A Sync Key
// reaches no other route.
func (c *Core) OpenToSyncKeys(route string) {
	c.syncKeys.Open(route)
}
