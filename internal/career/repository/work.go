package repository

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/career/model"
	"github.com/feriosch/nando-server/internal/core/utils"
)

// NewWork returns the WorkRepository over the work collection in db, where a
// Conversation has at most one open Item of each kind.
func NewWork(ctx context.Context, db *mongo.Database) (model.WorkRepository, error) {
	collection := db.Collection("work")
	_, err := collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "conversationId", Value: 1}, {Key: "kind", Value: 1}},
		Options: options.Index().SetUnique(true).
			SetPartialFilterExpression(bson.D{{Key: "open", Value: true}}),
	})
	if err != nil {
		return nil, err
	}
	return work{collection: collection}, nil
}

// work is the work collection.
type work struct {
	collection *mongo.Collection
}

func (r work) Add(ctx context.Context, kind string, conversationID, syncID bson.ObjectID, now time.Time) error {
	insert := bson.D{
		{Key: "state", Value: model.ItemWaiting},
		{Key: "claims", Value: 0},
		{Key: "createdAt", Value: now},
		{Key: "updatedAt", Value: now},
	}
	if !syncID.IsZero() {
		insert = append(insert, bson.E{Key: "syncId", Value: syncID})
	}
	_, err := r.collection.UpdateOne(ctx,
		bson.D{{Key: "conversationId", Value: conversationID}, {Key: "kind", Value: kind}, {Key: "open", Value: true}},
		bson.D{{Key: "$setOnInsert", Value: insert}},
		options.UpdateOne().SetUpsert(true),
	)
	// Another instance added the same Item at once.
	if mongo.IsDuplicateKeyError(err) {
		return nil
	}
	return err
}

// claimable matches the Items a run may claim: waiting, or claimed with the
// claim lapsed by now after fewer than maxClaims claims.
func claimable(now time.Time, maxClaims int) bson.D {
	return bson.D{{Key: "$or", Value: bson.A{
		bson.D{{Key: "state", Value: model.ItemWaiting}},
		append(lapsed(now), bson.E{Key: "claims", Value: bson.D{{Key: "$lt", Value: maxClaims}}}),
	}}}
}

// lapsed matches the claimed Items whose claim lapsed by now.
func lapsed(now time.Time) bson.D {
	return bson.D{
		{Key: "state", Value: model.ItemClaimed},
		{Key: "claimedUntil", Value: bson.D{{Key: "$lte", Value: now}}},
	}
}

func (r work) ClaimNext(ctx context.Context, now, claimedUntil time.Time, maxClaims int) (model.Item, error) {
	var item model.Item
	err := r.collection.FindOneAndUpdate(ctx,
		claimable(now, maxClaims),
		bson.D{
			{Key: "$set", Value: bson.D{
				{Key: "state", Value: model.ItemClaimed},
				{Key: "claimedUntil", Value: claimedUntil},
				{Key: "updatedAt", Value: now},
			}},
			{Key: "$inc", Value: bson.D{{Key: "claims", Value: 1}}},
		},
		options.FindOneAndUpdate().
			SetSort(bson.D{{Key: "createdAt", Value: 1}, {Key: "_id", Value: 1}}).
			SetReturnDocument(options.After),
	).Decode(&item)
	return item, utils.NotFound(err)
}

func (r work) FailUnfinished(ctx context.Context, now time.Time, maxClaims int) ([]model.Item, error) {
	failed := []model.Item{}
	for {
		var item model.Item
		err := r.collection.FindOneAndUpdate(ctx,
			append(lapsed(now), bson.E{Key: "claims", Value: bson.D{{Key: "$gte", Value: maxClaims}}}),
			bson.D{{Key: "$set", Value: bson.D{
				{Key: "state", Value: model.ItemFailed},
				{Key: "open", Value: false},
				{Key: "updatedAt", Value: now},
			}}},
			options.FindOneAndUpdate().SetReturnDocument(options.After),
		).Decode(&item)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return failed, nil
		}
		if err != nil {
			return failed, err
		}
		failed = append(failed, item)
	}
}

func (r work) Complete(ctx context.Context, conversationID bson.ObjectID, kinds []string, now time.Time) error {
	_, err := r.collection.UpdateMany(ctx,
		bson.D{
			{Key: "conversationId", Value: conversationID},
			{Key: "kind", Value: bson.D{{Key: "$in", Value: kinds}}},
			{Key: "open", Value: true},
		},
		bson.D{{Key: "$set", Value: bson.D{
			{Key: "state", Value: model.ItemDone},
			{Key: "open", Value: false},
			{Key: "updatedAt", Value: now},
		}}},
	)
	return err
}

func (r work) AnyClaimable(ctx context.Context, now time.Time, maxClaims int) (bool, error) {
	return r.any(ctx, claimable(now, maxClaims))
}

func (r work) AnyClaimed(ctx context.Context, now time.Time) (bool, error) {
	return r.any(ctx, bson.D{
		{Key: "state", Value: model.ItemClaimed},
		{Key: "claimedUntil", Value: bson.D{{Key: "$gt", Value: now}}},
	})
}

func (r work) AnyOpenFromSync(ctx context.Context, syncID bson.ObjectID) (bool, error) {
	return r.any(ctx, bson.D{{Key: "syncId", Value: syncID}, {Key: "open", Value: true}})
}

// any reports whether an Item matches filter.
func (r work) any(ctx context.Context, filter bson.D) (bool, error) {
	err := r.collection.FindOne(ctx, filter, options.FindOne().SetProjection(bson.D{{Key: "_id", Value: 1}})).Err()
	if errors.Is(err, mongo.ErrNoDocuments) {
		return false, nil
	}
	return err == nil, err
}
