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
// the last fire (firedAt) and the last run activity (activeAt).
type scheduledTask struct {
	collection *mongo.Collection
}

// lastFireID is the _id of the one document.
const lastFireID = "last_fire"

// never stands in for a time not recorded yet.
var never = time.Unix(0, 0).UTC()

func (r scheduledTask) ClaimFire(ctx context.Context, now, startingSince, activeSince time.Time) (bool, error) {
	// $expr can't be in an upsert's filter, so the one document is made
	// first if it isn't there; another instance may make it at once.
	_, err := r.collection.InsertOne(ctx, bson.D{{Key: "_id", Value: lastFireID}, {Key: "firedAt", Value: never}, {Key: "activeAt", Value: never}})
	if err != nil && !mongo.IsDuplicateKeyError(err) {
		return false, err
	}
	firedAt := bson.D{{Key: "$ifNull", Value: bson.A{"$firedAt", never}}}
	activeAt := bson.D{{Key: "$ifNull", Value: bson.A{"$activeAt", never}}}
	// The filter matches nothing while a fire is in flight or a run was just
	// active.
	result, err := r.collection.UpdateOne(ctx,
		bson.D{
			{Key: "_id", Value: lastFireID},
			{Key: "$expr", Value: bson.D{{Key: "$and", Value: bson.A{
				// No fire in flight: the last one is old, or a run was
				// active since it.
				bson.D{{Key: "$or", Value: bson.A{
					bson.D{{Key: "$lte", Value: bson.A{firedAt, startingSince}}},
					bson.D{{Key: "$gte", Value: bson.A{activeAt, firedAt}}},
				}}},
				// No run active lately.
				bson.D{{Key: "$lte", Value: bson.A{activeAt, activeSince}}},
			}}}},
		},
		bson.D{{Key: "$set", Value: bson.D{{Key: "firedAt", Value: now}}}},
	)
	if err != nil {
		return false, err
	}
	return result.MatchedCount == 1, nil
}

func (r scheduledTask) ReleaseFire(ctx context.Context, firedAt time.Time) error {
	_, err := r.collection.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: lastFireID}, {Key: "firedAt", Value: firedAt}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "firedAt", Value: never}}}},
	)
	return err
}

func (r scheduledTask) RecordActivity(ctx context.Context, now time.Time) error {
	_, err := r.collection.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: lastFireID}},
		bson.D{{Key: "$max", Value: bson.D{{Key: "activeAt", Value: now}}}},
		options.UpdateOne().SetUpsert(true),
	)
	return err
}
