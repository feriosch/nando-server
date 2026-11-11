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

// NewWords returns the WordsRepository over the words collection in db,
// first creating any indexes it lacks.
func NewWords(ctx context.Context, db *mongo.Database) (model.WordsRepository, error) {
	collection := db.Collection("words")
	// One Word per spelling and reading. A kana Word has no hiragana field,
	// which the index takes as null, so kana Words are unique by spelling.
	_, err := collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "word", Value: 1}, {Key: "hiragana", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("spelling_reading"),
	})
	if err != nil {
		return nil, err
	}
	return words{collection: collection}, nil
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
	return r.search(ctx, q, []string{"word", "hiragana", "spanish"}, limit)
}

func (r words) GetManyBySearchIn(ctx context.Context, q, field string, limit int) ([]model.Word, error) {
	return r.search(ctx, q, []string{field}, limit)
}

// search returns up to limit Words in which any of fields contains q, ranked
// as GetManyBySearch says.
func (r words) search(ctx context.Context, q string, fields []string, limit int) ([]model.Word, error) {
	pattern := utils.SearchPattern(q)
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

func (r words) CountTags(ctx context.Context) (wordTags, nuanceTags []model.TagCount, err error) {
	if wordTags, err = r.countTags(ctx, mongo.Pipeline{
		{{Key: "$unwind", Value: "$tags"}},
		{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$tags"}, {Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}}}}},
	}); err != nil {
		return nil, nil, err
	}
	// Only Word Links: a Hint has no wordId, and no Nuance Tags in Nando.
	nuanceTags, err = r.countTags(ctx, mongo.Pipeline{
		{{Key: "$unwind", Value: "$related"}},
		{{Key: "$match", Value: bson.D{{Key: "related.wordId", Value: bson.D{{Key: "$type", Value: "objectId"}}}}}},
		{{Key: "$unwind", Value: "$related.tags"}},
		{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$related.tags"}, {Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}}}}},
	})
	return wordTags, nuanceTags, err
}

// countTags runs pipeline, which groups tags as {_id: tag, count}, and
// returns them most used first, then alphabetically.
func (r words) countTags(ctx context.Context, pipeline mongo.Pipeline) ([]model.TagCount, error) {
	pipeline = append(pipeline, bson.D{{Key: "$sort", Value: bson.D{{Key: "count", Value: -1}, {Key: "_id", Value: 1}}}})
	cursor, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	var docs []struct {
		Tag   string `bson:"_id"`
		Count int    `bson:"count"`
	}
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}
	counts := make([]model.TagCount, len(docs))
	for i, d := range docs {
		counts[i] = model.TagCount(d)
	}
	return counts, nil
}

func (r words) Insert(ctx context.Context, word model.Word) (string, error) {
	id := bson.NewObjectID()
	doc := bson.D{{Key: "_id", Value: id}, {Key: "word", Value: word.Word}}
	// Stored as benkyo stored them: no hiragana for a kana Word, no notes when
	// there are none.
	if word.Hiragana != "" {
		doc = append(doc, bson.E{Key: "hiragana", Value: word.Hiragana})
	}
	sentences := bson.A{}
	for _, s := range word.Sentences {
		sentences = append(sentences, bson.D{{Key: "sentence", Value: s.Sentence}, {Key: "translation", Value: s.Translation}})
	}
	doc = append(doc,
		bson.E{Key: "spanish", Value: word.Spanish},
		bson.E{Key: "type", Value: word.Type},
		bson.E{Key: "group", Value: word.Group},
		bson.E{Key: "tags", Value: word.Tags},
	)
	if word.Notes != "" {
		doc = append(doc, bson.E{Key: "notes", Value: word.Notes})
	}
	doc = append(doc, bson.E{Key: "sentences", Value: sentences}, bson.E{Key: "related", Value: bson.A{}})
	if _, err := r.collection.InsertOne(ctx, doc); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return "", model.ErrWordExists
		}
		return "", err
	}
	return id.Hex(), nil
}

func (r words) Update(ctx context.Context, id string, changes model.WordChanges) error {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return coreutils.ErrNotFound
	}
	set, unset := bson.D{}, bson.D{}
	// setOrUnset sets field to value, or removes it if value is "", as benkyo
	// stored a kana Word with no hiragana and a Word without notes.
	setOrUnset := func(field string, value *string) {
		switch {
		case value == nil:
		case *value == "":
			unset = append(unset, bson.E{Key: field, Value: ""})
		default:
			set = append(set, bson.E{Key: field, Value: *value})
		}
	}
	if changes.Word != nil {
		set = append(set, bson.E{Key: "word", Value: *changes.Word})
	}
	setOrUnset("hiragana", changes.Hiragana)
	if changes.Spanish != nil {
		set = append(set, bson.E{Key: "spanish", Value: *changes.Spanish})
	}
	if changes.Type != nil {
		set = append(set, bson.E{Key: "type", Value: changes.Type})
	}
	if changes.Tags != nil {
		set = append(set, bson.E{Key: "tags", Value: *changes.Tags})
	}
	setOrUnset("notes", changes.Notes)
	if changes.Sentences != nil {
		sentences := bson.A{}
		for _, s := range *changes.Sentences {
			sentences = append(sentences, bson.D{{Key: "sentence", Value: s.Sentence}, {Key: "translation", Value: s.Translation}})
		}
		set = append(set, bson.E{Key: "sentences", Value: sentences})
	}
	update := bson.D{}
	if len(set) > 0 {
		update = append(update, bson.E{Key: "$set", Value: set})
	}
	if len(unset) > 0 {
		update = append(update, bson.E{Key: "$unset", Value: unset})
	}
	res, err := r.collection.UpdateByID(ctx, oid, update)
	if mongo.IsDuplicateKeyError(err) {
		return model.ErrWordExists
	}
	return coreutils.Matched(res, err)
}

func (r words) AddWordLinkSide(ctx context.Context, id string, side model.WordLink) (bool, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return false, coreutils.ErrNotFound
	}
	other, err := bson.ObjectIDFromHex(side.WordID)
	if err != nil {
		return false, coreutils.ErrNotFound
	}
	filter := bson.D{{Key: "_id", Value: oid}, {Key: "related.wordId", Value: bson.D{{Key: "$ne", Value: other}}}}
	push := bson.D{{Key: "$push", Value: bson.D{{Key: "related", Value: bson.D{
		{Key: "wordId", Value: other},
		{Key: "type", Value: side.Kind},
		{Key: "note", Value: side.Note},
		{Key: "tags", Value: side.NuanceTags},
	}}}}}
	res, err := r.collection.UpdateOne(ctx, filter, push)
	if err != nil {
		return false, err
	}
	return res.ModifiedCount == 1, nil
}

func (r words) UpdateWordLinkSide(ctx context.Context, id, otherID string, changes model.WordLinkChanges) error {
	oid, other, err := objectIDs(id, otherID)
	if err != nil {
		return err
	}
	set := bson.D{}
	if changes.Kind != nil {
		set = append(set, bson.E{Key: "related.$[side].type", Value: *changes.Kind})
	}
	if changes.Note != nil {
		set = append(set, bson.E{Key: "related.$[side].note", Value: *changes.Note})
	}
	if changes.NuanceTags != nil {
		set = append(set, bson.E{Key: "related.$[side].tags", Value: *changes.NuanceTags})
	}
	res, err := r.collection.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: oid}, {Key: "related.wordId", Value: other}},
		bson.D{{Key: "$set", Value: set}},
		options.UpdateOne().SetArrayFilters([]any{bson.D{{Key: "side.wordId", Value: other}}}),
	)
	return coreutils.Matched(res, err)
}

func (r words) RemoveWordLinkSide(ctx context.Context, id, otherID string) (bool, error) {
	oid, other, err := objectIDs(id, otherID)
	if err != nil {
		return false, nil
	}
	res, err := r.collection.UpdateByID(ctx, oid, bson.D{{Key: "$pull", Value: bson.D{{Key: "related", Value: bson.D{{Key: "wordId", Value: other}}}}}})
	if err != nil {
		return false, err
	}
	return res.ModifiedCount == 1, nil
}

// hintWith matches a Hint with text in related: an entry with no wordId.
func hintWith(text string) bson.D {
	return bson.D{{Key: "word", Value: text}, {Key: "wordId", Value: bson.D{{Key: "$exists", Value: false}}}}
}

func (r words) AddHint(ctx context.Context, id string, hint model.Hint) (bool, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return false, coreutils.ErrNotFound
	}
	filter := bson.D{{Key: "_id", Value: oid}, {Key: "related", Value: bson.D{{Key: "$not", Value: bson.D{{Key: "$elemMatch", Value: hintWith(hint.Text)}}}}}}
	// Stored as benkyo stored Hints, which the Anki export reads.
	push := bson.D{{Key: "$push", Value: bson.D{{Key: "related", Value: bson.D{
		{Key: "word", Value: hint.Text},
		{Key: "type", Value: "related"},
		{Key: "note", Value: hint.Note},
		{Key: "tags", Value: bson.A{}},
	}}}}}
	res, err := r.collection.UpdateOne(ctx, filter, push)
	if err != nil {
		return false, err
	}
	return res.ModifiedCount == 1, nil
}

func (r words) RemoveHint(ctx context.Context, id, text string) (bool, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return false, nil
	}
	res, err := r.collection.UpdateByID(ctx, oid, bson.D{{Key: "$pull", Value: bson.D{{Key: "related", Value: hintWith(text)}}}})
	if err != nil {
		return false, err
	}
	return res.ModifiedCount == 1, nil
}

func (r words) SetGroup(ctx context.Context, id, group string) error {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return coreutils.ErrNotFound
	}
	res, err := r.collection.UpdateByID(ctx, oid, bson.D{{Key: "$set", Value: bson.D{{Key: "group", Value: group}}}})
	return coreutils.Matched(res, err)
}

// pullSideTo is an update removing a side of a Word Link to the Word with oid.
func pullSideTo(oid bson.ObjectID) bson.D {
	return bson.D{{Key: "$pull", Value: bson.D{{Key: "related", Value: bson.D{{Key: "wordId", Value: oid}}}}}}
}

func (r words) RemoveWordLinksTo(ctx context.Context, id string) error {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return coreutils.ErrNotFound
	}
	_, err = r.collection.UpdateMany(ctx, bson.D{{Key: "related.wordId", Value: oid}}, pullSideTo(oid))
	return err
}

func (r words) RepointWordLinks(ctx context.Context, fromID, toID string) error {
	from, to, err := objectIDs(fromID, toID)
	if err != nil {
		return err
	}
	// A Word with sides to both keeps its side to toID.
	both := bson.D{{Key: "_id", Value: bson.D{{Key: "$ne", Value: to}}}, {Key: "$and", Value: bson.A{
		bson.D{{Key: "related.wordId", Value: from}},
		bson.D{{Key: "related.wordId", Value: to}},
	}}}
	if _, err := r.collection.UpdateMany(ctx, both, pullSideTo(from)); err != nil {
		return err
	}
	_, err = r.collection.UpdateMany(ctx,
		bson.D{{Key: "_id", Value: bson.D{{Key: "$ne", Value: to}}}, {Key: "related.wordId", Value: from}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "related.$[side].wordId", Value: to}}}},
		options.UpdateMany().SetArrayFilters([]any{bson.D{{Key: "side.wordId", Value: from}}}),
	)
	if err != nil {
		return err
	}
	_, err = r.collection.UpdateByID(ctx, to, pullSideTo(from))
	return err
}

func (r words) Delete(ctx context.Context, id string) error {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return coreutils.ErrNotFound
	}
	return coreutils.Deleted(r.collection.DeleteOne(ctx, bson.D{{Key: "_id", Value: oid}}))
}

// objectIDs are the ObjectIds of two Words' ids, or Core's utils.ErrNotFound
// if either isn't one.
func objectIDs(id, otherID string) (bson.ObjectID, bson.ObjectID, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return oid, oid, coreutils.ErrNotFound
	}
	other, err := bson.ObjectIDFromHex(otherID)
	if err != nil {
		return oid, other, coreutils.ErrNotFound
	}
	return oid, other, nil
}

func (r words) GetManyBySpellingOrReading(ctx context.Context, text string) ([]model.Word, error) {
	filter := bson.D{{Key: "$or", Value: bson.A{bson.D{{Key: "word", Value: text}}, bson.D{{Key: "hiragana", Value: text}}}}}
	cursor, err := r.collection.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
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

// ankiWordDocument is a Word as the JLPT deck reads it: projected as benkyo's
// export projected it, with the Words it has Word Links to looked up.
type ankiWordDocument struct {
	ID       bson.ObjectID `bson:"_id"`
	Word     string        `bson:"word"`
	Hiragana string        `bson:"hiragana"`
	Spanish  string        `bson:"spanish"`
	// Type is decoded in order, and its values as stored, which aren't always
	// whole numbers.
	Type        bson.D   `bson:"type"`
	Tags        []string `bson:"tags"`
	Notes       string   `bson:"notes"`
	Sentence    string   `bson:"sentence"`
	Translation string   `bson:"translation"`
	// Related holds both the Word's sides of its Word Links and its Hints.
	Related []struct {
		WordID *bson.ObjectID `bson:"wordId"`
		Word   string         `bson:"word"`
		// Type is nil when none is stored.
		Type *string  `bson:"type"`
		Note string   `bson:"note"`
		Tags []string `bson:"tags"`
	} `bson:"related"`
	Linked []struct {
		ID bson.ObjectID `bson:"_id"`
		// Word and Hiragana are nil when not stored, which benkyo told apart
		// from stored empty.
		Word     *string  `bson:"word"`
		Hiragana *string  `bson:"hiragana"`
		Group    string   `bson:"group"`
		Tags     []string `bson:"tags"`
	} `bson:"_linked"`
}

func (d ankiWordDocument) toAnkiWord() model.AnkiWord {
	w := model.AnkiWord{
		ID:          d.ID.Hex(),
		Word:        d.Word,
		Hiragana:    d.Hiragana,
		Spanish:     d.Spanish,
		Tags:        d.Tags,
		Notes:       d.Notes,
		Sentence:    d.Sentence,
		Translation: d.Translation,
	}
	for _, e := range d.Type {
		w.PartsOfSpeech = append(w.PartsOfSpeech, model.PartOfSpeech{Key: e.Key, Form: formOf(e.Value)})
	}
	others := map[bson.ObjectID]*model.AnkiOtherWord{}
	for _, l := range d.Linked {
		// A Word with no kanji spelling is shown by its kana, and one with
		// neither stored by "?", as benkyo did.
		spelling := "?"
		if l.Word != nil && *l.Word != "" {
			spelling = *l.Word
		} else if l.Hiragana != nil {
			spelling = *l.Hiragana
		}
		others[l.ID] = &model.AnkiOtherWord{Spelling: spelling, Group: l.Group, Tags: l.Tags}
	}
	for _, rel := range d.Related {
		related := model.AnkiRelated{Text: rel.Word, Kind: "related", Note: rel.Note, NuanceTags: rel.Tags}
		if rel.Type != nil {
			related.Kind = *rel.Type
		}
		if rel.WordID != nil {
			related.Other = others[*rel.WordID]
		}
		w.Related = append(w.Related, related)
	}
	return w
}

// formOf is the Form a stored part of speech's value is, compared as benkyo
// compared it, with Python's ==: true is 1, and a number that isn't whole,
// such as 1.5, is no form.
func formOf(v any) model.Form {
	var n float64
	switch v := v.(type) {
	case int32:
		n = float64(v)
	case int64:
		n = float64(v)
	case float64:
		n = v
	case bool:
		if v {
			n = 1
		}
	}
	switch form := model.Form(n); form {
	case model.PlainForm, model.ToForm, model.TekiForm:
		if float64(form) == n {
			return form
		}
	}
	return model.NoForm
}

func (r words) GetManyForJLPTDeck(ctx context.Context) ([]model.AnkiWord, error) {
	cursor, err := r.collection.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: bson.D{{Key: "$or", Value: bson.A{
			bson.D{{Key: "group", Value: bson.D{{Key: "$in", Value: model.JLPTGroups()}}}},
			bson.D{{Key: "tags", Value: "jlpt_n1"}},
		}}}}},
		{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
		{{Key: "$lookup", Value: bson.D{
			{Key: "from", Value: "words"},
			{Key: "localField", Value: "related.wordId"},
			{Key: "foreignField", Value: "_id"},
			{Key: "as", Value: "_linked"},
			{Key: "pipeline", Value: mongo.Pipeline{{{Key: "$project", Value: bson.D{
				{Key: "word", Value: 1}, {Key: "hiragana", Value: 1}, {Key: "group", Value: 1}, {Key: "tags", Value: 1},
			}}}}},
		}}},
		{{Key: "$project", Value: bson.D{
			{Key: "word", Value: 1},
			{Key: "hiragana", Value: 1},
			{Key: "spanish", Value: 1},
			{Key: "type", Value: 1},
			{Key: "tags", Value: 1},
			{Key: "notes", Value: 1},
			// The first stored sentence and the first stored translation, each
			// skipping example sentences without one, as benkyo took them.
			{Key: "sentence", Value: bson.D{{Key: "$first", Value: "$sentences.sentence"}}},
			{Key: "translation", Value: bson.D{{Key: "$first", Value: "$sentences.translation"}}},
			{Key: "related", Value: 1},
			{Key: "_linked", Value: 1},
		}}},
	})
	if err != nil {
		return nil, err
	}
	var docs []ankiWordDocument
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}
	list := make([]model.AnkiWord, len(docs))
	for i, d := range docs {
		list[i] = d.toAnkiWord()
	}
	return list, nil
}
