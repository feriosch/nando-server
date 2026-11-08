package repository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

const sentNotifications = "sent_notifications"

var sentNotificationIndexes = []mongo.IndexModel{expiresAtIndex}

func (r mongoRepository) ClaimNotification(ctx context.Context, id string, now, expiresAt time.Time) (bool, error) {
	_, err := r.db.Collection(sentNotifications).InsertOne(ctx, bson.D{
		{Key: "_id", Value: id},
		{Key: "sentAt", Value: now},
		{Key: "expiresAt", Value: expiresAt},
	})
	if mongo.IsDuplicateKeyError(err) {
		return false, nil
	}
	return err == nil, err
}
