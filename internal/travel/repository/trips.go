package repository

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/core/utils"
	"github.com/feriosch/nando-server/internal/travel/model"
)

// NewTrips returns the TripsRepository over the trips collection in db.
func NewTrips(db *mongo.Database) model.TripsRepository {
	return trips{collection: db.Collection("trips")}
}

// trips is the trips collection.
type trips struct {
	collection *mongo.Collection
}

func (r trips) Insert(ctx context.Context, trip model.Trip) (string, error) {
	trip.ID = bson.NewObjectID().Hex()
	_, err := r.collection.InsertOne(ctx, trip)
	return trip.ID, err
}

func (r trips) Find(ctx context.Context, id string) (model.Trip, error) {
	var trip model.Trip
	err := r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&trip)
	return trip, utils.NotFound(err)
}

func (r trips) List(ctx context.Context, statuses []model.Status) ([]model.Trip, error) {
	filter := bson.D{{Key: "status", Value: bson.D{{Key: "$in", Value: statuses}}}}
	cursor, err := r.collection.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "name", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	list := []model.Trip{}
	return list, cursor.All(ctx, &list)
}

func (r trips) Replace(ctx context.Context, trip model.Trip) error {
	return utils.Matched(r.collection.ReplaceOne(ctx, bson.D{{Key: "_id", Value: trip.ID}}, trip))
}

func (r trips) Delete(ctx context.Context, id string) error {
	return utils.Deleted(r.collection.DeleteOne(ctx, bson.D{{Key: "_id", Value: id}}))
}
