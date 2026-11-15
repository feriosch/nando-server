package repository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/core/utils"
	"github.com/feriosch/nando-server/internal/japanese/model"
)

// NewScheduledTask returns the ScheduledTaskRepository over the
// scheduled_task collection in db.
func NewScheduledTask(db *mongo.Database) model.ScheduledTaskRepository {
	return scheduledTask{collection: db.Collection("scheduled_task")}
}

// scheduledTask is the scheduled_task collection, which holds one document:
// the last fire (firedAt), when a run last claimed or resolved a Word Request
// (activeAt), and when one last found nothing left and stopped (endedAt).
type scheduledTask struct {
	collection *mongo.Collection
}

func (r scheduledTask) ClaimFire(ctx context.Context, now, startingSince, activeSince time.Time) (bool, error) {
	result, err := r.collection.UpdateOne(ctx, utils.ClaimFireFilter(startingSince, activeSince),
		bson.D{{Key: "$set", Value: bson.D{{Key: "firedAt", Value: now}}}})
	if err != nil || result.MatchedCount == 1 {
		return err == nil, err
	}
	// Nothing matched: the one document blocks the fire, or there is none
	// yet. $expr can't be in an upsert's filter, so it is inserted, and that
	// claims the fire unless another instance made it first.
	_, err = r.collection.InsertOne(ctx, bson.D{{Key: "_id", Value: utils.LastFireID}, {Key: "firedAt", Value: now}})
	if mongo.IsDuplicateKeyError(err) {
		return false, nil
	}
	return err == nil, err
}

func (r scheduledTask) ReleaseFire(ctx context.Context, firedAt time.Time) error {
	_, err := r.collection.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: utils.LastFireID}, {Key: "firedAt", Value: firedAt}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "firedAt", Value: utils.Never}}}},
	)
	return err
}

func (r scheduledTask) RecordWorking(ctx context.Context, now time.Time) error {
	return r.record(ctx, "activeAt", now)
}

func (r scheduledTask) RecordStopped(ctx context.Context, now time.Time) error {
	return r.record(ctx, "endedAt", now)
}

// record sets the time field to now, unless it holds a later one.
func (r scheduledTask) record(ctx context.Context, field string, now time.Time) error {
	_, err := r.collection.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: utils.LastFireID}},
		bson.D{{Key: "$max", Value: bson.D{{Key: field, Value: now}}}},
		options.UpdateOne().SetUpsert(true),
	)
	return err
}
