package repository

import (
	"context"
	"errors"
	"slices"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/career/model"
	"github.com/feriosch/nando-server/internal/core/utils"
)

// NewConversations returns the ConversationsRepository over the conversations
// collection in db, with one Conversation per LinkedIn thread.
func NewConversations(ctx context.Context, db *mongo.Database) (model.ConversationsRepository, error) {
	collection := db.Collection("conversations")
	_, err := collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "threadId", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return nil, err
	}
	return conversations{collection: collection}, nil
}

// conversations is the conversations collection.
type conversations struct {
	collection *mongo.Collection
}

func (r conversations) AddMessages(ctx context.Context, update model.ThreadUpdate) (int, error) {
	var held model.Conversation
	err := utils.NotFound(r.collection.FindOne(ctx, bson.D{{Key: "threadId", Value: update.ThreadID}},
		options.FindOne().SetProjection(bson.D{{Key: "messages", Value: 1}})).Decode(&held))
	if err != nil && !errors.Is(err, utils.ErrNotFound) {
		return 0, err
	}
	// Keep only messages Nando doesn't hold, and each of them once, even if
	// the Sync sent it twice.
	var added []model.Message
	for _, m := range update.Messages {
		isHeld := func(h model.Message) bool { return sameMessage(h, m) }
		if !slices.ContainsFunc(held.Messages, isHeld) && !slices.ContainsFunc(added, isHeld) {
			added = append(added, m)
		}
	}

	// A Sync that leaves out the URL or the person keeps what Nando has.
	set := bson.D{}
	if update.URL != "" {
		set = append(set, bson.E{Key: "url", Value: update.URL})
	}
	if update.Person != (model.Person{}) {
		set = append(set, bson.E{Key: "person", Value: update.Person})
	}
	change := bson.D{{Key: "$setOnInsert", Value: bson.D{{Key: "ignored", Value: false}}}}
	if len(set) > 0 {
		change = append(change, bson.E{Key: "$set", Value: set})
	}
	if len(added) > 0 {
		newest := added[0].SentAt
		for _, m := range added[1:] {
			if m.SentAt.After(newest) {
				newest = m.SentAt
			}
		}
		change = append(change,
			bson.E{Key: "$push", Value: bson.D{{Key: "messages", Value: bson.D{{Key: "$each", Value: added}}}}},
			bson.E{Key: "$max", Value: bson.D{{Key: "newest", Value: newest}}},
		)
	}
	_, err = r.collection.UpdateOne(ctx, bson.D{{Key: "threadId", Value: update.ThreadID}}, change, options.UpdateOne().SetUpsert(true))
	if err != nil {
		return 0, err
	}
	return len(added), nil
}

func (r conversations) Threads(ctx context.Context) ([]model.Conversation, error) {
	cursor, err := r.collection.Find(ctx, bson.D{}, options.Find().SetProjection(bson.D{{Key: "messages", Value: 0}}))
	if err != nil {
		return nil, err
	}
	threads := []model.Conversation{}
	return threads, cursor.All(ctx, &threads)
}

// sameMessage reports whether held, a message Nando holds, is m: the same
// LinkedIn id, or, when either has none, the same sender, time and text.
// MongoDB keeps times to the millisecond.
func sameMessage(held, m model.Message) bool {
	if held.LinkedInID != "" && m.LinkedInID != "" {
		return held.LinkedInID == m.LinkedInID
	}
	return held.From == m.From && held.Text == m.Text &&
		held.SentAt.Truncate(time.Millisecond).Equal(m.SentAt.Truncate(time.Millisecond))
}
