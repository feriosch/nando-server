package repository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/career/model"
	"github.com/feriosch/nando-server/internal/core/utils"
)

// NewSyncs returns the SyncsRepository over the syncs collection in db.
func NewSyncs(db *mongo.Database) model.SyncsRepository {
	return syncs{collection: db.Collection("syncs")}
}

// syncs is the syncs collection.
type syncs struct {
	collection *mongo.Collection
}

func (r syncs) Insert(ctx context.Context, sync model.Sync) error {
	_, err := r.collection.InsertOne(ctx, sync)
	return err
}

func (r syncs) Read(ctx context.Context, id bson.ObjectID) (model.Sync, error) {
	var sync model.Sync
	err := r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: id}, read}).Decode(&sync)
	return sync, utils.NotFound(err)
}

func (r syncs) AddThreads(ctx context.Context, id bson.ObjectID, n int) error {
	return utils.Matched(r.collection.UpdateByID(ctx, id, bson.D{{Key: "$inc", Value: bson.D{{Key: "threads", Value: n}}}}))
}

// read matches the Syncs that read LinkedIn: those with no problem.
var read = bson.E{Key: "problem", Value: bson.D{{Key: "$exists", Value: false}}}

func (r syncs) LatestRead(ctx context.Context, n int64) ([]model.Sync, error) {
	cursor, err := r.collection.Find(ctx, bson.D{read}, options.Find().SetSort(bson.D{{Key: "at", Value: -1}}).SetLimit(n))
	if err != nil {
		return nil, err
	}
	syncs := []model.Sync{}
	return syncs, cursor.All(ctx, &syncs)
}

func (r syncs) AllReadSince(ctx context.Context, since time.Time) ([]model.Sync, error) {
	cursor, err := r.collection.Find(ctx, bson.D{read, {Key: "at", Value: bson.D{{Key: "$gte", Value: since}}}},
		options.Find().SetSort(bson.D{{Key: "at", Value: 1}}))
	if err != nil {
		return nil, err
	}
	syncs := []model.Sync{}
	return syncs, cursor.All(ctx, &syncs)
}

func (r syncs) FirstProblemAfter(ctx context.Context, kind string, after time.Time) (model.Sync, error) {
	var sync model.Sync
	err := r.collection.FindOne(ctx,
		bson.D{{Key: "problem.kind", Value: kind}, {Key: "at", Value: bson.D{{Key: "$gt", Value: after}}}},
		options.FindOne().SetSort(bson.D{{Key: "at", Value: 1}}),
	).Decode(&sync)
	return sync, utils.NotFound(err)
}

func (r syncs) FirstRead(ctx context.Context) (model.Sync, error) {
	var sync model.Sync
	err := r.collection.FindOne(ctx, bson.D{read}, options.FindOne().SetSort(bson.D{{Key: "at", Value: 1}})).Decode(&sync)
	return sync, utils.NotFound(err)
}
