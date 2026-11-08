package repository

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/calories/model"
	coremodel "github.com/feriosch/nando-server/internal/core/model"
)

const entries = "entries"

var entryIndexes = []mongo.IndexModel{
	{Keys: bson.D{{Key: "day", Value: 1}, {Key: "loggedAt", Value: 1}}},
}

func (r mongoRepository) InsertEntry(ctx context.Context, entry model.Entry) (string, error) {
	entry.ID = bson.NewObjectID().Hex()
	_, err := r.db.Collection(entries).InsertOne(ctx, entry)
	return entry.ID, err
}

func (r mongoRepository) ListEntries(ctx context.Context, day coremodel.Day) ([]model.Entry, error) {
	cursor, err := r.db.Collection(entries).Find(ctx, bson.D{{Key: "day", Value: day}},
		options.Find().SetSort(bson.D{{Key: "loggedAt", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	list := []model.Entry{}
	return list, cursor.All(ctx, &list)
}

func (r mongoRepository) FindEntry(ctx context.Context, id string) (model.Entry, error) {
	var entry model.Entry
	err := r.db.Collection(entries).FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&entry)
	return entry, notFound(err)
}

func (r mongoRepository) UpdateEntry(ctx context.Context, entry model.Entry) error {
	result, err := r.db.Collection(entries).UpdateByID(ctx, entry.ID, bson.D{{Key: "$set", Value: bson.D{
		{Key: "name", Value: entry.Name},
		{Key: "kcalPerServing", Value: entry.KcalPerServing},
		{Key: "servings", Value: entry.Servings},
	}}})
	return matched(result, err)
}

func (r mongoRepository) DeleteEntry(ctx context.Context, id string) error {
	_, err := r.db.Collection(entries).DeleteOne(ctx, bson.D{{Key: "_id", Value: id}})
	return err
}

func (r mongoRepository) CountEntriesByMeal(ctx context.Context, first, last coremodel.Day) (map[string]int, error) {
	cursor, err := r.db.Collection(entries).Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: bson.D{
			{Key: "day", Value: bson.D{{Key: "$gte", Value: first}, {Key: "$lte", Value: last}}},
			{Key: "mealId", Value: bson.D{{Key: "$ne", Value: ""}}},
		}}},
		{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$mealId"}, {Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}}}}},
	})
	if err != nil {
		return nil, err
	}
	var rows []struct {
		MealID string `bson:"_id"`
		Count  int    `bson:"count"`
	}
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, err
	}
	counts := make(map[string]int, len(rows))
	for _, row := range rows {
		counts[row.MealID] = row.Count
	}
	return counts, nil
}
