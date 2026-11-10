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
	// VAPID signs every push message.
	VAPID VAPID
}

// VAPID is the key pair and contact that sign every push message.
type VAPID = service.VAPID

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
	identity          *service.IdentityService
	sessions          *service.SessionsService
	passkeys          *service.PasskeysService
	settings          *service.SettingsService
	weighIns          *service.WeighInsService
	body              *service.BodyService
	pushSubscriptions *service.PushSubscriptionsService
	notifications     *service.NotificationsService
	mcp               *service.MCPService
	connectors        *service.ConnectorsService
	oauth             *service.OAuthService
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
	passkeys := repository.NewPasskeys(db)
	weighIns := repository.NewWeighIns(db)
	pushSubscriptions := repository.NewPushSubscriptions(db)

	// apiURL is the API's own URL, which Connectors reach it at, and webURL
	// the web app's.
	apiURL, webURL := "https://api."+cfg.Domain, "https://web."+cfg.Domain
	c := &Core{
		sessions:          service.NewSessions(sessionsRepo, cfg.Domain, now),
		passkeys:          service.NewPasskeys(passkeys),
		settings:          service.NewSettings(repository.NewSettings(db), now),
		pushSubscriptions: service.NewPushSubscriptions(pushSubscriptions, cfg.VAPID.PublicKey, now),
		mcp:               service.NewMCP(connectors, apiURL, now),
		connectors:        service.NewConnectors(connectors),
		oauth:             service.NewOAuth(oauthClients, authorizationRequests, connectors, authorizationCodes, apiURL, webURL, now),
	}
	c.weighIns = service.NewWeighIns(weighIns, c.settings)
	c.coreService = coreService{settings: c.settings, weighIns: c.weighIns}
	c.body = service.NewBody(weighIns, c.settings)
	c.identity, err = service.NewIdentity(enrollmentCodes, ceremonies, passkeys, c.sessions, c.oauth, cfg.Domain, cfg.Origins, now)
	if err != nil {
		return nil, err
	}
	c.notifications, err = service.NewNotifications(pushSubscriptions, sentNotifications, c.settings,
		cfg.VerifySchedulerToken, cfg.VAPID, now)
	if err != nil {
		return nil, err
	}
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
	c.weighIns.Routes(mux)
	c.body.Routes(mux)
	c.pushSubscriptions.Routes(mux)
	c.notifications.Routes(mux)
	c.mcp.Routes(mux)
	c.connectors.Routes(mux)
	c.oauth.Routes(mux)
}

// Service is Core's service, which Modules' services call for Core's data.
func (c *Core) Service() model.CoreService {
	return c.coreService
}

// coreService is model.CoreService: what Modules may ask of Core, answered by
// the services whose data it is.
type coreService struct {
	settings *service.SettingsService
	weighIns *service.WeighInsService
}

func (s coreService) OwnerTimeZone(ctx context.Context) (*time.Location, error) {
	return s.settings.OwnerTimeZone(ctx)
}

func (s coreService) Today(ctx context.Context) (model.Day, error) {
	return s.settings.Today(ctx)
}

func (s coreService) LatestWeighIn(ctx context.Context) (model.WeighIn, error) {
	return s.weighIns.LatestWeighIn(ctx)
}

// AddNotificationProvider has Core ask p for its due Notifications at every
// tick. Each Module with Notifications adds its provider once, at startup,
// before serving.
func (c *Core) AddNotificationProvider(p model.NotificationProvider) {
	c.notifications.AddNotificationProvider(p)
}

// AddToolProvider has Core serve p's Tools to every Connector at /mcp. Each
// Module with Tools adds its provider once, at startup, before serving.
func (c *Core) AddToolProvider(p model.ToolProvider) {
	c.mcp.AddToolProvider(p)
}

// RequireSession lets a request through to next only if it carries a valid
// Session; otherwise it answers 401.
func (c *Core) RequireSession(next http.Handler) http.Handler {
	return c.sessions.RequireSession(next)
}
