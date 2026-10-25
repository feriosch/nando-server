package repository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/feriosch/nando-server/internal/core/model"
)

const ceremonies = "ceremonies"

var ceremonyIndexes = []mongo.IndexModel{expiresAtIndex}

func (r mongoRepository) InsertCeremony(ctx context.Context, ceremony model.Ceremony) error {
	_, err := r.db.Collection(ceremonies).InsertOne(ctx, ceremony)
	return err
}

func (r mongoRepository) TakeCeremony(ctx context.Context, challenge string, now time.Time) (model.Ceremony, error) {
	var ceremony model.Ceremony
	err := r.db.Collection(ceremonies).FindOneAndDelete(ctx, bson.D{{Key: "_id", Value: challenge}, unexpired(now)}).Decode(&ceremony)
	return ceremony, notFound(err)
}
