package tests

import (
	"fmt"
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// kanjiJSON is a Kanji as GET /api/japanese/kanji/{id} answers it.
type kanjiJSON struct {
	ID         string          `json:"id"`
	Kanji      string          `json:"kanji"`
	Spanish    string          `json:"spanish"`
	On         *string         `json:"on"`
	Kun        *string         `json:"kun"`
	V1         int             `json:"v1"`
	V2         *int            `json:"v2"`
	Story      *string         `json:"story"`
	Components []componentJSON `json:"components"`
	Words      []wordRowJSON   `json:"words"`
}

// componentJSON is a Component of a Kanji: its keyword, and the Kanji it is,
// or null for an irregular shape.
type componentJSON struct {
	Name  string        `json:"name"`
	Kanji *kanjiRowJSON `json:"kanji"`
}

// irregularComponent is an irregular shape as benkyo stored it.
func irregularComponent(name string) bson.M {
	return bson.M{"_id": bson.NewObjectID(), "component": name, "radicals": bson.A{name}}
}

func TestAKanjiPageShowsTheKanjiItsComponentsAndWords(t *testing.T) {
	n, phone := signedInBrowser(t)
	every := kanji("毎", "cada", 486)
	sea := kanji("海", "mar", 491)
	sea["on"] = "カイ"
	sea["kun"] = "うみ"
	sea["v2"] = 1520
	sea["story"] = "Las gotas de agua de cada río acaban en el mar."
	sea["components"] = bson.A{"gotas de agua", "cada"}
	n.store("kanji", every, sea, kanji("水", "agua", 137))
	n.store("irregular_components", irregularComponent("gotas de agua"))
	overseas := word("海外", "かいがい", "extranjero", "basic")
	seaWord := word("海", "うみ", "mar", "basic")
	japanSea := word("日本海", "にほんかい", "mar de Japón", "miscellaneous")
	n.store("words", japanSea, overseas, seaWord, word("山", "やま", "montaña", "basic"))

	got := get[kanjiJSON](phone, "/api/japanese/kanji/"+id(sea))

	v2 := 1520
	want := kanjiJSON{
		ID:      id(sea),
		Kanji:   "海",
		Spanish: "mar",
		On:      ptr("カイ"),
		Kun:     ptr("うみ"),
		V1:      491,
		V2:      &v2,
		Story:   ptr("Las gotas de agua de cada río acaban en el mar."),
		Components: []componentJSON{
			{Name: "gotas de agua"},
			{Name: "cada", Kanji: &kanjiRowJSON{ID: id(every), Kanji: "毎", Spanish: "cada"}},
		},
		Words: []wordRowJSON{
			{ID: id(seaWord), Word: "海", Hiragana: ptr("うみ"), Spanish: "mar", Group: "basic"},
			{ID: id(overseas), Word: "海外", Hiragana: ptr("かいがい"), Spanish: "extranjero", Group: "basic"},
			{ID: id(japanSea), Word: "日本海", Hiragana: ptr("にほんかい"), Spanish: "mar de Japón", Group: "miscellaneous"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Kanji = %+v,\nwant %+v", got, want)
	}
}

func TestAKanjiWithoutReadingsOrComponents(t *testing.T) {
	n, phone := signedInBrowser(t)
	one := kanji("一", "uno", 1)
	n.store("kanji", one)

	got := get[kanjiJSON](phone, "/api/japanese/kanji/"+id(one))

	if got.On != nil || got.Kun != nil || got.V2 != nil || got.Story != nil || got.Components == nil || len(got.Components) != 0 || got.Words == nil {
		t.Errorf("Kanji = %+v, want on, kun, v2 and story null, components and words []", got)
	}
}

// A Component that is the keyword of an irregular shape is that shape, even
// when a Kanji has the same keyword, as benkyo worked out a Kanji's radicals.
func TestAnIrregularComponentWinsOverAKanjiOfTheSameKeyword(t *testing.T) {
	n, phone := signedInBrowser(t)
	swim := kanji("泳", "nadar", 140)
	swim["components"] = bson.A{"agua", "eterno"}
	eternal := kanji("永", "eterno", 139)
	n.store("kanji", swim, eternal, kanji("水", "agua", 137))
	n.store("irregular_components", irregularComponent("agua"))

	got := get[kanjiJSON](phone, "/api/japanese/kanji/"+id(swim))

	want := []componentJSON{
		{Name: "agua"},
		{Name: "eterno", Kanji: &kanjiRowJSON{ID: id(eternal), Kanji: "永", Spanish: "eterno"}},
	}
	if !reflect.DeepEqual(got.Components, want) {
		t.Errorf("Components = %+v, want %+v", got.Components, want)
	}
}

func TestAKanjiPageShowsAtMost50Words(t *testing.T) {
	n, phone := signedInBrowser(t)
	day := kanji("日", "día", 12)
	n.store("kanji", day)
	var words []any
	for i := range 60 {
		words = append(words, word(fmt.Sprintf("日%02d", i), "", fmt.Sprintf("día %d", i), "basic"))
	}
	n.store("words", words...)

	if got := get[kanjiJSON](phone, "/api/japanese/kanji/"+id(day)); len(got.Words) != 50 {
		t.Errorf("Words with 日: %d, want 50", len(got.Words))
	}
}
