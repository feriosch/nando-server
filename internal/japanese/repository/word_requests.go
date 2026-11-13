package repository

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	coreutils "github.com/feriosch/nando-server/internal/core/utils"
	"github.com/feriosch/nando-server/internal/japanese/model"
)

// NewWordRequests returns the WordRequestsRepository over the word_requests
// collection in db.
func NewWordRequests(db *mongo.Database) model.WordRequestsRepository {
	return wordRequests{collection: db.Collection("word_requests")}
}

// wordRequests is the word_requests collection.
type wordRequests struct {
	collection *mongo.Collection
}

// wordRequestDocument is a Word Request as it is stored.
type wordRequestDocument struct {
	ID          bson.ObjectID `bson:"_id"`
	Text        string        `bson:"text"`
	Kind        string        `bson:"kind"`
	Group       string        `bson:"group,omitempty"`
	WordID      string        `bson:"wordId,omitempty"`
	State       string        `bson:"state"`
	Attempts    int           `bson:"attempts"`
	Answer      string        `bson:"answer,omitempty"`
	Summary     string        `bson:"summary,omitempty"`
	Suggestions []string      `bson:"suggestions,omitempty"`
	Question    string        `bson:"question,omitempty"`
	Choices     []string      `bson:"choices,omitempty"`
	CreatedAt   time.Time     `bson:"createdAt"`
	UpdatedAt   time.Time     `bson:"updatedAt"`
	DismissedAt *time.Time    `bson:"dismissedAt,omitempty"`
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

func (r wordRequests) Insert(ctx context.Context, request model.WordRequest) (string, error) {
	id := bson.NewObjectID()
	_, err := r.collection.InsertOne(ctx, wordRequestDocument{
		ID:        id,
		Text:      request.Text,
		Kind:      string(request.Kind),
		Group:     request.Group,
		WordID:    request.WordID,
		State:     string(request.State),
		CreatedAt: request.CreatedAt,
		UpdatedAt: request.UpdatedAt,
	})
	if err != nil {
		return "", err
	}
	return id.Hex(), nil
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

func (r wordRequests) GetOpen(ctx context.Context, text string, kind model.WordRequestKind, wordID string) (model.WordRequest, error) {
	filter := bson.D{
		{Key: "text", Value: text},
		{Key: "kind", Value: string(kind)},
		{Key: "state", Value: bson.D{{Key: "$in", Value: model.OpenStates}}},
	}
	if wordID != "" {
		filter = append(filter, bson.E{Key: "wordId", Value: wordID})
	}
	var doc wordRequestDocument
	err := r.collection.FindOne(ctx, filter).Decode(&doc)
	if err != nil {
		return model.WordRequest{}, coreutils.NotFound(err)
	}
	return doc.wordRequest(), nil
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
