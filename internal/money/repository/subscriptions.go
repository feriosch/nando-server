package repository

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/core/utils"
	"github.com/feriosch/nando-server/internal/money/model"
)

// NewSubscriptions returns the SubscriptionsRepository over the
// subscriptions collection in db.
func NewSubscriptions(db *mongo.Database) model.SubscriptionsRepository {
	return subscriptions{collection: db.Collection("subscriptions")}
}

// subscriptions is the subscriptions collection.
type subscriptions struct {
	collection *mongo.Collection
}

func (r subscriptions) Insert(ctx context.Context, s model.Subscription) (string, error) {
	s.ID = bson.NewObjectID().Hex()
	_, err := r.collection.InsertOne(ctx, s)
	return s.ID, err
}

func (r subscriptions) Find(ctx context.Context, id string) (model.Subscription, error) {
	var s model.Subscription
	err := r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&s)
	return s, utils.NotFound(err)
}

func (r subscriptions) List(ctx context.Context) ([]model.Subscription, error) {
	order := options.Find().SetSort(bson.D{{Key: "active", Value: -1}, {Key: "lastCharged", Value: -1}, {Key: "_id", Value: 1}})
	cursor, err := r.collection.Find(ctx, bson.D{}, order)
	if err != nil {
		return nil, err
	}
	list := []model.Subscription{}
	return list, cursor.All(ctx, &list)
}

func (r subscriptions) Replace(ctx context.Context, s model.Subscription) error {
	_, err := r.collection.ReplaceOne(ctx, bson.D{{Key: "_id", Value: s.ID}}, s)
	return err
}

func (r subscriptions) MoveSubcategory(ctx context.Context, from, into, intoCategory string) error {
	return moveSubcategory(ctx, r.collection, from, into, intoCategory)
}
