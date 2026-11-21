package repository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/career/model"
	"github.com/feriosch/nando-server/internal/core/utils"
)

// NewGmailPasses returns the GmailPassesRepository over the gmail_passes
// collection in db.
func NewGmailPasses(db *mongo.Database) model.GmailPassesRepository {
	return gmailPasses{collection: db.Collection("gmail_passes")}
}

// gmailPasses is the gmail_passes collection.
type gmailPasses struct {
	collection *mongo.Collection
}

func (r gmailPasses) Start(ctx context.Context, at time.Time) (model.GmailPass, error) {
	pass := model.GmailPass{ID: bson.NewObjectID(), StartedAt: at}
	_, err := r.collection.InsertOne(ctx, pass)
	return pass, err
}

func (r gmailPasses) Finish(ctx context.Context, id bson.ObjectID, at time.Time) error {
	result, err := r.collection.UpdateOne(ctx, bson.D{{Key: "_id", Value: id}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "finishedAt", Value: at}}}})
	if err == nil && result.MatchedCount == 0 {
		return utils.ErrNotFound
	}
	return err
}

func (r gmailPasses) LastFinished(ctx context.Context) (model.GmailPass, error) {
	var pass model.GmailPass
	err := r.collection.FindOne(ctx, bson.D{{Key: "finishedAt", Value: bson.D{{Key: "$exists", Value: true}}}},
		options.FindOne().SetSort(bson.D{{Key: "startedAt", Value: -1}})).Decode(&pass)
	return pass, utils.NotFound(err)
}
