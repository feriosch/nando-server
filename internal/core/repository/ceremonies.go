package repository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/core/utils"
)

// NewCeremonies returns the CeremoniesRepository over the ceremonies
// collection in db, first creating any indexes it lacks.
func NewCeremonies(ctx context.Context, db *mongo.Database) (model.CeremoniesRepository, error) {
	collection, err := expiringCollection(ctx, db, "ceremonies")
	if err != nil {
		return nil, err
	}
	return ceremonies{collection: collection}, nil
}

// ceremonies is the ceremonies collection.
type ceremonies struct {
	collection *mongo.Collection
}

func (r ceremonies) Insert(ctx context.Context, ceremony model.Ceremony) error {
	_, err := r.collection.InsertOne(ctx, ceremony)
	return err
}

func (r ceremonies) Take(ctx context.Context, challenge string, now time.Time) (model.Ceremony, error) {
	var ceremony model.Ceremony
	err := r.collection.FindOneAndDelete(ctx, bson.D{{Key: "_id", Value: challenge}, unexpired(now)}).Decode(&ceremony)
	return ceremony, utils.NotFound(err)
}
