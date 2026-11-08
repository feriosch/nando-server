package repository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/core/utils"
)

// NewSentNotifications returns the SentNotificationsRepository over the
// sent_notifications collection in db, first creating any indexes it lacks.
func NewSentNotifications(ctx context.Context, db *mongo.Database) (model.SentNotificationsRepository, error) {
	collection := db.Collection("sent_notifications")
	if _, err := collection.Indexes().CreateMany(ctx, []mongo.IndexModel{utils.ExpiresAtIndex}); err != nil {
		return nil, err
	}
	return sentNotifications{collection: collection}, nil
}

// sentNotifications is the sent_notifications collection.
type sentNotifications struct {
	collection *mongo.Collection
}

func (r sentNotifications) Claim(ctx context.Context, id string, now, expiresAt time.Time) (bool, error) {
	_, err := r.collection.InsertOne(ctx, bson.D{
		{Key: "_id", Value: id},
		{Key: "sentAt", Value: now},
		{Key: "expiresAt", Value: expiresAt},
	})
	if mongo.IsDuplicateKeyError(err) {
		return false, nil
	}
	return err == nil, err
}
