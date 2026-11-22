package repository

import (
	"context"
	"slices"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	coremodel "github.com/feriosch/nando-server/internal/core/model"
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
	// By name as the Owner reads it: "beach" between "Alps" and "City".
	byName := options.Find().
		SetSort(bson.D{{Key: "name", Value: 1}, {Key: "_id", Value: 1}}).
		SetCollation(&options.Collation{Locale: "en", Strength: 2})
	cursor, err := r.collection.Find(ctx, filter, byName)
	if err != nil {
		return nil, err
	}
	list := []model.Trip{}
	return list, cursor.All(ctx, &list)
}

func (r trips) Update(ctx context.Context, trip model.Trip) error {
	doc, err := bson.Marshal(trip)
	if err != nil {
		return err
	}
	var fields bson.D
	if err := bson.Unmarshal(doc, &fields); err != nil {
		return err
	}
	// The Status is SetStatus's alone, so a save racing a status change
	// never puts the old Status back.
	fields = slices.DeleteFunc(fields, func(e bson.E) bool {
		return e.Key == "_id" || e.Key == "status" || e.Key == "statusChangedOn" || e.Key == "targetFares"
	})
	return utils.Matched(r.collection.UpdateOne(ctx, bson.D{{Key: "_id", Value: trip.ID}}, bson.D{{Key: "$set", Value: fields}}))
}

func (r trips) SetTargetFare(ctx context.Context, id string, party model.PartyKey, target *model.TargetFare) error {
	// One pipeline update, so two Parties' Target Fares set at once both stay.
	added := bson.A{}
	if target != nil {
		added = append(added, *target)
	}
	others := bson.D{{Key: "$filter", Value: bson.D{
		{Key: "input", Value: bson.D{{Key: "$ifNull", Value: bson.A{"$targetFares", bson.A{}}}}},
		{Key: "cond", Value: bson.D{{Key: "$ne", Value: bson.A{"$$this.party", party}}}},
	}}}
	return utils.Matched(r.collection.UpdateOne(ctx, bson.D{{Key: "_id", Value: id}}, mongo.Pipeline{
		{{Key: "$set", Value: bson.D{{Key: "targetFares", Value: bson.D{{Key: "$concatArrays", Value: bson.A{others, added}}}}}}},
	}))
}

func (r trips) SetStatus(ctx context.Context, id string, status model.Status, on coremodel.Day) error {
	return utils.Matched(r.collection.UpdateOne(ctx, bson.D{{Key: "_id", Value: id}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "status", Value: status}, {Key: "statusChangedOn", Value: on}}}}))
}

func (r trips) Delete(ctx context.Context, id string) error {
	return utils.Deleted(r.collection.DeleteOne(ctx, bson.D{{Key: "_id", Value: id}}))
}
