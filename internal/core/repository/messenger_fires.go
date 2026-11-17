package repository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/core/utils"
)

// NewMessengerFires returns the fire record of assistant's runs: its document
// in the messenger_scheduled_task collection in db. Every Assistant's runs
// come from the one scheduled task, but each Assistant is woken under the
// wake rules on its own (ADR 0008).
func NewMessengerFires(db *mongo.Database, assistant string) utils.FireRecord {
	return messengerFires{collection: db.Collection("messenger_scheduled_task"), id: assistant}
}

// messengerFires is one Assistant's document in messenger_scheduled_task: its
// last fire (firedAt), when one of its runs last claimed or answered its Talk
// (activeAt), and when one last found nothing left and stopped (endedAt).
type messengerFires struct {
	collection *mongo.Collection
	id         string
}

func (r messengerFires) ClaimFire(ctx context.Context, now, startingSince, activeSince time.Time) (bool, error) {
	result, err := r.collection.UpdateOne(ctx, utils.ClaimFireFilterFor(r.id, startingSince, activeSince),
		bson.D{{Key: "$set", Value: bson.D{{Key: "firedAt", Value: now}}}})
	if err != nil || result.MatchedCount == 1 {
		return err == nil, err
	}
	// As in Japanese's: nothing matched, so the document blocks the fire or
	// there is none yet, and inserting it claims the fire.
	_, err = r.collection.InsertOne(ctx, bson.D{{Key: "_id", Value: r.id}, {Key: "firedAt", Value: now}})
	if mongo.IsDuplicateKeyError(err) {
		return false, nil
	}
	return err == nil, err
}

func (r messengerFires) ReleaseFire(ctx context.Context, firedAt time.Time) error {
	_, err := r.collection.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: r.id}, {Key: "firedAt", Value: firedAt}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "firedAt", Value: utils.Never}}}},
	)
	return err
}

func (r messengerFires) RecordWorking(ctx context.Context, now time.Time) error {
	return r.record(ctx, "activeAt", now)
}

func (r messengerFires) RecordStopped(ctx context.Context, now time.Time) error {
	return r.record(ctx, "endedAt", now)
}

// record sets the time field to now, unless it holds a later one.
func (r messengerFires) record(ctx context.Context, field string, now time.Time) error {
	_, err := r.collection.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: r.id}},
		bson.D{{Key: "$max", Value: bson.D{{Key: field, Value: now}}}},
		options.UpdateOne().SetUpsert(true),
	)
	return err
}
