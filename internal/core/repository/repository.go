// Package repository is Core's data access: the only part of Core that touches
// the database, and only Core's own database. Each file holds one collection's
// repository; this one holds what several of them share.
package repository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

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

// expiringCollection returns the named collection in db, first creating its
// expiresAtIndex and any other indexes it lacks.
func expiringCollection(ctx context.Context, db *mongo.Database, name string, indexes ...mongo.IndexModel) (*mongo.Collection, error) {
	collection := db.Collection(name)
	_, err := collection.Indexes().CreateMany(ctx, append([]mongo.IndexModel{expiresAtIndex}, indexes...))
	return collection, err
}
