package repository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/core/utils"
)

// NewSyncKeys returns the SyncKeysRepository over the sync_keys collection in
// db, first creating any indexes it lacks.
func NewSyncKeys(ctx context.Context, db *mongo.Database) (model.SyncKeysRepository, error) {
	collection := db.Collection("sync_keys")
	_, err := collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "keyHash", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return nil, err
	}
	return syncKeys{collection: collection}, nil
}

// syncKeys is the sync_keys collection.
type syncKeys struct {
	collection *mongo.Collection
}

func (r syncKeys) Insert(ctx context.Context, key model.SyncKey) error {
	_, err := r.collection.InsertOne(ctx, key)
	return err
}

func (r syncKeys) FindByKey(ctx context.Context, hash []byte) (model.SyncKey, error) {
	var key model.SyncKey
	err := r.collection.FindOne(ctx, bson.D{{Key: "keyHash", Value: hash}}).Decode(&key)
	return key, utils.NotFound(err)
}

func (r syncKeys) List(ctx context.Context) ([]model.SyncKey, error) {
	cursor, err := r.collection.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	list := []model.SyncKey{}
	err = cursor.All(ctx, &list)
	return list, err
}

func (r syncKeys) MarkUsed(ctx context.Context, id string, at time.Time) error {
	_, err := r.collection.UpdateByID(ctx, id, bson.D{{Key: "$set", Value: bson.D{{Key: "lastUsedAt", Value: at}}}})
	return err
}

func (r syncKeys) Delete(ctx context.Context, id string) error {
	_, err := r.collection.DeleteOne(ctx, bson.D{{Key: "_id", Value: id}})
	return err
}
