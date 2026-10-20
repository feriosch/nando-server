// Package calories is the Module for tracking what the Owner eats each Day.
//
// The Module is split in three layers: model declares its data and repository
// interface, service handles requests and validation, and repository holds all
// database code. The service sees only the CaloriesRepository and
// CoreService interfaces, never a database.
package calories

import (
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/feriosch/nando-server/internal/calories/repository"
	"github.com/feriosch/nando-server/internal/calories/service"
	coremodel "github.com/feriosch/nando-server/internal/core/model"
)

// Module is the Calories Module. Its only access to data is its own database
// and Core's service.
type Module struct {
	service *service.Service
}

// NewCaloriesModule returns the Calories Module reading and writing its own
// data through db and asking Core for Core's data through core.
func NewCaloriesModule(db *mongo.Database, core coremodel.CoreService) *Module {
	return &Module{service: service.New(repository.New(db), core)}
}
