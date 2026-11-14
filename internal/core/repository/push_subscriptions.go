package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/core/utils"
)

// NewPushSubscriptions returns the PushSubscriptionsRepository over the
// push_subscriptions collection in db, and forgotten_push_subscriptions for
// the endpoints a push service no longer knows, first creating any indexes it
// lacks.
func NewPushSubscriptions(ctx context.Context, db *mongo.Database) (model.PushSubscriptionsRepository, error) {
	forgotten := db.Collection("forgotten_push_subscriptions")
	if _, err := forgotten.Indexes().CreateMany(ctx, []mongo.IndexModel{utils.ExpiresAtIndex}); err != nil {
		return nil, err
	}
	return pushSubscriptions{collection: db.Collection("push_subscriptions"), forgotten: forgotten}, nil
}

// pushSubscriptions is the push_subscriptions collection, and
// forgotten_push_subscriptions.
type pushSubscriptions struct {
	collection *mongo.Collection
	// forgotten holds each endpoint a push service no longer knows by its
	// SHA-256 alone, so that the endpoint, a secret capability URL, isn't kept
	// once it is no use.
	forgotten *mongo.Collection
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

// Forget remembers first, so that a Push Subscription is never deleted
// without the web app being told why when it registers it again.
func (r pushSubscriptions) Forget(ctx context.Context, endpoint string, now, expiresAt time.Time) error {
	_, err := r.forgotten.ReplaceOne(ctx, bson.D{{Key: "_id", Value: endpointHash(endpoint)}}, bson.D{
		{Key: "forgottenAt", Value: now},
		{Key: "expiresAt", Value: expiresAt},
	}, options.Replace().SetUpsert(true))
	if err != nil {
		return err
	}
	return r.Delete(ctx, endpoint)
}

func (r pushSubscriptions) Forgotten(ctx context.Context, endpoint string, now time.Time) (bool, error) {
	err := r.forgotten.FindOne(ctx, bson.D{{Key: "_id", Value: endpointHash(endpoint)}, utils.Unexpired(now)}).Err()
	if errors.Is(err, mongo.ErrNoDocuments) {
		return false, nil
	}
	return err == nil, err
}

// endpointHash is endpoint's SHA-256, hex.
func endpointHash(endpoint string) string {
	sum := sha256.Sum256([]byte(endpoint))
	return hex.EncodeToString(sum[:])
}
