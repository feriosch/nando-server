// Package repository is the Calories Module's data access: the only part of the
// Module that touches the database, and only the Module's own database.
package repository

import (
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/feriosch/nando-server/internal/calories/model"
)

// New returns the CaloriesRepository over the Calories database.
func New(db *mongo.Database) model.CaloriesRepository {
	return mongoRepository{db: db}
}

type mongoRepository struct {
	db *mongo.Database
}
