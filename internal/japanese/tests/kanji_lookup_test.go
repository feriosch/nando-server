package tests

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"github.com/feriosch/nando-server/internal/japanese"
)

// kanjiLookupJSON is a Kanji Lookup, as GET /api/japanese/lookup/kanji/{kanji} answers it.
type kanjiLookupJSON struct {
	Kanji    string           `json:"kanji"`
	Meanings []string         `json:"meanings"`
	On       []string         `json:"on"`
	Kun      []string         `json:"kun"`
	Strokes  int              `json:"strokes"`
	Grade    *int             `json:"grade"`
	Joyo     bool             `json:"joyo"`
	JLPT     *int             `json:"jlpt"`
	Words    []lookupWordJSON `json:"words"`
}

// lookupWordJSON is a common word with the kanji, from Jisho.
type lookupWordJSON struct {
	Word     string   `json:"word"`
	Reading  string   `json:"reading"`
	Meanings []string `json:"meanings"`
}

// shi is kanjiapi.dev's answer for 士, as its API gives it.
const shi = `{"kanji":"士","grade":4,"stroke_count":3,"meanings":["gentleman","scholar","samurai"],
  "kun_readings":["さむらい"],"on_readings":["シ"],"name_readings":["お"],"jlpt":1,"unicode":"58eb",
  "heisig_en":"soldier","freq_mainichi_shinbun":526,"notes":[]}`

// shiWords is Jisho's search for 士, trimmed: common words with it, one that isn't common, and a
// common one found by its English, without 士.
const shiWords = `{"meta":{"status":200},"data":[
  {"slug":"武士","is_common":true,"japanese":[{"word":"武士","reading":"ぶし"},{"word":"武士","reading":"もののふ"}],
   "senses":[{"english_definitions":["warrior","samurai"]},{"english_definitions":["knight"]}]},
  {"slug":"侍","is_common":true,"japanese":[{"word":"侍","reading":"さむらい"}],
   "senses":[{"english_definitions":["samurai"]}]},
  {"slug":"士君子","is_common":false,"japanese":[{"word":"士君子","reading":"しくんし"}],
   "senses":[{"english_definitions":["man of learning and virtue"]}]},
  {"slug":"弁護士","is_common":true,"japanese":[{"word":"弁護士","reading":"べんごし"}],
   "senses":[{"english_definitions":["lawyer","attorney"]}]}
]}`

func TestAKanjiLookupTellsWhatAnUnstoredKanjiIs(t *testing.T) {
	_, kanjiAPI := newFakeKanjiAPI(t, map[string][]kanjiReply{"士": {{200, shi}}})
	_, jisho := newFakeJisho(t, 200, shiWords)
	n, phone := signedInBrowserWith(t, japanese.Config{KanjiAPIURL: kanjiAPI, JishoURL: jisho, KanjiAPIPace: pace})
	n.store("kanji", kanji("水", "agua", 137))

	got := get[kanjiLookupJSON](phone, "/api/japanese/lookup/kanji/士")

	want := kanjiLookupJSON{
		Kanji:    "士",
		Meanings: []string{"gentleman", "scholar", "samurai"},
		On:       []string{"シ"},
		Kun:      []string{"さむらい"},
		Strokes:  3,
		Grade:    iptr(4),
		Joyo:     true,
		JLPT:     iptr(1),
		Words: []lookupWordJSON{
			{Word: "武士", Reading: "ぶし", Meanings: []string{"warrior", "samurai"}},
			{Word: "弁護士", Reading: "べんごし", Meanings: []string{"lawyer", "attorney"}},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Kanji Lookup = %+v,\nwant %+v", got, want)
	}
}

// idJSON is the 409 a Kanji Lookup answers for a stored Kanji: which one it is.
type idJSON struct {
	ID string `json:"id"`
}

func TestAStoredKanjiIsNeverLookedUpOutside(t *testing.T) {
	kanjiAPI, kanjiAPIURL := newFakeKanjiAPI(t, map[string][]kanjiReply{"水": {{200, shi}}})
	jisho, jishoURL := newFakeJisho(t, 200, shiWords)
	n, phone := signedInBrowserWith(t, japanese.Config{KanjiAPIURL: kanjiAPIURL, JishoURL: jishoURL, KanjiAPIPace: pace})
	water := kanji("水", "agua", 137)
	n.store("kanji", water)

	rec := phone.must(http.MethodGet, "/api/japanese/lookup/kanji/水", nil, http.StatusConflict)

	var got idJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.ID != id(water) {
		t.Errorf("409 body = %s, want the stored Kanji's id %s", rec.Body.String(), id(water))
	}
	kanjiAPI.mu.Lock()
	defer kanjiAPI.mu.Unlock()
	jisho.mu.Lock()
	defer jisho.mu.Unlock()
	if len(kanjiAPI.asked) != 0 || len(jisho.keywords) != 0 {
		t.Errorf("asked kanjiapi.dev %d times and Jisho %d times, want neither", len(kanjiAPI.asked), len(jisho.keywords))
	}
}

func TestOnlyOneKanjiCanBeLookedUp(t *testing.T) {
	kanjiAPI, kanjiAPIURL := newFakeKanjiAPI(t, nil)
	jisho, jishoURL := newFakeJisho(t, 200, shiWords)
	_, phone := signedInBrowserWith(t, japanese.Config{KanjiAPIURL: kanjiAPIURL, JishoURL: jishoURL, KanjiAPIPace: pace})

	for _, notOneKanji := range []string{"ab", "a", "あ", "士水", "%20", "%E5"} {
		phone.must(http.MethodGet, "/api/japanese/lookup/kanji/"+notOneKanji, nil, http.StatusBadRequest)
	}

	kanjiAPI.mu.Lock()
	defer kanjiAPI.mu.Unlock()
	jisho.mu.Lock()
	defer jisho.mu.Unlock()
	if len(kanjiAPI.asked) != 0 || len(jisho.keywords) != 0 {
		t.Errorf("asked kanjiapi.dev %d times and Jisho %d times, want neither", len(kanjiAPI.asked), len(jisho.keywords))
	}
}

func TestAKanjiKanjiAPIDoesNotKnowIsNotFound(t *testing.T) {
	_, kanjiAPIURL := newFakeKanjiAPI(t, nil) // it knows no kanji
	_, jishoURL := newFakeJisho(t, 200, `{"meta":{"status":200},"data":[]}`)
	_, phone := signedInBrowserWith(t, japanese.Config{KanjiAPIURL: kanjiAPIURL, JishoURL: jishoURL, KanjiAPIPace: pace})

	phone.must(http.MethodGet, "/api/japanese/lookup/kanji/龘", nil, http.StatusNotFound)
}

func TestAKanjiLookupKanjiAPIWontAnswerIsUnavailable(t *testing.T) {
	// kanjiapi.dev rate-limits: it answers every time, but without the kanji.
	_, kanjiAPIURL := newFakeKanjiAPI(t, map[string][]kanjiReply{"士": {{200, `{"error":"Too many requests"}`}}})
	_, jishoURL := newFakeJisho(t, 200, shiWords)
	_, phone := signedInBrowserWith(t, japanese.Config{KanjiAPIURL: kanjiAPIURL, JishoURL: jishoURL, KanjiAPIPace: pace})

	phone.must(http.MethodGet, "/api/japanese/lookup/kanji/士", nil, http.StatusServiceUnavailable)
}

func TestAKanjiOutsideTheSchoolGradesIsNotJoyo(t *testing.T) {
	// 龘 is in no school grade and no JLPT level: kanjiapi.dev says so with nulls.
	_, kanjiAPIURL := newFakeKanjiAPI(t, map[string][]kanjiReply{"龘": {{200, `{"kanji":"龘","grade":null,"stroke_count":48,
	  "meanings":["appearance of a dragon"],"kun_readings":[],"on_readings":["トウ","ドウ"],"jlpt":null}`}}})
	_, jishoURL := newFakeJisho(t, 200, `{"meta":{"status":200},"data":[]}`)
	_, phone := signedInBrowserWith(t, japanese.Config{KanjiAPIURL: kanjiAPIURL, JishoURL: jishoURL, KanjiAPIPace: pace})

	got := get[kanjiLookupJSON](phone, "/api/japanese/lookup/kanji/龘")

	want := kanjiLookupJSON{
		Kanji: "龘", Meanings: []string{"appearance of a dragon"}, On: []string{"トウ", "ドウ"}, Kun: []string{},
		Strokes: 48, Grade: nil, Joyo: false, JLPT: nil, Words: []lookupWordJSON{},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Kanji Lookup = %+v,\nwant %+v", got, want)
	}
}

func TestAKanjiLookupStandsWithoutJisho(t *testing.T) {
	_, kanjiAPIURL := newFakeKanjiAPI(t, map[string][]kanjiReply{"士": {{200, shi}}})
	_, jishoURL := newFakeJisho(t, http.StatusServiceUnavailable, `{}`)
	_, phone := signedInBrowserWith(t, japanese.Config{KanjiAPIURL: kanjiAPIURL, JishoURL: jishoURL, KanjiAPIPace: pace})

	got := get[kanjiLookupJSON](phone, "/api/japanese/lookup/kanji/士")

	if got.Kanji != "士" || !reflect.DeepEqual(got.Meanings, []string{"gentleman", "scholar", "samurai"}) || got.Words == nil || len(got.Words) != 0 {
		t.Errorf("Kanji Lookup = %+v, want 士's from kanjiapi.dev with words []", got)
	}
}
