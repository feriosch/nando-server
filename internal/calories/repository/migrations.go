package repository

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/calories/model"
)

// NewMigrations returns the MigrationsRepository over the migrations
// collection in db.
func NewMigrations(db *mongo.Database) model.MigrationsRepository {
	return migrations{collection: db.Collection("migrations")}
}

// migrations is the migrations collection, one document per migration done,
// with the migration's name as its _id.
type migrations struct {
	collection *mongo.Collection
}

func (r migrations) Done(ctx context.Context, name string) (bool, error) {
	err := r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: name}}).Err()
	if errors.Is(err, mongo.ErrNoDocuments) {
		return false, nil
	}
	return err == nil, err
}

func (r migrations) MarkDone(ctx context.Context, name string) error {
	_, err := r.collection.UpdateOne(ctx, bson.D{{Key: "_id", Value: name}},
		bson.D{{Key: "$setOnInsert", Value: bson.D{{Key: "_id", Value: name}}}}, options.UpdateOne().SetUpsert(true))
	return err
}
