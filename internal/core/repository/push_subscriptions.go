package repository

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/core/model"
)

// NewPushSubscriptions returns the PushSubscriptionsRepository over the
// push_subscriptions collection in db.
func NewPushSubscriptions(db *mongo.Database) model.PushSubscriptionsRepository {
	return pushSubscriptions{collection: db.Collection("push_subscriptions")}
}

// pushSubscriptions is the push_subscriptions collection.
type pushSubscriptions struct {
	collection *mongo.Collection
}

func (r pushSubscriptions) Save(ctx context.Context, sub model.PushSubscription) error {
	_, err := r.collection.ReplaceOne(ctx, bson.D{{Key: "_id", Value: sub.Endpoint}}, sub, options.Replace().SetUpsert(true))
	return err
}

func (r pushSubscriptions) List(ctx context.Context) ([]model.PushSubscription, error) {
	cursor, err := r.collection.Find(ctx, bson.D{})
	if err != nil {
		return nil, err
	}
	list := []model.PushSubscription{}
	return list, cursor.All(ctx, &list)
}

func (r pushSubscriptions) Delete(ctx context.Context, endpoint string) error {
	_, err := r.collection.DeleteOne(ctx, bson.D{{Key: "_id", Value: endpoint}})
	return err
}
