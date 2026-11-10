package repository

import (
	"context"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	coreutils "github.com/feriosch/nando-server/internal/core/utils"
	"github.com/feriosch/nando-server/internal/japanese/model"
	"github.com/feriosch/nando-server/internal/japanese/utils"
)

// NewKanji returns the KanjiRepository over the kanji collection in db.
func NewKanji(db *mongo.Database) model.KanjiRepository {
	return kanji{collection: db.Collection("kanji")}
}

// kanji is the kanji collection.
type kanji struct {
	collection *mongo.Collection
}

// kanjiDocument is a Kanji as it is stored.
type kanjiDocument struct {
	ID         bson.ObjectID `bson:"_id"`
	Kanji      string        `bson:"kanji"`
	Spanish    string        `bson:"spanish"`
	On         string        `bson:"on"`
	Kun        string        `bson:"kun"`
	V1         int           `bson:"v1"`
	V2         int           `bson:"v2"`
	Story      string        `bson:"story"`
	Components []string      `bson:"components"`
}

func (d kanjiDocument) toKanji() model.Kanji {
	return model.Kanji{
		ID:         d.ID.Hex(),
		Kanji:      d.Kanji,
		Spanish:    d.Spanish,
		On:         d.On,
		Kun:        d.Kun,
		V1:         d.V1,
		V2:         d.V2,
		Story:      d.Story,
		Components: d.Components,
	}
}

// kanjiRowFields projects what a list shows of a Kanji.
var kanjiRowFields = bson.D{{Key: "kanji", Value: 1}, {Key: "spanish", Value: 1}}

func (r kanji) GetManyBySearch(ctx context.Context, q string, limit int) ([]model.Kanji, error) {
	pattern := utils.SearchPattern(q)
	characters := strings.Split(q, "")
	// A Kanji whose character is in q matches it exactly.
	inQ := bson.D{{Key: "$in", Value: bson.A{"$kanji", literal(characters)}}}
	cursor, err := r.collection.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: bson.D{{Key: "$or", Value: bson.A{
			bson.D{{Key: "kanji", Value: bson.D{{Key: "$in", Value: characters}}}},
			utils.MatchesAny(pattern, "spanish"),
		}}}}},
		{{Key: "$addFields", Value: bson.D{
			{Key: "_exact", Value: bson.D{{Key: "$or", Value: bson.A{inQ, utils.IsExact(pattern, "spanish")}}}},
			{Key: "_length", Value: bson.D{{Key: "$cond", Value: bson.A{inQ, 1, utils.MatchedLength(pattern, "spanish")}}}},
			// The Kanji of a Word come in the order it is spelled.
			{Key: "_position", Value: positionIn(q)},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "_exact", Value: -1}, {Key: "_length", Value: 1}, {Key: "_position", Value: 1}, {Key: "v1", Value: 1}, {Key: "_id", Value: 1}}}},
		{{Key: "$limit", Value: limit}},
		{{Key: "$project", Value: kanjiRowFields}},
	})
	if err != nil {
		return nil, err
	}
	return toKanji(ctx, cursor)
}

func (r kanji) GetManyBySpelling(ctx context.Context, spelling string) ([]model.Kanji, error) {
	cursor, err := r.collection.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: bson.D{{Key: "kanji", Value: bson.D{{Key: "$in", Value: strings.Split(spelling, "")}}}}}},
		{{Key: "$addFields", Value: bson.D{{Key: "_position", Value: positionIn(spelling)}}}},
		{{Key: "$sort", Value: bson.D{{Key: "_position", Value: 1}, {Key: "_id", Value: 1}}}},
		{{Key: "$project", Value: kanjiRowFields}},
	})
	if err != nil {
		return nil, err
	}
	return toKanji(ctx, cursor)
}

func (r kanji) GetOneByID(ctx context.Context, id string) (model.Kanji, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return model.Kanji{}, coreutils.ErrNotFound
	}
	var d kanjiDocument
	if err := r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: oid}}).Decode(&d); err != nil {
		return model.Kanji{}, coreutils.NotFound(err)
	}
	return d.toKanji(), nil
}

func (r kanji) GetManyByKeywords(ctx context.Context, keywords []string) ([]model.Kanji, error) {
	cursor, err := r.collection.Find(ctx, bson.D{{Key: "spanish", Value: bson.D{{Key: "$in", Value: keywords}}}}, options.Find().SetProjection(kanjiRowFields))
	if err != nil {
		return nil, err
	}
	return toKanji(ctx, cursor)
}

// positionIn is an expression: where the Kanji's character first appears in
// text, or -1.
func positionIn(text string) bson.D {
	return bson.D{{Key: "$indexOfCP", Value: bson.A{literal(text), "$kanji"}}}
}

// literal is an expression: v as it is. Without it, MongoDB reads a string
// starting with "$" in an expression as a field path, so text the Owner typed
// could fail the query or match the wrong thing.
func literal(v any) bson.D {
	return bson.D{{Key: "$literal", Value: v}}
}

// toKanji reads every Kanji from cursor.
func toKanji(ctx context.Context, cursor *mongo.Cursor) ([]model.Kanji, error) {
	var docs []kanjiDocument
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}
	list := make([]model.Kanji, len(docs))
	for i, d := range docs {
		list[i] = d.toKanji()
	}
	return list, nil
}
