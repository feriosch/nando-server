package repository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/japanese/model"
)

// NewClaudeWakes returns the ClaudeWakesRepository over the claude_wakes
// collection in db, which holds one document: the last wake.
func NewClaudeWakes(db *mongo.Database) model.ClaudeWakesRepository {
	return claudeWakes{collection: db.Collection("claude_wakes")}
}

// claudeWakes is the claude_wakes collection.
type claudeWakes struct {
	collection *mongo.Collection
}

// lastWakeID is the _id of the one document, the last wake.
const lastWakeID = "last"

func (r claudeWakes) Claim(ctx context.Context, at, since time.Time) (bool, error) {
	_, err := r.collection.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: lastWakeID}, {Key: "wokenAt", Value: bson.D{{Key: "$lte", Value: since}}}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "wokenAt", Value: at}}}},
		options.UpdateOne().SetUpsert(true),
	)
	if mongo.IsDuplicateKeyError(err) {
		// The last wake is after since, so the upsert tried a second one.
		return false, nil
	}
	return err == nil, err
}
