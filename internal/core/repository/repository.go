// Package repository is Core's data access: the only part of Core that touches
// the database, and only Core's own database.
package repository

import (
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/feriosch/nando-server/internal/core/model"
)

// New returns the CoreRepository over Core's database.
func New(db *mongo.Database) model.CoreRepository {
	return mongoRepository{db: db}
}

type mongoRepository struct {
	db *mongo.Database
}
