package repository

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/core/model"
)

const weighIns = "weigh_ins"

func (r mongoRepository) SaveWeighIn(ctx context.Context, weighIn model.WeighIn) error {
	_, err := r.db.Collection(weighIns).ReplaceOne(ctx, bson.D{{Key: "_id", Value: weighIn.Week}}, weighIn, options.Replace().SetUpsert(true))
	return err
}

func (r mongoRepository) ListWeighIns(ctx context.Context) ([]model.WeighIn, error) {
	cursor, err := r.db.Collection(weighIns).Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: -1}}))
	if err != nil {
		return nil, err
	}
	list := []model.WeighIn{}
	return list, cursor.All(ctx, &list)
}

func (r mongoRepository) DeleteWeighIn(ctx context.Context, week model.Day) error {
	_, err := r.db.Collection(weighIns).DeleteOne(ctx, bson.D{{Key: "_id", Value: week}})
	return err
}

func (r mongoRepository) FindLatestWeighIn(ctx context.Context) (model.WeighIn, error) {
	var weighIn model.WeighIn
	err := r.db.Collection(weighIns).FindOne(ctx, bson.D{}, options.FindOne().SetSort(bson.D{{Key: "_id", Value: -1}})).Decode(&weighIn)
	return weighIn, notFound(err)
}

func (r mongoRepository) FindWeighIn(ctx context.Context, week model.Day) (model.WeighIn, error) {
	var weighIn model.WeighIn
	err := r.db.Collection(weighIns).FindOne(ctx, bson.D{{Key: "_id", Value: week}}).Decode(&weighIn)
	return weighIn, notFound(err)
}
