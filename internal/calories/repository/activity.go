package repository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/calories/model"
	coremodel "github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/core/utils"
)

// NewHealthSyncs returns the HealthSyncsRepository over the health_syncs
// collection in db, first creating any indexes it lacks.
func NewHealthSyncs(ctx context.Context, db *mongo.Database) (model.HealthSyncsRepository, error) {
	collection := db.Collection("health_syncs")
	_, err := collection.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "syncedAt", Value: -1}}})
	if err != nil {
		return nil, err
	}
	return healthSyncs{collection: collection}, nil
}

// healthSyncs is the health_syncs collection, one document per Day.
type healthSyncs struct {
	collection *mongo.Collection
}

func (r healthSyncs) Save(ctx context.Context, sync model.HealthSync) error {
	_, err := r.collection.ReplaceOne(ctx, bson.D{{Key: "_id", Value: sync.Day}}, sync, options.Replace().SetUpsert(true))
	return err
}

func (r healthSyncs) Find(ctx context.Context, day coremodel.Day) (model.HealthSync, error) {
	var sync model.HealthSync
	err := r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: day}}).Decode(&sync)
	return sync, utils.NotFound(err)
}

func (r healthSyncs) ListBetween(ctx context.Context, from, to coremodel.Day) ([]model.HealthSync, error) {
	cursor, err := r.collection.Find(ctx, bson.D{{Key: "_id", Value: bson.D{{Key: "$gte", Value: from}, {Key: "$lte", Value: to}}}})
	if err != nil {
		return nil, err
	}
	list := []model.HealthSync{}
	return list, cursor.All(ctx, &list)
}

func (r healthSyncs) LastSyncedAt(ctx context.Context) (time.Time, error) {
	var sync model.HealthSync
	err := r.collection.FindOne(ctx, bson.D{}, options.FindOne().SetSort(bson.D{{Key: "syncedAt", Value: -1}})).Decode(&sync)
	return sync.SyncedAt, utils.NotFound(err)
}

// NewTreadmillWalks returns the TreadmillWalksRepository over the
// treadmill_walks collection in db, first creating any indexes it lacks.
func NewTreadmillWalks(ctx context.Context, db *mongo.Database) (model.TreadmillWalksRepository, error) {
	collection := db.Collection("treadmill_walks")
	_, err := collection.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "day", Value: 1}, {Key: "loggedAt", Value: 1}}})
	if err != nil {
		return nil, err
	}
	return treadmillWalks{collection: collection}, nil
}

// treadmillWalks is the treadmill_walks collection.
type treadmillWalks struct {
	collection *mongo.Collection
}

func (r treadmillWalks) Insert(ctx context.Context, walk model.TreadmillWalk) (string, error) {
	walk.ID = bson.NewObjectID().Hex()
	_, err := r.collection.InsertOne(ctx, walk)
	return walk.ID, err
}

func (r treadmillWalks) Find(ctx context.Context, id string) (model.TreadmillWalk, error) {
	var walk model.TreadmillWalk
	err := r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&walk)
	return walk, utils.NotFound(err)
}

func (r treadmillWalks) Update(ctx context.Context, walk model.TreadmillWalk) error {
	result, err := r.collection.UpdateByID(ctx, walk.ID, bson.D{{Key: "$set", Value: bson.D{
		{Key: "km", Value: walk.Km},
		{Key: "steps", Value: walk.Steps},
		{Key: "kcal", Value: walk.Kcal},
		{Key: "kcalEstimated", Value: walk.KcalEstimated},
	}}})
	return utils.Matched(result, err)
}

func (r treadmillWalks) Delete(ctx context.Context, id string) error {
	_, err := r.collection.DeleteOne(ctx, bson.D{{Key: "_id", Value: id}})
	return err
}

func (r treadmillWalks) ListBetween(ctx context.Context, from, to coremodel.Day) ([]model.TreadmillWalk, error) {
	cursor, err := r.collection.Find(ctx, bson.D{{Key: "day", Value: bson.D{{Key: "$gte", Value: from}, {Key: "$lte", Value: to}}}},
		options.Find().SetSort(bson.D{{Key: "day", Value: 1}, {Key: "loggedAt", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	list := []model.TreadmillWalk{}
	return list, cursor.All(ctx, &list)
}
