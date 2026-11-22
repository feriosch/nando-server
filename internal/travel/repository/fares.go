package repository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/core/utils"
	"github.com/feriosch/nando-server/internal/travel/model"
)

// NewFares returns the FaresRepository over the fares collection in db, with
// its index: a Trip's Party's Fares by price.
func NewFares(ctx context.Context, db *mongo.Database) (model.FaresRepository, error) {
	collection := db.Collection("fares")
	_, err := collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "tripId", Value: 1}, {Key: "party", Value: 1}, {Key: "perPerson", Value: 1}},
	})
	return fares{collection: collection}, err
}

// fares is the fares collection.
type fares struct {
	collection *mongo.Collection
}

func (r fares) Insert(ctx context.Context, fare model.Fare) (string, error) {
	fare.ID = bson.NewObjectID().Hex()
	_, err := r.collection.InsertOne(ctx, fare)
	return fare.ID, err
}

func (r fares) List(ctx context.Context, tripID string, party model.PartyKey, since time.Time) ([]model.Fare, error) {
	filter := bson.D{
		{Key: "tripId", Value: tripID}, {Key: "party", Value: party},
		{Key: "foundAt", Value: bson.D{{Key: "$gte", Value: since}}},
	}
	cheapest := options.Find().SetSort(bson.D{{Key: "perPerson", Value: 1}, {Key: "foundAt", Value: -1}, {Key: "_id", Value: 1}})
	cursor, err := r.collection.Find(ctx, filter, cheapest)
	if err != nil {
		return nil, err
	}
	list := []model.Fare{}
	return list, cursor.All(ctx, &list)
}

func (r fares) Find(ctx context.Context, id string) (model.Fare, error) {
	var fare model.Fare
	err := r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&fare)
	return fare, utils.NotFound(err)
}

func (r fares) DeleteTrip(ctx context.Context, tripID string) error {
	_, err := r.collection.DeleteMany(ctx, bson.D{{Key: "tripId", Value: tripID}})
	return err
}
