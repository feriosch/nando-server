package tests

import (
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/feriosch/nando-server/internal/japanese/model"
)

// group is a Group as benkyo stored it.
func group(key, name, imageURL string) bson.M {
	return bson.M{
		"_id":             bson.NewObjectID(),
		"collection_name": key,
		"printing_name":   name,
		"group":           "Academic",
		"image_url":       imageURL,
	}
}

const n2Picture = "https://storage.googleapis.com/benkyo/jlpt_n2.png"

func TestAWordPageShowsEverythingAboutTheWord(t *testing.T) {
	n, phone := signedInBrowser(t)
	n.store("groups", group("jlpt_n2", "JLPT N2", n2Picture), group("basic", "Basic", "https://storage.googleapis.com/benkyo/basic.png"))
	footprint := word("足跡", "あしあと", "huella, pisada", "jlpt_n2")
	footprint["type"] = bson.M{"noun": 1, "suru": 4}
	footprint["tags"] = bson.A{"common", "usually_kana"}
	footprint["notes"] = "También figurado: el rastro que deja alguien."
	footprint["sentences"] = bson.A{
		bson.M{"sentence": "雪に足跡が残っていた。", "translation": "Quedaban huellas en la nieve."},
	}
	n.store("words", footprint)

	got := get[model.WordJSON](phone, "/api/japanese/words/"+id(footprint))

	want := model.WordJSON{
		ID:        id(footprint),
		Word:      "足跡",
		Hiragana:  ptr("あしあと"),
		Spanish:   "huella, pisada",
		Type:      map[string]int{"noun": 1, "suru": 4},
		Group:     model.GroupJSON{Key: "jlpt_n2", Name: "JLPT N2", ImageURL: ptr(n2Picture)},
		Tags:      []string{"common", "usually_kana"},
		Notes:     ptr("También figurado: el rastro que deja alguien."),
		Sentences: []model.SentenceJSON{{Sentence: "雪に足跡が残っていた。", Translation: "Quedaban huellas en la nieve."}},
		WordLinks: []model.WordLinkJSON{},
		Hints:     []model.HintJSON{},
		Kanji:     []model.KanjiRowJSON{},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Word = %+v,\nwant %+v", got, want)
	}
}

func TestAWordWithoutReadingTagsOrNotes(t *testing.T) {
	n, phone := signedInBrowser(t)
	n.store("groups", group("miscellaneous", "Miscellaneous", "https://storage.googleapis.com/benkyo/misc.png"))
	sorry := word("あしからず", "", "no lo tomes a mal", "miscellaneous")
	n.store("words", sorry)

	got := get[model.WordJSON](phone, "/api/japanese/words/"+id(sorry))

	if got.Hiragana != nil || got.Notes != nil || got.Tags == nil || len(got.Tags) != 0 || got.Sentences == nil {
		t.Errorf("Word = %+v, want hiragana and notes null, tags and sentences []", got)
	}
}

func TestAWordPageShowsItsWordLinksAndHints(t *testing.T) {
	n, phone := signedInBrowser(t)
	n.store("groups", group("jlpt_n2", "JLPT N2", n2Picture))
	longFor := word("待ち望む", "まちのぞむ", "esperar con ansia", "jlpt_n1")
	hope := word("希望", "きぼう", "esperanza", "basic")
	despair := word("絶望", "ぜつぼう", "desesperación", "jlpt_n2")
	expect := word("期待", "きたい", "expectativa", "jlpt_n2")
	expect["related"] = bson.A{
		bson.M{"wordId": longFor["_id"], "type": "synonym", "note": "Más intenso y escrito.", "tags": bson.A{"written", "literary"}},
		bson.M{"wordId": hope["_id"], "type": "related", "note": ""},
		bson.M{"wordId": despair["_id"], "type": "antonym", "note": "", "tags": bson.A{}},
		// A Word Link to a Word that is no longer stored can't be opened.
		bson.M{"wordId": bson.NewObjectID(), "type": "related", "note": "gone"},
		bson.M{"word": "天気", "type": "related", "note": "Se espera buen tiempo.", "tags": bson.A{}},
		bson.M{"word": "楽しみ", "type": "related", "note": ""},
	}
	n.store("words", expect, longFor, hope, despair)

	got := get[model.WordJSON](phone, "/api/japanese/words/"+id(expect))

	wantLinks := []model.WordLinkJSON{
		{Kind: "synonym", Note: ptr("Más intenso y escrito."), NuanceTags: []string{"written", "literary"},
			Word: model.WordRowJSON{ID: id(longFor), Word: "待ち望む", Hiragana: ptr("まちのぞむ"), Spanish: "esperar con ansia", Group: "jlpt_n1"}},
		{Kind: "related", NuanceTags: []string{},
			Word: model.WordRowJSON{ID: id(hope), Word: "希望", Hiragana: ptr("きぼう"), Spanish: "esperanza", Group: "basic"}},
		{Kind: "antonym", NuanceTags: []string{},
			Word: model.WordRowJSON{ID: id(despair), Word: "絶望", Hiragana: ptr("ぜつぼう"), Spanish: "desesperación", Group: "jlpt_n2"}},
	}
	if !reflect.DeepEqual(got.WordLinks, wantLinks) {
		t.Errorf("Word Links = %+v,\nwant %+v", got.WordLinks, wantLinks)
	}
	wantHints := []model.HintJSON{{Text: "天気", Note: ptr("Se espera buen tiempo.")}, {Text: "楽しみ"}}
	if !reflect.DeepEqual(got.Hints, wantHints) {
		t.Errorf("Hints = %+v, want %+v", got.Hints, wantHints)
	}
}

func TestAWordPageShowsTheStoredKanjiInItsSpelling(t *testing.T) {
	n, phone := signedInBrowser(t)
	n.store("groups", group("basic", "Basic", "https://storage.googleapis.com/benkyo/basic.png"))
	sunday := word("日曜日", "にちようび", "domingo", "basic")
	walk := word("歩き回る", "あるきまわる", "andar de un lado a otro", "basic")
	n.store("words", sunday, walk)
	day := kanji("日", "día", 12)
	weekday := kanji("曜", "día de la semana", 1825)
	turn := kanji("回", "vez", 600)
	n.store("kanji", turn, weekday, day, kanji("月", "mes", 13))

	got := get[model.WordJSON](phone, "/api/japanese/words/"+id(sunday))
	want := []model.KanjiRowJSON{
		{ID: id(day), Kanji: "日", Spanish: "día"},
		{ID: id(weekday), Kanji: "曜", Spanish: "día de la semana"},
	}
	if !reflect.DeepEqual(got.Kanji, want) {
		t.Errorf("Kanji in 日曜日 = %+v, want %+v", got.Kanji, want)
	}

	// 歩 isn't stored, and neither kana is a Kanji.
	got = get[model.WordJSON](phone, "/api/japanese/words/"+id(walk))
	if want := []string{"回"}; !reflect.DeepEqual(characters(got.Kanji), want) {
		t.Errorf("Kanji in 歩き回る = %v, want %v", characters(got.Kanji), want)
	}
}
