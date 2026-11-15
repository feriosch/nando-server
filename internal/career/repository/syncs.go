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

func (r syncs) Insert(ctx context.Context, sync model.Sync) (bson.ObjectID, error) {
	sync.ID = bson.NewObjectID()
	_, err := r.collection.InsertOne(ctx, sync)
	return sync.ID, err
}

// read matches the Syncs that read LinkedIn: those with no problem.
var read = bson.E{Key: "problem", Value: bson.D{{Key: "$exists", Value: false}}}

func (r syncs) CountReadSince(ctx context.Context, from time.Time) (int64, error) {
	return r.collection.CountDocuments(ctx, bson.D{read, {Key: "at", Value: bson.D{{Key: "$gte", Value: from}}}})
}

func (r syncs) LatestRead(ctx context.Context) (model.Sync, error) {
	var sync model.Sync
	err := r.collection.FindOne(ctx, bson.D{read}, options.FindOne().SetSort(bson.D{{Key: "at", Value: -1}})).Decode(&sync)
	return sync, utils.NotFound(err)
}
