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

// notMarkedReplied matches a Conversation whose Suggested Reply, if any,
// the Owner hasn't marked replied.
var notMarkedReplied = bson.E{Key: "suggestedReply.markedRepliedAt", Value: bson.D{{Key: "$exists", Value: false}}}

// conversations is the conversations collection.
type conversations struct {
	collection *mongo.Collection
}

func (r conversations) AddMessages(ctx context.Context, update model.ThreadUpdate) (model.Added, error) {
	var held model.Conversation
	err := utils.NotFound(r.collection.FindOne(ctx, bson.D{{Key: "threadId", Value: update.ThreadID}}).Decode(&held))
	if err != nil && !errors.Is(err, utils.ErrNotFound) {
		return model.Added{}, err
	}
	// An ignored thread isn't about a role: its messages are dropped.
	if held.Ignored {
		return model.Added{ConversationID: held.ID, Ignored: true}, nil
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
	var after model.Conversation
	err = r.collection.FindOneAndUpdate(ctx, bson.D{{Key: "threadId", Value: update.ThreadID}}, change,
		options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After).
			SetProjection(bson.D{{Key: "messages", Value: 0}})).Decode(&after)
	if err != nil {
		return model.Added{}, err
	}
	if err := r.removeAnsweredReply(ctx, after.ID, added); err != nil {
		return model.Added{}, err
	}
	fromOther := slices.ContainsFunc(added, func(m model.Message) bool { return m.From == model.MessageFromOther })
	return model.Added{ConversationID: after.ID, Stored: len(added), Filed: after.RecruiterID != nil, FromOther: fromOther}, nil
}

// removeAnsweredReply removes the Conversation's Suggested Reply if one of
// the added messages is the Owner's and newer than the message it answers:
// the Owner has replied themselves.
func (r conversations) removeAnsweredReply(ctx context.Context, id bson.ObjectID, added []model.Message) error {
	var ownerNewest time.Time
	for _, m := range added {
		if m.From == model.MessageFromOwner && m.SentAt.After(ownerNewest) {
			ownerNewest = m.SentAt
		}
	}
	if ownerNewest.IsZero() {
		return nil
	}
	_, err := r.collection.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: id}, {Key: "suggestedReply.answers", Value: bson.D{{Key: "$lt", Value: ownerNewest}}}},
		bson.D{{Key: "$unset", Value: bson.D{{Key: "suggestedReply", Value: ""}}}},
	)
	return err
}

func (r conversations) GetByID(ctx context.Context, id bson.ObjectID) (model.Conversation, error) {
	var c model.Conversation
	err := r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&c)
	return c, utils.NotFound(err)
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

func (r conversations) Ignore(ctx context.Context, id bson.ObjectID) (model.Conversation, error) {
	var c model.Conversation
	err := r.collection.FindOneAndUpdate(ctx,
		bson.D{{Key: "_id", Value: id}},
		bson.D{
			{Key: "$set", Value: bson.D{{Key: "ignored", Value: true}, {Key: "messages", Value: bson.A{}}}},
			{Key: "$unset", Value: bson.D{
				{Key: "url", Value: ""},
				{Key: "person", Value: ""},
				{Key: "recruiterId", Value: ""},
				{Key: "suggestedReply", Value: ""},
			}},
		},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&c)
	return c, utils.NotFound(err)
}

func (r conversations) File(ctx context.Context, id, recruiterID bson.ObjectID) (model.Conversation, error) {
	var c model.Conversation
	err := r.collection.FindOneAndUpdate(ctx,
		bson.D{{Key: "_id", Value: id}, {Key: "ignored", Value: false}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "recruiterId", Value: recruiterID}}}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&c)
	return c, utils.NotFound(err)
}

func (r conversations) ByRecruiter(ctx context.Context, recruiterID bson.ObjectID) ([]model.Conversation, error) {
	cursor, err := r.collection.Find(ctx, bson.D{{Key: "recruiterId", Value: recruiterID}})
	if err != nil {
		return nil, err
	}
	filed := []model.Conversation{}
	return filed, cursor.All(ctx, &filed)
}

func (r conversations) WithSuggestedReply(ctx context.Context, kinds []string) ([]model.Conversation, error) {
	cursor, err := r.collection.Find(ctx, bson.D{
		{Key: "recruiterId", Value: bson.D{{Key: "$exists", Value: true}}},
		{Key: "suggestedReply.kind", Value: bson.D{{Key: "$in", Value: kinds}}},
		notMarkedReplied,
	})
	if err != nil {
		return nil, err
	}
	found := []model.Conversation{}
	return found, cursor.All(ctx, &found)
}

func (r conversations) SetSuggestedReply(ctx context.Context, id bson.ObjectID, reply model.SuggestedReply) (model.Conversation, error) {
	var c model.Conversation
	err := r.collection.FindOneAndUpdate(ctx,
		bson.D{{Key: "_id", Value: id}, {Key: "ignored", Value: false}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "suggestedReply", Value: reply}}}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&c)
	return c, utils.NotFound(err)
}

func (r conversations) WithWaitingReplyBefore(ctx context.Context, before time.Time) ([]model.Conversation, error) {
	cursor, err := r.collection.Find(ctx, bson.D{
		{Key: "suggestedReply.kind", Value: bson.D{{Key: "$in", Value: model.WaitingReplyKinds}}},
		{Key: "suggestedReply.answers", Value: bson.D{{Key: "$lt", Value: before}}},
		notMarkedReplied,
	})
	if err != nil {
		return nil, err
	}
	found := []model.Conversation{}
	return found, cursor.All(ctx, &found)
}

func (r conversations) MakeReplyLate(ctx context.Context, id bson.ObjectID, reply model.SuggestedReply) (bool, error) {
	result, err := r.collection.UpdateOne(ctx,
		bson.D{
			{Key: "_id", Value: id},
			{Key: "suggestedReply.kind", Value: reply.Kind},
			{Key: "suggestedReply.writtenAt", Value: reply.WrittenAt},
			notMarkedReplied,
		},
		bson.D{{Key: "$set", Value: bson.D{{Key: "suggestedReply.kind", Value: model.ReplyLate}}}},
	)
	if err != nil {
		return false, err
	}
	return result.ModifiedCount == 1, nil
}

func (r conversations) MarkReplied(ctx context.Context, id bson.ObjectID, at time.Time) error {
	_, err := r.collection.UpdateOne(ctx,
		bson.D{
			{Key: "_id", Value: id},
			{Key: "suggestedReply", Value: bson.D{{Key: "$exists", Value: true}}},
			{Key: "suggestedReply.markedRepliedAt", Value: bson.D{{Key: "$exists", Value: false}}},
		},
		bson.D{{Key: "$set", Value: bson.D{{Key: "suggestedReply.markedRepliedAt", Value: at}}}},
	)
	return err
}

func (r conversations) UnmarkReplied(ctx context.Context, id bson.ObjectID) error {
	return utils.Matched(r.collection.UpdateByID(ctx, id, bson.D{{Key: "$unset", Value: bson.D{{Key: "suggestedReply.markedRepliedAt", Value: ""}}}}))
}

func (r conversations) MarkFollowedUp(ctx context.Context, id bson.ObjectID, sentAt time.Time) error {
	return utils.Matched(r.collection.UpdateByID(ctx, id, bson.D{{Key: "$set", Value: bson.D{{Key: "followedUp", Value: sentAt}}}}))
}

func (r conversations) AnyAnswerFromSync(ctx context.Context, syncID bson.ObjectID) (bool, error) {
	err := r.collection.FindOne(ctx,
		bson.D{{Key: "suggestedReply.kind", Value: model.ReplyAnswer}, {Key: "suggestedReply.syncId", Value: syncID}, notMarkedReplied},
		options.FindOne().SetProjection(bson.D{{Key: "_id", Value: 1}}),
	).Err()
	if errors.Is(err, mongo.ErrNoDocuments) {
		return false, nil
	}
	return err == nil, err
}

func (r conversations) WaitingRecruiters(ctx context.Context, since time.Time) (int, error) {
	waiting, err := r.collection.Distinct(ctx, "recruiterId", bson.D{
		{Key: "recruiterId", Value: bson.D{{Key: "$exists", Value: true}}},
		{Key: "suggestedReply.kind", Value: bson.D{{Key: "$in", Value: model.WaitingReplyKinds}}},
		{Key: "suggestedReply.answers", Value: bson.D{{Key: "$gte", Value: since}}},
		notMarkedReplied,
	}).Raw()
	if err != nil {
		return 0, err
	}
	values, err := waiting.Values()
	return len(values), err
}
