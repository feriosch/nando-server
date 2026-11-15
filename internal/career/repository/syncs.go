package repository

import (
	"context"

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

func (r syncs) Read(ctx context.Context, id string) (model.Sync, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return model.Sync{}, utils.ErrNotFound
	}
	var sync model.Sync
	err = r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: oid}, read}).Decode(&sync)
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
