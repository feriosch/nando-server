// Package repository is the Calories Module's data access: the only part of the
// Module that touches the database, and only the Module's own database.
package repository

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/feriosch/nando-server/internal/calories/model"
)

// New returns the CaloriesRepository over the Calories database, first
// creating any indexes it lacks.
func New(ctx context.Context, db *mongo.Database) (model.CaloriesRepository, error) {
	if _, err := db.Collection(entries).Indexes().CreateMany(ctx, entryIndexes); err != nil {
		return nil, err
	}
	return mongoRepository{db: db}, nil
}

type mongoRepository struct {
	db *mongo.Database
}

// matched turns an update that matched no document into model.ErrNotFound.
func matched(result *mongo.UpdateResult, err error) error {
	if err == nil && result.MatchedCount == 0 {
		return model.ErrNotFound
	}
	return err
}

// notFound turns the driver's "no documents" into model.ErrNotFound.
func notFound(err error) error {
	if errors.Is(err, mongo.ErrNoDocuments) {
		return model.ErrNotFound
	}
	return err
}
