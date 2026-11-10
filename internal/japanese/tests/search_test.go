package tests

import (
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// wordRowJSON is a Word as a search result row shows it.
type wordRowJSON struct {
	ID       string  `json:"id"`
	Word     string  `json:"word"`
	Hiragana *string `json:"hiragana"`
	Spanish  string  `json:"spanish"`
	Group    string  `json:"group"`
}

// kanjiRowJSON is a Kanji as a search result row shows it.
type kanjiRowJSON struct {
	ID      string `json:"id"`
	Kanji   string `json:"kanji"`
	Spanish string `json:"spanish"`
}

type searchJSON struct {
	Words []wordRowJSON  `json:"words"`
	Kanji []kanjiRowJSON `json:"kanji"`
}

func search(d *browser, q string) searchJSON {
	d.nando.t.Helper()
	return get[searchJSON](d, "/api/japanese/search?q="+url.QueryEscape(q))
}

// word is a Word as benkyo stored it, under a new ObjectId.
func word(spelling, hiragana, spanish, group string) bson.M {
	doc := bson.M{
		"_id":       bson.NewObjectID(),
		"word":      spelling,
		"spanish":   spanish,
		"group":     group,
		"sentences": bson.A{},
		"type":      bson.M{"noun": 1},
	}
	if hiragana != "" {
		doc["hiragana"] = hiragana
	}
	return doc
}

// kanji is a Kanji as benkyo stored it, under a new ObjectId.
func kanji(character, spanish string, v1 int) bson.M {
	return bson.M{
		"_id":      bson.NewObjectID(),
		"v1":       v1,
		"kanji":    character,
		"spanish":  spanish,
		"radicals": bson.A{spanish},
	}
}

func id(doc bson.M) string { return doc["_id"].(bson.ObjectID).Hex() }

// spellings is the spelling of each Word, in order.
func spellings(rows []wordRowJSON) []string {
	list := []string{}
	for _, r := range rows {
		list = append(list, r.Word)
	}
	return list
}

// characters is the character of each Kanji, in order.
func characters(rows []kanjiRowJSON) []string {
	list := []string{}
	for _, r := range rows {
		list = append(list, r.Kanji)
	}
	return list
}

func TestSearchByKanjiFindsWordsSpelledWithIt(t *testing.T) {
	n, phone := signedInBrowser(t)
	foot := word("足", "あし", "pie, pierna", "basic")
	footprint := word("足跡", "あしあと", "huella", "jlpt_n2")
	n.store("words", footprint, foot, word("天気", "てんき", "tiempo", "basic"))

	got := search(phone, "足")

	want := []wordRowJSON{
		{ID: id(foot), Word: "足", Hiragana: ptr("あし"), Spanish: "pie, pierna", Group: "basic"},
		{ID: id(footprint), Word: "足跡", Hiragana: ptr("あしあと"), Spanish: "huella", Group: "jlpt_n2"},
	}
	if !reflect.DeepEqual(got.Words, want) {
		t.Errorf("Words for 足 = %+v, want %+v", got.Words, want)
	}
}

func ptr(s string) *string { return &s }

func TestSearchByWordFindsTheKanjiInIt(t *testing.T) {
	n, phone := signedInBrowser(t)
	trace := kanji("跡", "huellas", 1640)
	foot := kanji("足", "pie", 1308)
	n.store("kanji", trace, foot, kanji("天", "cielo", 425))

	got := search(phone, "足跡")

	want := []kanjiRowJSON{
		{ID: id(foot), Kanji: "足", Spanish: "pie"},
		{ID: id(trace), Kanji: "跡", Spanish: "huellas"},
	}
	if !reflect.DeepEqual(got.Kanji, want) {
		t.Errorf("Kanji for 足跡 = %+v, want %+v", got.Kanji, want)
	}
}

func TestSearchByKanaFindsWordsReadOrSpelledWithIt(t *testing.T) {
	n, phone := signedInBrowser(t)
	n.store("words",
		word("足跡", "あしあと", "huella", "jlpt_n2"),
		word("あしからず", "", "no lo tomes a mal", "miscellaneous"),
		word("明日", "あした", "mañana", "basic"),
		word("足", "あし", "pie, pierna", "basic"),
		word("朝", "あさ", "mañana (temprano)", "basic"),
	)

	got := search(phone, "あし")

	if want := []string{"足", "明日", "足跡", "あしからず"}; !reflect.DeepEqual(spellings(got.Words), want) {
		t.Errorf("Words for あし = %v, want %v", spellings(got.Words), want)
	}
}

func TestSearchBySpanishIgnoresCaseAndAccents(t *testing.T) {
	n, phone := signedInBrowser(t)
	n.store("words",
		word("胸騒ぎ", "むなさわぎ", "Corazonada, mal presentimiento", "literature"),
		word("心", "こころ", "corazón", "basic"),
		word("頭", "あたま", "cabeza", "basic"),
	)
	n.store("kanji", kanji("心", "corazón", 645), kanji("頭", "cabeza", 1503))

	for _, q := range []string{"corazon", "corazón", "CORAZÓN", "Corazon"} {
		got := search(phone, q)
		if want := []string{"心", "胸騒ぎ"}; !reflect.DeepEqual(spellings(got.Words), want) {
			t.Errorf("Words for %s = %v, want %v", q, spellings(got.Words), want)
		}
		if want := []string{"心"}; !reflect.DeepEqual(characters(got.Kanji), want) {
			t.Errorf("Kanji for %s = %v, want %v", q, characters(got.Kanji), want)
		}
	}
}

func TestAnEmptySearchFindsNothing(t *testing.T) {
	n, phone := signedInBrowser(t)
	n.store("words", word("足", "あし", "pie", "basic"))
	n.store("kanji", kanji("足", "pie", 1308))

	for _, q := range []string{"", "  "} {
		if got := search(phone, q); len(got.Words) != 0 || len(got.Kanji) != 0 {
			t.Errorf("search %q = %+v, want no Words and no Kanji", q, got)
		}
	}
}

func TestSearchListsExactMatchesThenShorterThenLonger(t *testing.T) {
	n, phone := signedInBrowser(t)
	n.store("words",
		word("足跡", "あしあと", "huella", "jlpt_n2"),
		word("石", "いし", "piedra", "basic"),
		word("脚", "あし", "pierna, pata", "jlpt_n2"),
		word("足", "あし", "pie", "basic"),
	)
	n.store("kanji",
		kanji("脚", "pierna", 1309),
		kanji("石", "piedra", 113),
		kanji("足", "pie", 1308),
		kanji("促", "estimular", 1311),
	)

	got := search(phone, "pie")

	if want := []string{"足", "石", "脚"}; !reflect.DeepEqual(spellings(got.Words), want) {
		t.Errorf("Words for pie = %v, want %v", spellings(got.Words), want)
	}
	// piedra and pierna are as long: then by Heisig number.
	if want := []string{"足", "石", "脚"}; !reflect.DeepEqual(characters(got.Kanji), want) {
		t.Errorf("Kanji for pie = %v, want %v", characters(got.Kanji), want)
	}
}

func TestSearchFindsAtMost50WordsAnd20Kanji(t *testing.T) {
	n, phone := signedInBrowser(t)
	var words, kanjiDocs []any
	for i := range 60 {
		words = append(words, word(fmt.Sprintf("日%02d", i), "", fmt.Sprintf("día %d", i), "basic"))
	}
	for i := range 30 {
		kanjiDocs = append(kanjiDocs, kanji(string(rune('一'+i)), fmt.Sprintf("día %d", i), i+1))
	}
	n.store("words", words...)
	n.store("kanji", kanjiDocs...)

	got := search(phone, "dia")

	if len(got.Words) != 50 || len(got.Kanji) != 20 {
		t.Errorf("search dia found %d Words and %d Kanji, want 50 and 20", len(got.Words), len(got.Kanji))
	}
}

func TestSearchFoldsDecomposedAccentsAndKana(t *testing.T) {
	n, phone := signedInBrowser(t)
	n.store("words", word("心", "こころ", "corazón", "basic"), word("学校", "がっこう", "escuela", "basic"))

	// corazón and がっこう as some keyboards send them: a base letter, then a combining mark.
	if got := search(phone, "corazón"); !reflect.DeepEqual(spellings(got.Words), []string{"心"}) {
		t.Errorf("Words for decomposed corazón = %v, want [心]", spellings(got.Words))
	}
	if got := search(phone, "がっこう"); !reflect.DeepEqual(spellings(got.Words), []string{"学校"}) {
		t.Errorf("Words for decomposed がっこう = %v, want [学校]", spellings(got.Words))
	}
}

func TestATooLongSearchIsRejected(t *testing.T) {
	_, phone := signedInBrowser(t)

	phone.must(http.MethodGet, "/api/japanese/search?q="+strings.Repeat("a", 101), nil, http.StatusBadRequest)
	phone.must(http.MethodGet, "/api/japanese/search?q="+url.QueryEscape(strings.Repeat("足", 100)), nil, http.StatusOK)
}
