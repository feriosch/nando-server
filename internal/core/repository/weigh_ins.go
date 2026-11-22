package repository

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/core/model"
)

// NewWeighIns returns the WeighInsRepository over the weigh_ins collection in
// db, which Core kept until Weigh-ins moved to Calories (ADR 0012).
func NewWeighIns(db *mongo.Database) model.WeighInsRepository {
	return weighIns{collection: db.Collection("weigh_ins")}
}

// weighIns is the weigh_ins collection.
type weighIns struct {
	collection *mongo.Collection
}

func (r weighIns) List(ctx context.Context) ([]model.WeighIn, error) {
	cursor, err := r.collection.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: -1}}))
	if err != nil {
		return nil, err
	}
	list := []model.WeighIn{}
	return list, cursor.All(ctx, &list)
}
