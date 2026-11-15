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
// the last fire (firedAt), when a run last claimed or resolved a Word Request
// (activeAt), and when one last found nothing left and stopped (endedAt).
type scheduledTask struct {
	collection *mongo.Collection
}

// lastFireID is the _id of the one document.
const lastFireID = "last_fire"

// never stands in for a time not recorded yet.
var never = time.Unix(0, 0).UTC()

func (r scheduledTask) ClaimFire(ctx context.Context, now, startingSince, activeSince time.Time) (bool, error) {
	firedAt := bson.D{{Key: "$ifNull", Value: bson.A{"$firedAt", never}}}
	activeAt := bson.D{{Key: "$ifNull", Value: bson.A{"$activeAt", never}}}
	endedAt := bson.D{{Key: "$ifNull", Value: bson.A{"$endedAt", never}}}
	// The filter matches nothing while a fire is in flight or a run was just
	// active.
	result, err := r.collection.UpdateOne(ctx,
		bson.D{
			{Key: "_id", Value: lastFireID},
			{Key: "$expr", Value: bson.D{{Key: "$and", Value: bson.A{
				// No fire in flight: the last one is old, or a run worked
				// or stopped since it.
				bson.D{{Key: "$or", Value: bson.A{
					bson.D{{Key: "$lte", Value: bson.A{firedAt, startingSince}}},
					bson.D{{Key: "$gte", Value: bson.A{activeAt, firedAt}}},
					bson.D{{Key: "$gte", Value: bson.A{endedAt, firedAt}}},
				}}},
				// No run between Word Requests: none worked lately, or it
				// stopped since.
				bson.D{{Key: "$or", Value: bson.A{
					bson.D{{Key: "$lte", Value: bson.A{activeAt, activeSince}}},
					bson.D{{Key: "$gte", Value: bson.A{endedAt, activeAt}}},
				}}},
			}}}},
		},
		bson.D{{Key: "$set", Value: bson.D{{Key: "firedAt", Value: now}}}},
	)
	if err != nil || result.MatchedCount == 1 {
		return err == nil, err
	}
	// Nothing matched: the one document blocks the fire, or there is none
	// yet. $expr can't be in an upsert's filter, so it is inserted, and that
	// claims the fire unless another instance made it first.
	_, err = r.collection.InsertOne(ctx, bson.D{{Key: "_id", Value: lastFireID}, {Key: "firedAt", Value: now}})
	if mongo.IsDuplicateKeyError(err) {
		return false, nil
	}
	return err == nil, err
}

func (r scheduledTask) ReleaseFire(ctx context.Context, firedAt time.Time) error {
	_, err := r.collection.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: lastFireID}, {Key: "firedAt", Value: firedAt}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "firedAt", Value: never}}}},
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
		bson.D{{Key: "_id", Value: lastFireID}},
		bson.D{{Key: "$max", Value: bson.D{{Key: field, Value: now}}}},
		options.UpdateOne().SetUpsert(true),
	)
	return err
}
