package repository

import (
	"context"
	"regexp"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/core/utils"
	"github.com/feriosch/nando-server/internal/money/model"
)

// NewMerchants returns the MerchantsRepository over the merchants collection
// in db, with an index on the raw texts Merchants are found by.
func NewMerchants(ctx context.Context, db *mongo.Database) (model.MerchantsRepository, error) {
	collection := db.Collection("merchants")
	_, err := collection.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "rawTexts", Value: 1}}})
	return merchants{collection: collection}, err
}

// merchants is the merchants collection.
type merchants struct {
	collection *mongo.Collection
}

func (r merchants) Insert(ctx context.Context, m model.Merchant) (string, error) {
	m.ID = bson.NewObjectID().Hex()
	_, err := r.collection.InsertOne(ctx, m)
	return m.ID, err
}

func (r merchants) Find(ctx context.Context, id string) (model.Merchant, error) {
	var m model.Merchant
	err := r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&m)
	return m, utils.NotFound(err)
}

func (r merchants) WithRawText(ctx context.Context, rawText string) (model.Merchant, error) {
	var m model.Merchant
	err := r.collection.FindOne(ctx, bson.D{{Key: "rawTexts", Value: rawText}}).Decode(&m)
	if err == nil || !errorsIsNoDocuments(err) {
		return m, err
	}
	err = r.collection.FindOne(ctx, bson.D{
		{Key: "name", Value: bson.Regex{Pattern: "^" + regexp.QuoteMeta(rawText) + "$", Options: "i"}},
	}).Decode(&m)
	return m, utils.NotFound(err)
}

func (r merchants) List(ctx context.Context) ([]model.Merchant, error) {
	byName := options.Find().
		SetSort(bson.D{{Key: "name", Value: 1}, {Key: "_id", Value: 1}}).
		SetCollation(&options.Collation{Locale: "en", Strength: 2})
	cursor, err := r.collection.Find(ctx, bson.D{}, byName)
	if err != nil {
		return nil, err
	}
	list := []model.Merchant{}
	return list, cursor.All(ctx, &list)
}

func (r merchants) Replace(ctx context.Context, m model.Merchant) error {
	_, err := r.collection.ReplaceOne(ctx, bson.D{{Key: "_id", Value: m.ID}}, m)
	return err
}

func (r merchants) MoveSubcategory(ctx context.Context, from, into, intoCategory string) error {
	return moveSubcategory(ctx, r.collection, from, into, intoCategory)
}
