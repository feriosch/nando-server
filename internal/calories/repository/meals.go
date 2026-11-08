package repository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/calories/model"
	"github.com/feriosch/nando-server/internal/core/utils"
)

// NewMeals returns the MealsRepository over the meals collection in db.
func NewMeals(db *mongo.Database) model.MealsRepository {
	return meals{collection: db.Collection("meals")}
}

// meals is the meals collection.
type meals struct {
	collection *mongo.Collection
}

func (r meals) Insert(ctx context.Context, meal model.Meal) (string, error) {
	meal.ID = bson.NewObjectID().Hex()
	_, err := r.collection.InsertOne(ctx, meal)
	return meal.ID, err
}

func (r meals) List(ctx context.Context, archived bool) ([]model.Meal, error) {
	// archivedAt: null matches a missing field too.
	filter := bson.D{{Key: "archivedAt", Value: nil}}
	if archived {
		filter = bson.D{{Key: "archivedAt", Value: bson.D{{Key: "$ne", Value: nil}}}}
	}
	cursor, err := r.collection.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
	if err != nil {
		return nil, err
	}
	list := []model.Meal{}
	return list, cursor.All(ctx, &list)
}

func (r meals) Find(ctx context.Context, id string) (model.Meal, error) {
	var meal model.Meal
	err := r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&meal)
	return meal, utils.NotFound(err)
}

func (r meals) Update(ctx context.Context, meal model.Meal) error {
	result, err := r.collection.UpdateByID(ctx, meal.ID, bson.D{{Key: "$set", Value: bson.D{
		{Key: "name", Value: meal.Name},
		{Key: "kcal", Value: meal.Kcal},
		{Key: "store", Value: meal.Store},
		{Key: "emoji", Value: meal.Emoji},
		{Key: "imageUrl", Value: meal.ImageURL},
		{Key: "tags", Value: meal.Tags},
		{Key: "pinned", Value: meal.Pinned},
	}}})
	return utils.Matched(result, err)
}

func (r meals) SetArchivedAt(ctx context.Context, id string, archivedAt *time.Time) error {
	result, err := r.collection.UpdateByID(ctx, id, bson.D{{Key: "$set", Value: bson.D{{Key: "archivedAt", Value: archivedAt}}}})
	return utils.Matched(result, err)
}
