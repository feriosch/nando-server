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

// NewSubcategories returns the SubcategoriesRepository over the
// subcategories collection in db.
func NewSubcategories(db *mongo.Database) model.SubcategoriesRepository {
	return subcategories{collection: db.Collection("subcategories")}
}

// subcategories is the subcategories collection.
type subcategories struct {
	collection *mongo.Collection
}

func (r subcategories) Insert(ctx context.Context, s model.Subcategory) (string, error) {
	s.ID = bson.NewObjectID().Hex()
	_, err := r.collection.InsertOne(ctx, s)
	return s.ID, err
}

func (r subcategories) Find(ctx context.Context, id string) (model.Subcategory, error) {
	var s model.Subcategory
	err := r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&s)
	return s, utils.NotFound(err)
}

func (r subcategories) Named(ctx context.Context, category, name string) (model.Subcategory, error) {
	var s model.Subcategory
	err := r.collection.FindOne(ctx, bson.D{
		{Key: "category", Value: category},
		{Key: "name", Value: bson.Regex{Pattern: "^" + regexp.QuoteMeta(name) + "$", Options: "i"}},
	}).Decode(&s)
	return s, utils.NotFound(err)
}

func (r subcategories) List(ctx context.Context) ([]model.Subcategory, error) {
	byName := options.Find().
		SetSort(bson.D{{Key: "name", Value: 1}, {Key: "_id", Value: 1}}).
		SetCollation(&options.Collation{Locale: "en", Strength: 2})
	cursor, err := r.collection.Find(ctx, bson.D{}, byName)
	if err != nil {
		return nil, err
	}
	list := []model.Subcategory{}
	return list, cursor.All(ctx, &list)
}

func (r subcategories) Replace(ctx context.Context, s model.Subcategory) error {
	_, err := r.collection.ReplaceOne(ctx, bson.D{{Key: "_id", Value: s.ID}}, s)
	return err
}

func (r subcategories) Delete(ctx context.Context, id string) error {
	_, err := r.collection.DeleteOne(ctx, bson.D{{Key: "_id", Value: id}})
	return err
}

// moveSubcategory points every document of collection whose subcategoryId
// is from at into, in intoCategory, instead.
func moveSubcategory(ctx context.Context, collection *mongo.Collection, from, into, intoCategory string) error {
	_, err := collection.UpdateMany(ctx, bson.D{{Key: "subcategoryId", Value: from}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "subcategoryId", Value: into}, {Key: "category", Value: intoCategory}}}})
	return err
}
