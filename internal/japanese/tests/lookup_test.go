package tests

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"

	"github.com/feriosch/nando-server/internal/japanese"
)

// fakeJisho is jisho.org's search API, answering every search with answer
// and keeping the keywords it was asked for.
type fakeJisho struct {
	mu       sync.Mutex
	keywords []string
}

func newFakeJisho(t *testing.T, status int, answer string) (*fakeJisho, string) {
	t.Helper()
	f := &fakeJisho{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/search/words" {
			t.Errorf("jisho asked for %s, want /api/v1/search/words", r.URL.Path)
		}
		f.mu.Lock()
		f.keywords = append(f.keywords, r.URL.Query().Get("keyword"))
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(answer))
	}))
	t.Cleanup(srv.Close)
	return f, srv.URL
}

// sasayaku is jisho's answer for 囁く, as its API gives it, trimmed to one
// entry with fields the Tool leaves out (links, source, attribution).
const sasayaku = `{"meta":{"status":200},"data":[{
  "slug":"囁く","is_common":true,"tags":[],"jlpt":["jlpt-n1"],
  "japanese":[{"word":"囁く","reading":"ささやく"},{"word":"私語く","reading":"ささやく"}],
  "senses":[
    {"english_definitions":["to whisper","to murmur"],"parts_of_speech":["Godan verb with 'ku' ending","Intransitive verb"],
     "links":[],"tags":["Usually written using kana alone"],"restrictions":[],"see_also":["呟く"],"antonyms":[],"source":[],"info":[]},
    {"english_definitions":["to rumor","to rumour"],"parts_of_speech":["Godan verb with 'ku' ending"],
     "links":[],"tags":[],"restrictions":[],"see_also":[],"antonyms":["叫ぶ"],"source":[],"info":[]}
  ],
  "attribution":{"jmdict":true,"jmnedict":false,"dbpedia":false}
}]}`

// jishoJSON is what japanese_lookup_jisho answers: jisho's entries, trimmed.
type jishoJSON struct {
	Entries []jishoEntryJSON `json:"entries"`
}

type jishoEntryJSON struct {
	Slug     string              `json:"slug"`
	IsCommon bool                `json:"isCommon"`
	JLPT     []string            `json:"jlpt"`
	Japanese []jishoJapaneseJSON `json:"japanese"`
	Senses   []jishoSenseJSON    `json:"senses"`
}

type jishoJapaneseJSON struct {
	Word    string `json:"word"`
	Reading string `json:"reading"`
}

type jishoSenseJSON struct {
	English       []string `json:"english"`
	PartsOfSpeech []string `json:"partsOfSpeech"`
	Tags          []string `json:"tags"`
	SeeAlso       []string `json:"seeAlso"`
	Antonyms      []string `json:"antonyms"`
}

func TestAConnectorLooksAWordUpOnJisho(t *testing.T) {
	jisho, url := newFakeJisho(t, http.StatusOK, sasayaku)
	_, phone := signedInBrowserWith(t, japanese.Config{JishoURL: url})
	session := phone.connect()

	got := callTool[jishoJSON](t, session, "japanese_lookup_jisho", map[string]any{"keyword": "ささやく"})

	want := jishoJSON{Entries: []jishoEntryJSON{{
		Slug:     "囁く",
		IsCommon: true,
		JLPT:     []string{"jlpt-n1"},
		Japanese: []jishoJapaneseJSON{{"囁く", "ささやく"}, {"私語く", "ささやく"}},
		Senses: []jishoSenseJSON{
			{English: []string{"to whisper", "to murmur"}, PartsOfSpeech: []string{"Godan verb with 'ku' ending", "Intransitive verb"},
				Tags: []string{"Usually written using kana alone"}, SeeAlso: []string{"呟く"}, Antonyms: []string{}},
			{English: []string{"to rumor", "to rumour"}, PartsOfSpeech: []string{"Godan verb with 'ku' ending"},
				Tags: []string{}, SeeAlso: []string{}, Antonyms: []string{"叫ぶ"}},
		},
	}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("japanese_lookup_jisho = %+v,\nwant %+v", got, want)
	}
	if !reflect.DeepEqual(jisho.keywords, []string{"ささやく"}) {
		t.Errorf("jisho was asked for %q, want [ささやく]", jisho.keywords)
	}
}

func TestALookupFailsWhenJishoDoes(t *testing.T) {
	_, url := newFakeJisho(t, http.StatusServiceUnavailable, `{}`)
	_, phone := signedInBrowserWith(t, japanese.Config{JishoURL: url})
	session := phone.connect()

	toolFails(t, session, "japanese_lookup_jisho", map[string]any{"keyword": "囁く"})
	toolFails(t, session, "japanese_lookup_jisho", map[string]any{"keyword": " "})
}
