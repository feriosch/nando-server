package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/core/utils"
)

// NewTalks returns the TalksRepository over the talks collection in db, first
// creating any indexes it lacks.
func NewTalks(ctx context.Context, db *mongo.Database) (model.TalksRepository, error) {
	collection := db.Collection("talks")
	// Each Assistant has at most one open Talk.
	if _, err := collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "assistant", Value: 1}},
		Options: options.Index().SetUnique(true).SetPartialFilterExpression(bson.D{{Key: "open", Value: true}}),
	}); err != nil {
		return nil, err
	}
	updates := db.Collection("telegram_updates")
	if _, err := updates.Indexes().CreateMany(ctx, []mongo.IndexModel{utils.ExpiresAtIndex}); err != nil {
		return nil, err
	}
	return talks{collection: collection, updates: updates}, nil
}

// talks is the talks collection: one document per Talk, with its messages,
// and open: true on each Assistant's open one.
type talks struct {
	collection *mongo.Collection
	// updates is the telegram_updates collection: one document per update a
	// bot had, by "<assistant>:<update id>", until it expires.
	updates *mongo.Collection
}

func (r talks) Open(ctx context.Context, assistant string) (model.Talk, error) {
	var talk model.Talk
	err := r.collection.FindOne(ctx, openTalk(assistant)).Decode(&talk)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return model.Talk{Assistant: assistant, Messages: []model.TalkMessage{}}, nil
	}
	return talk, err
}

func (r talks) AddOwnerMessage(ctx context.Context, assistant string, m model.TalkMessage) error {
	push := func() error {
		_, err := r.collection.UpdateOne(ctx, openTalk(assistant), bson.D{
			{Key: "$push", Value: bson.D{{Key: "messages", Value: m}}},
		}, options.UpdateOne().SetUpsert(true))
		return err
	}
	err := push()
	if mongo.IsDuplicateKeyError(err) {
		// Another message opened the Talk at the same moment: it exists now.
		err = push()
	}
	return err
}

// openTalk matches assistant's open Talk.
func openTalk(assistant string) bson.D {
	return bson.D{{Key: "assistant", Value: assistant}, {Key: "open", Value: true}}
}

func (r talks) MarkUpdate(ctx context.Context, assistant string, updateID int64, expiresAt time.Time) (bool, error) {
	_, err := r.updates.InsertOne(ctx, bson.D{
		{Key: "_id", Value: updateKey(assistant, updateID)},
		{Key: "expiresAt", Value: expiresAt},
	})
	if mongo.IsDuplicateKeyError(err) {
		return false, nil
	}
	return err == nil, err
}

func (r talks) UnmarkUpdate(ctx context.Context, assistant string, updateID int64) error {
	_, err := r.updates.DeleteOne(ctx, bson.D{{Key: "_id", Value: updateKey(assistant, updateID)}})
	return err
}

// updateKey identifies one update to assistant's bot: each bot numbers its
// own.
func updateKey(assistant string, updateID int64) string {
	return fmt.Sprintf("%s:%d", assistant, updateID)
}
