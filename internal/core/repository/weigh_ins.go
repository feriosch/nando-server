package repository

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/core/utils"
)

// NewWeighIns returns the WeighInsRepository over the weigh_ins collection in db.
func NewWeighIns(db *mongo.Database) model.WeighInsRepository {
	return weighIns{collection: db.Collection("weigh_ins")}
}

// weighIns is the weigh_ins collection.
type weighIns struct {
	collection *mongo.Collection
}

func (r weighIns) Save(ctx context.Context, weighIn model.WeighIn) error {
	_, err := r.collection.ReplaceOne(ctx, bson.D{{Key: "_id", Value: weighIn.Week}}, weighIn, options.Replace().SetUpsert(true))
	return err
}

func (r weighIns) List(ctx context.Context) ([]model.WeighIn, error) {
	cursor, err := r.collection.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: -1}}))
	if err != nil {
		return nil, err
	}
	list := []model.WeighIn{}
	return list, cursor.All(ctx, &list)
}

func (r weighIns) Delete(ctx context.Context, week model.Day) error {
	_, err := r.collection.DeleteOne(ctx, bson.D{{Key: "_id", Value: week}})
	return err
}

func (r weighIns) FindLatest(ctx context.Context) (model.WeighIn, error) {
	var weighIn model.WeighIn
	err := r.collection.FindOne(ctx, bson.D{}, options.FindOne().SetSort(bson.D{{Key: "_id", Value: -1}})).Decode(&weighIn)
	return weighIn, utils.NotFound(err)
}

func (r weighIns) Find(ctx context.Context, week model.Day) (model.WeighIn, error) {
	var weighIn model.WeighIn
	err := r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: week}}).Decode(&weighIn)
	return weighIn, utils.NotFound(err)
}
