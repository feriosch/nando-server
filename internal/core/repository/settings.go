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

// stored is the settings document as it may be stored: the Gmail Digest was
// first saved as mailDigest (#150), before it was renamed (#157).
type stored struct {
	model.Settings `bson:",inline"`
	MailDigest     *model.GmailDigest `bson:"mailDigest,omitempty"`
}

func (r settings) Find(ctx context.Context) (model.Settings, error) {
	var s stored
	if err := r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: settingsID}}).Decode(&s); err != nil {
		return model.Settings{}, utils.NotFound(err)
	}
	if s.GmailDigest.Time == "" && s.MailDigest != nil {
		// Saved under its old name; Save replaces the document, so the next
		// save stores it under the new one.
		s.GmailDigest = *s.MailDigest
	}
	return s.Settings, nil
}

func (r settings) Save(ctx context.Context, s model.Settings) error {
	_, err := r.collection.ReplaceOne(ctx, bson.D{{Key: "_id", Value: settingsID}}, s, options.Replace().SetUpsert(true))
	return err
}
