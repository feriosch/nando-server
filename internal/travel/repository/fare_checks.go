package repository

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/core/utils"
	"github.com/feriosch/nando-server/internal/travel/model"
)

// NewFareChecks returns the FareChecksRepository over the fare_checks
// collection in db.
func NewFareChecks(db *mongo.Database) model.FareChecksRepository {
	return fareChecks{collection: db.Collection("fare_checks")}
}

// fareChecks is the fare_checks collection.
type fareChecks struct {
	collection *mongo.Collection
}

func (r fareChecks) Insert(ctx context.Context, check model.FareCheck) (string, error) {
	check.ID = bson.NewObjectID().Hex()
	_, err := r.collection.InsertOne(ctx, check)
	return check.ID, err
}

func (r fareChecks) Find(ctx context.Context, id string) (model.FareCheck, error) {
	var check model.FareCheck
	err := r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&check)
	return check, utils.NotFound(err)
}

func (r fareChecks) Finish(ctx context.Context, id string, at time.Time, sources []string, summary string, errs []string) error {
	return utils.Matched(r.collection.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: id}, {Key: "finishedAt", Value: nil}},
		bson.D{{Key: "$set", Value: bson.D{
			{Key: "finishedAt", Value: at}, {Key: "sources", Value: sources},
			{Key: "summary", Value: summary}, {Key: "errors", Value: errs},
		}}}))
}

func (r fareChecks) LastFinished(ctx context.Context, kind model.FareCheckKind) (time.Time, error) {
	var check model.FareCheck
	err := r.collection.FindOne(ctx,
		bson.D{{Key: "kind", Value: kind}, {Key: "finishedAt", Value: bson.D{{Key: "$ne", Value: nil}}}},
		options.FindOne().SetSort(bson.D{{Key: "finishedAt", Value: -1}})).Decode(&check)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	return *check.FinishedAt, nil
}
