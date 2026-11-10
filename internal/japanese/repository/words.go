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
	ID        bson.ObjectID  `bson:"_id"`
	Word      string         `bson:"word"`
	Hiragana  string         `bson:"hiragana"`
	Spanish   string         `bson:"spanish"`
	Group     string         `bson:"group"`
	Type      map[string]int `bson:"type"`
	Tags      []string       `bson:"tags"`
	Notes     string         `bson:"notes"`
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
		Type:     d.Type,
		Tags:     d.Tags,
		Notes:    d.Notes,
	}
	for _, s := range d.Sentences {
		w.Sentences = append(w.Sentences, model.Sentence(s))
	}
	for _, rel := range d.Related {
		if rel.WordID == nil {
			w.Hints = append(w.Hints, model.Hint{Text: rel.Word, Note: rel.Note})
			continue
		}
		w.WordLinks = append(w.WordLinks, model.WordLink{WordID: rel.WordID.Hex(), Kind: rel.Type, Note: rel.Note, NuanceTags: rel.Tags})
	}
	return w
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
	cursor, err := r.collection.Find(ctx, bson.D{{Key: "_id", Value: bson.D{{Key: "$in", Value: oids}}}}, options.Find().SetProjection(rowFields))
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
