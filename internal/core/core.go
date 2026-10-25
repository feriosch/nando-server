// Package core is the shared foundation every Module relies on.
//
// Core is split like a Module: model declares its data and interfaces, service
// handles requests and validation, and repository holds all database code.
// Unlike a Module's, Core's service is also handed to every Module's service.
package core

import (
	"context"
	"net/http"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/core/repository"
	"github.com/feriosch/nando-server/internal/core/service"
)

// Config is what Core needs to know about where Nando is served.
type Config struct {
	// Domain is the parent domain of the web app and the API, such as
	// "nanders.app": the passkeys' relying-party ID and the session cookie's domain.
	Domain string
	// Origins are the web app origins passkeys are used from.
	Origins []string
	// Now is the clock; nil means time.Now.
	Now func() time.Time
}

// Core is Nando's Core. Its only access to data is its own database.
type Core struct {
	service *service.Service
}

// New returns Core reading and writing through db, Core's own database.
func New(ctx context.Context, db *mongo.Database, cfg Config) (*Core, error) {
	repo, err := repository.New(ctx, db)
	if err != nil {
		return nil, err
	}
	svc, err := service.New(repo, service.Config{
		Domain:  cfg.Domain,
		Origins: cfg.Origins,
		Now:     cfg.Now,
	})
	if err != nil {
		return nil, err
	}
	return &Core{service: svc}, nil
}

// IssueEnrollmentCode stores and returns a new Enrollment Code, valid once
// within the next 15 minutes. It is what `nando-server enroll` prints.
func (c *Core) IssueEnrollmentCode(ctx context.Context) (string, error) {
	return c.service.IssueEnrollmentCode(ctx)
}

// Routes adds Core's routes to mux: signing in under /auth/, and everything
// else under /api/, which must be served behind RequireSession.
func (c *Core) Routes(mux *http.ServeMux) {
	c.service.Routes(mux)
}

// Service is Core's service, which Modules' services call for Core's data.
func (c *Core) Service() model.CoreService {
	return c.service
}

// RequireSession lets a request through to next only if it carries a valid
// session; otherwise it answers 401.
func (c *Core) RequireSession(next http.Handler) http.Handler {
	return c.service.RequireSession(next)
}
