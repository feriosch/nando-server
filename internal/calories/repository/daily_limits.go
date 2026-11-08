package repository

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/calories/model"
)

// NewDailyLimits returns the DailyLimitsRepository over the daily_limits
// collection in db.
func NewDailyLimits(db *mongo.Database) model.DailyLimitsRepository {
	return dailyLimits{collection: db.Collection("daily_limits")}
}

// dailyLimits is the daily_limits collection, one document per Day a Daily
// Limit took effect, keyed by that Day.
type dailyLimits struct {
	collection *mongo.Collection
}

func (r dailyLimits) Save(ctx context.Context, limit model.DailyLimit) error {
	_, err := r.collection.ReplaceOne(ctx, bson.D{{Key: "_id", Value: limit.EffectiveFrom}}, limit, options.Replace().SetUpsert(true))
	return err
}

func (r dailyLimits) List(ctx context.Context) (model.DailyLimits, error) {
	cursor, err := r.collection.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	list := model.DailyLimits{}
	return list, cursor.All(ctx, &list)
}
