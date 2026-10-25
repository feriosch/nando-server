// Package repository is Core's data access: the only part of Core that touches
// the database, and only Core's own database.
package repository

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/core/model"
)

// New returns the CoreRepository over Core's database, first creating any
// indexes it lacks.
func New(ctx context.Context, db *mongo.Database) (model.CoreRepository, error) {
	indexes := map[string][]mongo.IndexModel{
		enrollmentCodes: enrollmentCodeIndexes,
		ceremonies:      ceremonyIndexes,
		sessions:        sessionIndexes,
	}
	for collection, models := range indexes {
		if _, err := db.Collection(collection).Indexes().CreateMany(ctx, models); err != nil {
			return nil, err
		}
	}
	return mongoRepository{db: db}, nil
}

type mongoRepository struct {
	db *mongo.Database
}

// expiresAtIndex has MongoDB delete each document within about a minute after
// its expiresAt. Queries match only unexpired documents, so none can be used
// in between.
var expiresAtIndex = mongo.IndexModel{
	Keys:    bson.D{{Key: "expiresAt", Value: 1}},
	Options: options.Index().SetExpireAfterSeconds(0),
}

// unexpired matches documents whose expiresAt is after now.
func unexpired(now time.Time) bson.E {
	return bson.E{Key: "expiresAt", Value: bson.D{{Key: "$gt", Value: now}}}
}

// notFound turns the driver's "no documents" into model.ErrNotFound.
func notFound(err error) error {
	if errors.Is(err, mongo.ErrNoDocuments) {
		return model.ErrNotFound
	}
	return err
}
