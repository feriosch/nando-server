package repository

import (
	"context"
	"regexp"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	coreutils "github.com/feriosch/nando-server/internal/core/utils"
	"github.com/feriosch/nando-server/internal/japanese/model"
	"github.com/feriosch/nando-server/internal/japanese/utils"
)

// NewWords returns the WordsRepository over the words collection in db.
func NewWords(db *mongo.Database) model.WordsRepository {
	return words{collection: db.Collection("words")}
}

// words is the words collection.
type words struct {
	collection *mongo.Collection
}

// wordDocument is a Word as it is stored.
type wordDocument struct {
	ID       bson.ObjectID `bson:"_id"`
	Word     string        `bson:"word"`
	Hiragana string        `bson:"hiragana"`
	Spanish  string        `bson:"spanish"`
	Group    string        `bson:"group"`
	// Type is a document, decoded in order: the order of a Word's parts of
	// speech is the order Anki lists them in.
	Type      bson.D   `bson:"type"`
	Tags      []string `bson:"tags"`
	Notes     string   `bson:"notes"`
	Sentences []struct {
		Sentence    string `bson:"sentence"`
		Translation string `bson:"translation"`
	} `bson:"sentences"`
	// Related holds both the Word's sides of its Word Links, which name the
	// other Word by wordId, and its Hints, which have no wordId.
	Related []struct {
		WordID *bson.ObjectID `bson:"wordId"`
		Word   string         `bson:"word"`
		Type   string         `bson:"type"`
		Note   string         `bson:"note"`
		Tags   []string       `bson:"tags"`
	} `bson:"related"`
}

func (d wordDocument) toWord() model.Word {
	w := model.Word{
		ID:       d.ID.Hex(),
		Word:     d.Word,
		Hiragana: d.Hiragana,
		Spanish:  d.Spanish,
		Group:    d.Group,
		Tags:     d.Tags,
		Notes:    d.Notes,
	}
	for _, e := range d.Type {
		w.Type = append(w.Type, model.PartOfSpeech{Key: e.Key, Form: toInt(e.Value)})
	}
	for _, s := range d.Sentences {
		w.Sentences = append(w.Sentences, model.Sentence(s))
	}
	for i, rel := range d.Related {
		if rel.WordID == nil {
			w.Hints = append(w.Hints, model.Hint{Text: rel.Word, Note: rel.Note, Kind: rel.Type, NuanceTags: rel.Tags, Position: i})
			continue
		}
		w.WordLinks = append(w.WordLinks, model.WordLink{WordID: rel.WordID.Hex(), Kind: rel.Type, Note: rel.Note, NuanceTags: rel.Tags, Position: i})
	}
	return w
}

// toInt is v as an int, whichever number type it was stored as, or 0 if it
// isn't a number.
func toInt(v any) int {
	switch n := v.(type) {
	case int32:
		return int(n)
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}

func (r words) GetManyBySearch(ctx context.Context, q string, limit int) ([]model.Word, error) {
	pattern := utils.SearchPattern(q)
	fields := []string{"word", "hiragana", "spanish"}
	cursor, err := r.collection.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: utils.MatchesAny(pattern, fields...)}},
		{{Key: "$addFields", Value: bson.D{
			{Key: "_exact", Value: utils.IsExact(pattern, fields...)},
			{Key: "_length", Value: utils.MatchedLength(pattern, fields...)},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "_exact", Value: -1}, {Key: "_length", Value: 1}, {Key: "word", Value: 1}, {Key: "_id", Value: 1}}}},
		{{Key: "$limit", Value: limit}},
		{{Key: "$project", Value: rowFields}},
	})
	if err != nil {
		return nil, err
	}
	return toWords(ctx, cursor)
}

// toWords reads every Word from cursor.
func toWords(ctx context.Context, cursor *mongo.Cursor) ([]model.Word, error) {
	var docs []wordDocument
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}
	list := make([]model.Word, len(docs))
	for i, d := range docs {
		list[i] = d.toWord()
	}
	return list, nil
}

// rowFields projects what a list shows of a Word.
var rowFields = bson.D{{Key: "word", Value: 1}, {Key: "hiragana", Value: 1}, {Key: "spanish", Value: 1}, {Key: "group", Value: 1}}

func (r words) GetOneByID(ctx context.Context, id string) (model.Word, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return model.Word{}, coreutils.ErrNotFound
	}
	var d wordDocument
	if err := r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: oid}}).Decode(&d); err != nil {
		return model.Word{}, coreutils.NotFound(err)
	}
	return d.toWord(), nil
}

func (r words) GetManyByIDs(ctx context.Context, ids []string) ([]model.Word, error) {
	oids := bson.A{}
	for _, id := range ids {
		if oid, err := bson.ObjectIDFromHex(id); err == nil {
			oids = append(oids, oid)
		}
	}
	projection := append(bson.D{{Key: "tags", Value: 1}}, rowFields...)
	cursor, err := r.collection.Find(ctx, bson.D{{Key: "_id", Value: bson.D{{Key: "$in", Value: oids}}}}, options.Find().SetProjection(projection))
	if err != nil {
		return nil, err
	}
	return toWords(ctx, cursor)
}

func (r words) GetManyByKanji(ctx context.Context, kanji string, limit int) ([]model.Word, error) {
	cursor, err := r.collection.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: bson.D{{Key: "word", Value: bson.D{{Key: "$regex", Value: regexp.QuoteMeta(kanji)}}}}}},
		{{Key: "$addFields", Value: bson.D{{Key: "_length", Value: bson.D{{Key: "$strLenCP", Value: "$word"}}}}}},
		{{Key: "$sort", Value: bson.D{{Key: "_length", Value: 1}, {Key: "word", Value: 1}, {Key: "_id", Value: 1}}}},
		{{Key: "$limit", Value: limit}},
		{{Key: "$project", Value: rowFields}},
	})
	if err != nil {
		return nil, err
	}
	return toWords(ctx, cursor)
}

func (r words) GetManyForJLPTDeck(ctx context.Context) ([]model.Word, error) {
	filter := bson.D{{Key: "$or", Value: bson.A{
		bson.D{{Key: "group", Value: bson.D{{Key: "$in", Value: bson.A{"jlpt_n3", "jlpt_n2", "jlpt_n1"}}}}},
		bson.D{{Key: "tags", Value: "jlpt_n1"}},
	}}}
	cursor, err := r.collection.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	return toWords(ctx, cursor)
}
