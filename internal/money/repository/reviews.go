package repository

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/money/model"
)

// NewReviews returns the ReviewsRepository over the reviews collection in db:
// one document per month asked for, and one, "from", for the first month.
func NewReviews(db *mongo.Database) model.ReviewsRepository {
	return reviews{collection: db.Collection("reviews")}
}

// reviews is the reviews collection.
type reviews struct {
	collection *mongo.Collection
}

func (r reviews) From(ctx context.Context, month string) (string, error) {
	var doc struct {
		Month string `bson:"month"`
	}
	err := r.collection.FindOneAndUpdate(ctx, bson.D{{Key: "_id", Value: "from"}},
		bson.D{{Key: "$setOnInsert", Value: bson.D{{Key: "month", Value: month}}}},
		options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)).Decode(&doc)
	return doc.Month, err
}

func (r reviews) Asked(ctx context.Context, month string) (bool, error) {
	n, err := r.collection.CountDocuments(ctx, bson.D{{Key: "_id", Value: month}})
	return n > 0, err
}

func (r reviews) Record(ctx context.Context, month string) error {
	_, err := r.collection.UpdateOne(ctx, bson.D{{Key: "_id", Value: month}},
		bson.D{{Key: "$setOnInsert", Value: bson.D{{Key: "asked", Value: true}}}}, options.UpdateOne().SetUpsert(true))
	return err
}
