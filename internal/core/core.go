// Package core is the shared foundation every Module relies on.
//
// Core is split like a Module: model declares its data and interfaces, service
// handles requests and validation, and repository holds all database code.
// Unlike a Module's, Core's service is also handed to every Module's service.
package core

import (
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/core/repository"
	"github.com/feriosch/nando-server/internal/core/service"
)

// Core is Nando's Core. Its only access to data is its own database.
type Core struct {
	service *service.Service
}

// NewCore returns Core reading and writing through db, Core's own database.
func NewCore(db *mongo.Database) *Core {
	return &Core{service: service.New(repository.New(db))}
}

// Service is Core's service, which Modules' services call for Core's data.
func (c *Core) Service() model.CoreService {
	return c.service
}
