package repository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/calories/model"
)

const meals = "meals"

func (r mongoRepository) InsertMeal(ctx context.Context, meal model.Meal) (string, error) {
	meal.ID = bson.NewObjectID().Hex()
	_, err := r.db.Collection(meals).InsertOne(ctx, meal)
	return meal.ID, err
}

func (r mongoRepository) ListMeals(ctx context.Context, archived bool) ([]model.Meal, error) {
	// archivedAt: null matches a missing field too.
	filter := bson.D{{Key: "archivedAt", Value: nil}}
	if archived {
		filter = bson.D{{Key: "archivedAt", Value: bson.D{{Key: "$ne", Value: nil}}}}
	}
	cursor, err := r.db.Collection(meals).Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
	if err != nil {
		return nil, err
	}
	list := []model.Meal{}
	return list, cursor.All(ctx, &list)
}

func (r mongoRepository) FindMeal(ctx context.Context, id string) (model.Meal, error) {
	var meal model.Meal
	err := r.db.Collection(meals).FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&meal)
	return meal, notFound(err)
}

func (r mongoRepository) UpdateMeal(ctx context.Context, meal model.Meal) error {
	result, err := r.db.Collection(meals).UpdateByID(ctx, meal.ID, bson.D{{Key: "$set", Value: bson.D{
		{Key: "name", Value: meal.Name},
		{Key: "kcal", Value: meal.Kcal},
		{Key: "store", Value: meal.Store},
		{Key: "emoji", Value: meal.Emoji},
		{Key: "imageUrl", Value: meal.ImageURL},
		{Key: "tags", Value: meal.Tags},
		{Key: "pinned", Value: meal.Pinned},
	}}})
	return matched(result, err)
}

func (r mongoRepository) SetMealArchivedAt(ctx context.Context, id string, archivedAt *time.Time) error {
	result, err := r.db.Collection(meals).UpdateByID(ctx, id, bson.D{{Key: "$set", Value: bson.D{{Key: "archivedAt", Value: archivedAt}}}})
	return matched(result, err)
}
