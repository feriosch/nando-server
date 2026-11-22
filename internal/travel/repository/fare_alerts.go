package repository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/travel/model"
)

// NewFareAlerts returns the FareAlertsRepository over the fare_alerts
// collection in db.
func NewFareAlerts(db *mongo.Database) model.FareAlertsRepository {
	return fareAlerts{collection: db.Collection("fare_alerts")}
}

// fareAlerts is the fare_alerts collection.
type fareAlerts struct {
	collection *mongo.Collection
}

func (r fareAlerts) Insert(ctx context.Context, alert model.FareAlert) (string, error) {
	alert.ID = bson.NewObjectID().Hex()
	_, err := r.collection.InsertOne(ctx, alert)
	return alert.ID, err
}

func (r fareAlerts) List(ctx context.Context, tripID string, party model.PartyKey, since time.Time) ([]model.FareAlert, error) {
	cursor, err := r.collection.Find(ctx, bson.D{
		{Key: "tripId", Value: tripID}, {Key: "party", Value: party},
		{Key: "sentAt", Value: bson.D{{Key: "$gte", Value: since}}},
	}, options.Find().SetSort(bson.D{{Key: "sentAt", Value: -1}, {Key: "_id", Value: -1}}))
	if err != nil {
		return nil, err
	}
	list := []model.FareAlert{}
	return list, cursor.All(ctx, &list)
}

func (r fareAlerts) DeleteTrip(ctx context.Context, tripID string) error {
	_, err := r.collection.DeleteMany(ctx, bson.D{{Key: "tripId", Value: tripID}})
	return err
}
