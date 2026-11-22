package repository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	coremodel "github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/core/utils"
	"github.com/feriosch/nando-server/internal/travel/model"
)

// NewPriceVerdicts returns the PriceVerdictsRepository over the
// price_verdicts collection in db.
func NewPriceVerdicts(db *mongo.Database) model.PriceVerdictsRepository {
	return priceVerdicts{collection: db.Collection("price_verdicts")}
}

// priceVerdicts is the price_verdicts collection.
type priceVerdicts struct {
	collection *mongo.Collection
}

func (r priceVerdicts) Insert(ctx context.Context, verdict model.PriceVerdict) error {
	verdict.ID = bson.NewObjectID().Hex()
	_, err := r.collection.InsertOne(ctx, verdict)
	return err
}

func (r priceVerdicts) FindOn(ctx context.Context, tripID string, party model.PartyKey, depart, back, day coremodel.Day) (model.PriceVerdict, error) {
	var verdict model.PriceVerdict
	err := r.collection.FindOne(ctx, bson.D{
		{Key: "tripId", Value: tripID}, {Key: "party", Value: party},
		{Key: "departDate", Value: depart}, {Key: "returnDate", Value: back}, {Key: "day", Value: day},
	}).Decode(&verdict)
	return verdict, utils.NotFound(err)
}

func (r priceVerdicts) List(ctx context.Context, tripID string, party model.PartyKey, limit int) ([]model.PriceVerdict, error) {
	cursor, err := r.collection.Find(ctx, bson.D{{Key: "tripId", Value: tripID}, {Key: "party", Value: party}},
		options.Find().SetSort(bson.D{{Key: "fetchedAt", Value: -1}, {Key: "_id", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	list := []model.PriceVerdict{}
	return list, cursor.All(ctx, &list)
}

func (r priceVerdicts) DeleteTrip(ctx context.Context, tripID string) error {
	_, err := r.collection.DeleteMany(ctx, bson.D{{Key: "tripId", Value: tripID}})
	return err
}

// NewSerpAPICalls returns the SerpAPICallsRepository over the serpapi_calls
// collection in db.
func NewSerpAPICalls(db *mongo.Database) model.SerpAPICallsRepository {
	return serpAPICalls{collection: db.Collection("serpapi_calls")}
}

// serpAPICalls is the serpapi_calls collection.
type serpAPICalls struct {
	collection *mongo.Collection
}

func (r serpAPICalls) Insert(ctx context.Context, at time.Time) error {
	_, err := r.collection.InsertOne(ctx, bson.D{{Key: "at", Value: at}})
	return err
}

func (r serpAPICalls) CountSince(ctx context.Context, since time.Time) (int, error) {
	n, err := r.collection.CountDocuments(ctx, bson.D{{Key: "at", Value: bson.D{{Key: "$gte", Value: since}}}})
	return int(n), err
}
