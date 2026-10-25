package repository

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/core/model"
)

const settings = "settings"

// settingsID is the _id of the only settings document.
const settingsID = "settings"

func (r mongoRepository) FindSettings(ctx context.Context) (model.Settings, error) {
	var s model.Settings
	err := r.db.Collection(settings).FindOne(ctx, bson.D{{Key: "_id", Value: settingsID}}).Decode(&s)
	return s, notFound(err)
}

func (r mongoRepository) SaveSettings(ctx context.Context, s model.Settings) error {
	_, err := r.db.Collection(settings).ReplaceOne(ctx, bson.D{{Key: "_id", Value: settingsID}}, s, options.Replace().SetUpsert(true))
	return err
}
