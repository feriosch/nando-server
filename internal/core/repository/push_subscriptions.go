package repository

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/core/model"
)

const pushSubscriptions = "push_subscriptions"

func (r mongoRepository) SavePushSubscription(ctx context.Context, sub model.PushSubscription) error {
	_, err := r.db.Collection(pushSubscriptions).ReplaceOne(ctx, bson.D{{Key: "_id", Value: sub.Endpoint}}, sub, options.Replace().SetUpsert(true))
	return err
}

func (r mongoRepository) ListPushSubscriptions(ctx context.Context) ([]model.PushSubscription, error) {
	cursor, err := r.db.Collection(pushSubscriptions).Find(ctx, bson.D{})
	if err != nil {
		return nil, err
	}
	list := []model.PushSubscription{}
	return list, cursor.All(ctx, &list)
}

func (r mongoRepository) DeletePushSubscription(ctx context.Context, endpoint string) error {
	_, err := r.db.Collection(pushSubscriptions).DeleteOne(ctx, bson.D{{Key: "_id", Value: endpoint}})
	return err
}
