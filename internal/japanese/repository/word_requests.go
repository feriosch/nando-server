package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	coreutils "github.com/feriosch/nando-server/internal/core/utils"
	"github.com/feriosch/nando-server/internal/japanese/model"
)

// NewWordRequests returns the WordRequestsRepository over the word_requests
// collection in db, first creating any indexes it lacks.
func NewWordRequests(ctx context.Context, db *mongo.Database) (model.WordRequestsRepository, error) {
	collection := db.Collection("word_requests")
	// At most one open Word Request asks for the same thing.
	_, err := collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "key", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("one_open_per_key").
			SetPartialFilterExpression(bson.D{{Key: "state", Value: bson.D{{Key: "$in", Value: model.OpenStates}}}}),
	})
	if err != nil {
		return nil, err
	}
	return wordRequests{collection: collection}, nil
}

// askKey is what a Word Request asks for: the text to add, or the Word to
// improve, whose spelling may change while it is open.
func askKey(request model.WordRequest) string {
	if request.Kind == model.ImproveWord {
		return string(model.ImproveWord) + ":" + request.WordID
	}
	return string(request.Kind) + ":" + request.Text
}

// wordRequests is the word_requests collection.
type wordRequests struct {
	collection *mongo.Collection
}

// wordRequestDocument is a Word Request as it is stored.
type wordRequestDocument struct {
	ID bson.ObjectID `bson:"_id"`
	// Key is what it asks for (askKey), unique among open Word Requests.
	Key         string     `bson:"key"`
	Text        string     `bson:"text"`
	Kind        string     `bson:"kind"`
	Group       string     `bson:"group,omitempty"`
	WordID      string     `bson:"wordId,omitempty"`
	State       string     `bson:"state"`
	Attempts    int        `bson:"attempts"`
	Answer      string     `bson:"answer,omitempty"`
	Summary     string     `bson:"summary,omitempty"`
	Suggestions []string   `bson:"suggestions,omitempty"`
	Question    string     `bson:"question,omitempty"`
	Choices     []string   `bson:"choices,omitempty"`
	CreatedAt   time.Time  `bson:"createdAt"`
	UpdatedAt   time.Time  `bson:"updatedAt"`
	DismissedAt *time.Time `bson:"dismissedAt,omitempty"`
}

func (d wordRequestDocument) wordRequest() model.WordRequest {
	return model.WordRequest{
		ID:          d.ID.Hex(),
		Text:        d.Text,
		Kind:        model.WordRequestKind(d.Kind),
		Group:       d.Group,
		WordID:      d.WordID,
		State:       model.WordRequestState(d.State),
		Attempts:    d.Attempts,
		Answer:      d.Answer,
		Summary:     d.Summary,
		Suggestions: d.Suggestions,
		Question:    d.Question,
		Choices:     d.Choices,
		CreatedAt:   d.CreatedAt,
		UpdatedAt:   d.UpdatedAt,
	}
}

func (r wordRequests) InsertUnlessOpen(ctx context.Context, request model.WordRequest) (model.WordRequest, bool, error) {
	key := askKey(request)
	id := bson.NewObjectID()
	_, err := r.collection.InsertOne(ctx, wordRequestDocument{
		ID:        id,
		Key:       key,
		Text:      request.Text,
		Kind:      string(request.Kind),
		Group:     request.Group,
		WordID:    request.WordID,
		State:     string(request.State),
		CreatedAt: request.CreatedAt,
		UpdatedAt: request.UpdatedAt,
	})
	if err == nil {
		request.ID = id.Hex()
		return request, true, nil
	}
	if !mongo.IsDuplicateKeyError(err) {
		return model.WordRequest{}, false, err
	}
	var open wordRequestDocument
	err = r.collection.FindOne(ctx, bson.D{
		{Key: "key", Value: key},
		{Key: "state", Value: bson.D{{Key: "$in", Value: model.OpenStates}}},
	}).Decode(&open)
	if err != nil {
		// It finished between the insert and this read: rare enough to
		// let the Owner tap again.
		return model.WordRequest{}, false, err
	}
	return open.wordRequest(), false, nil
}

func (r wordRequests) GetListed(ctx context.Context, finishedAfter time.Time) ([]model.WordRequest, error) {
	filter := bson.D{
		{Key: "dismissedAt", Value: bson.D{{Key: "$exists", Value: false}}},
		{Key: "$or", Value: bson.A{
			bson.D{{Key: "state", Value: bson.D{{Key: "$in", Value: model.OpenStates}}}},
			// A finished Word Request was last updated when it finished.
			bson.D{{Key: "updatedAt", Value: bson.D{{Key: "$gt", Value: finishedAfter}}}},
		}},
	}
	cursor, err := r.collection.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}, {Key: "_id", Value: -1}}))
	if err != nil {
		return nil, err
	}
	var docs []wordRequestDocument
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}
	requests := make([]model.WordRequest, len(docs))
	for i, d := range docs {
		requests[i] = d.wordRequest()
	}
	return requests, nil
}

func (r wordRequests) Update(ctx context.Context, id string, from []model.WordRequestState, changes model.WordRequestChanges) (model.WordRequest, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return model.WordRequest{}, coreutils.ErrNotFound
	}
	set := bson.D{{Key: "updatedAt", Value: changes.UpdatedAt}}
	if changes.State != nil {
		set = append(set, bson.E{Key: "state", Value: string(*changes.State)})
	}
	if changes.Answer != nil {
		set = append(set, bson.E{Key: "answer", Value: *changes.Answer})
	}
	if changes.Attempts != nil {
		set = append(set, bson.E{Key: "attempts", Value: *changes.Attempts})
	}
	if changes.WordID != nil {
		set = append(set, bson.E{Key: "wordId", Value: *changes.WordID})
	}
	if changes.Summary != nil {
		set = append(set, bson.E{Key: "summary", Value: *changes.Summary})
	}
	if changes.Suggestions != nil {
		set = append(set, bson.E{Key: "suggestions", Value: *changes.Suggestions})
	}
	if changes.Question != nil {
		set = append(set, bson.E{Key: "question", Value: *changes.Question})
	}
	if changes.Choices != nil {
		set = append(set, bson.E{Key: "choices", Value: *changes.Choices})
	}
	if changes.DismissedAt != nil {
		set = append(set, bson.E{Key: "dismissedAt", Value: *changes.DismissedAt})
	}
	var doc wordRequestDocument
	err = r.collection.FindOneAndUpdate(ctx,
		bson.D{{Key: "_id", Value: oid}, {Key: "state", Value: bson.D{{Key: "$in", Value: from}}}},
		bson.D{{Key: "$set", Value: set}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		// Either there's no such Word Request or it is in another state.
		count, err := r.collection.CountDocuments(ctx, bson.D{{Key: "_id", Value: oid}})
		if err != nil {
			return model.WordRequest{}, err
		}
		if count == 0 {
			return model.WordRequest{}, coreutils.ErrNotFound
		}
		return model.WordRequest{}, model.ErrWrongState
	}
	if err != nil {
		return model.WordRequest{}, err
	}
	return doc.wordRequest(), nil
}

// runOut matches the Working Word Requests whose claim has run out by now.
func runOut(now time.Time) bson.D {
	return bson.D{
		{Key: "state", Value: string(model.Working)},
		{Key: "claimedUntil", Value: bson.D{{Key: "$lte", Value: now}}},
	}
}

func (r wordRequests) FailUnfinished(ctx context.Context, now time.Time, maxAttempts int) error {
	filter := append(runOut(now), bson.E{Key: "attempts", Value: bson.D{{Key: "$gte", Value: maxAttempts}}})
	_, err := r.collection.UpdateMany(ctx, filter, bson.D{{Key: "$set", Value: bson.D{
		{Key: "state", Value: string(model.Failed)},
		{Key: "summary", Value: fmt.Sprintf("No Claude run finished it in %d tries.", maxAttempts)},
		{Key: "updatedAt", Value: now},
	}}})
	return err
}

// claimable matches the Word Requests a Claude run may claim: Pending, or
// Working with its claim run out by now and fewer than maxAttempts claims.
func claimable(now time.Time, maxAttempts int) bson.D {
	return bson.D{{Key: "$or", Value: bson.A{
		bson.D{{Key: "state", Value: string(model.Pending)}},
		// A run that stopped without resolving it lets it go, while it
		// has claims left.
		append(runOut(now), bson.E{Key: "attempts", Value: bson.D{{Key: "$lt", Value: maxAttempts}}}),
	}}}
}

func (r wordRequests) ClaimNext(ctx context.Context, now, claimedUntil time.Time, maxAttempts int) (model.WordRequest, error) {
	var doc wordRequestDocument
	err := r.collection.FindOneAndUpdate(ctx,
		claimable(now, maxAttempts),
		bson.D{
			{Key: "$set", Value: bson.D{
				{Key: "state", Value: string(model.Working)},
				{Key: "claimedUntil", Value: claimedUntil},
				{Key: "updatedAt", Value: now},
			}},
			{Key: "$inc", Value: bson.D{{Key: "attempts", Value: 1}}},
		},
		options.FindOneAndUpdate().
			SetSort(bson.D{{Key: "createdAt", Value: 1}, {Key: "_id", Value: 1}}).
			SetReturnDocument(options.After),
	).Decode(&doc)
	if err != nil {
		return model.WordRequest{}, coreutils.NotFound(err)
	}
	return doc.wordRequest(), nil
}

func (r wordRequests) AnyClaimable(ctx context.Context, now time.Time, maxAttempts int) (bool, error) {
	return r.any(ctx, claimable(now, maxAttempts))
}

func (r wordRequests) AnyClaimed(ctx context.Context, now time.Time) (bool, error) {
	return r.any(ctx, bson.D{
		{Key: "state", Value: string(model.Working)},
		{Key: "claimedUntil", Value: bson.D{{Key: "$gt", Value: now}}},
	})
}

// any reports whether a Word Request matches filter.
func (r wordRequests) any(ctx context.Context, filter bson.D) (bool, error) {
	err := r.collection.FindOne(ctx, filter, options.FindOne().SetProjection(bson.D{{Key: "_id", Value: 1}})).Err()
	if errors.Is(err, mongo.ErrNoDocuments) {
		return false, nil
	}
	return err == nil, err
}

func (r wordRequests) GetOneByID(ctx context.Context, id string) (model.WordRequest, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return model.WordRequest{}, coreutils.ErrNotFound
	}
	var doc wordRequestDocument
	if err := r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: oid}}).Decode(&doc); err != nil {
		return model.WordRequest{}, coreutils.NotFound(err)
	}
	return doc.wordRequest(), nil
}
