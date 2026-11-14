package repository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/japanese/model"
)

// NewScheduledTask returns the ScheduledTaskRepository over the
// scheduled_task collection in db.
func NewScheduledTask(db *mongo.Database) model.ScheduledTaskRepository {
	return scheduledTask{collection: db.Collection("scheduled_task")}
}

// scheduledTask is the scheduled_task collection, which holds one document:
// the last fire.
type scheduledTask struct {
	collection *mongo.Collection
}

// lastFireID is the _id of the one document, the last fire.
const lastFireID = "last_fire"

func (r scheduledTask) ClaimFire(ctx context.Context, now, notSince time.Time) (bool, error) {
	// With a fire after notSince the filter matches nothing, and the upsert
	// then collides with that fire's _id.
	_, err := r.collection.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: lastFireID}, {Key: "firedAt", Value: bson.D{{Key: "$lte", Value: notSince}}}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "firedAt", Value: now}}}},
		options.UpdateOne().SetUpsert(true),
	)
	if mongo.IsDuplicateKeyError(err) {
		return false, nil
	}
	return err == nil, err
}
