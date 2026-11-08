package repository

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/core/utils"
)

// settingsID is the _id of the only settings document.
const settingsID = "settings"

// NewSettings returns the SettingsRepository over the settings collection in db.
func NewSettings(db *mongo.Database) model.SettingsRepository {
	return settings{collection: db.Collection("settings")}
}

// settings is the settings collection.
type settings struct {
	collection *mongo.Collection
}

func (r settings) Find(ctx context.Context) (model.Settings, error) {
	var s model.Settings
	err := r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: settingsID}}).Decode(&s)
	return s, utils.NotFound(err)
}

func (r settings) Save(ctx context.Context, s model.Settings) error {
	_, err := r.collection.ReplaceOne(ctx, bson.D{{Key: "_id", Value: settingsID}}, s, options.Replace().SetUpsert(true))
	return err
}
