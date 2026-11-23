package repository

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/core/utils"
	"github.com/feriosch/nando-server/internal/money/model"
)

// NewRates returns the RatesRepository over the rates collection in db.
func NewRates(db *mongo.Database) model.RatesRepository {
	return rates{collection: db.Collection("rates")}
}

// rates is the rates collection.
type rates struct {
	collection *mongo.Collection
}

func (r rates) Find(ctx context.Context, month string) (model.Rate, error) {
	var rate model.Rate
	err := r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: month}}).Decode(&rate)
	return rate, utils.NotFound(err)
}

func (r rates) Insert(ctx context.Context, rate model.Rate) error {
	// $setOnInsert: a Rate already stored never changes.
	_, err := r.collection.UpdateOne(ctx, bson.D{{Key: "_id", Value: rate.Month}},
		bson.D{{Key: "$setOnInsert", Value: bson.D{{Key: "average", Value: rate.Average}, {Key: "monthEnd", Value: rate.MonthEnd}}}},
		options.UpdateOne().SetUpsert(true))
	return err
}
