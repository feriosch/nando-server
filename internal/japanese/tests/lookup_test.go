package tests

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

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

// fakeKanjiAPI is kanjiapi.dev, answering each Kanji with the next of its
// replies (the last one repeats), and keeping when it was asked.
type fakeKanjiAPI struct {
	mu      sync.Mutex
	replies map[string][]kanjiReply
	asked   []time.Time
}

// kanjiReply is one answer of kanjiapi.dev: a status and a body.
type kanjiReply struct {
	status int
	body   string
}

func newFakeKanjiAPI(t *testing.T, replies map[string][]kanjiReply) (*fakeKanjiAPI, string) {
	t.Helper()
	f := &fakeKanjiAPI{replies: replies}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		character, ok := strings.CutPrefix(r.URL.Path, "/v1/kanji/")
		f.mu.Lock()
		f.asked = append(f.asked, time.Now())
		queue := f.replies[character]
		reply := kanjiReply{http.StatusNotFound, `{"error":"No kanji found"}`}
		if len(queue) > 0 {
			reply = queue[0]
			if len(queue) > 1 {
				f.replies[character] = queue[1:]
			}
		}
		f.mu.Unlock()
		if !ok {
			t.Errorf("kanjiapi asked for %s, want /v1/kanji/<kanji>", r.URL.Path)
		}
		w.WriteHeader(reply.status)
		_, _ = w.Write([]byte(reply.body))
	}))
	t.Cleanup(srv.Close)
	return f, srv.URL
}

// joyoJSON is what japanese_check_joyo answers: each Kanji in the text.
type joyoJSON struct {
	Kanji []kanjiJoyoJSON `json:"kanji"`
}

type kanjiJoyoJSON struct {
	Kanji  string `json:"kanji"`
	Stored bool   `json:"stored"`
	Joyo   *bool  `json:"joyo"`
	Grade  *int   `json:"grade"`
}

func bptr(b bool) *bool { return &b }
func iptr(i int) *int   { return &i }

// pace is how far apart the tests' fake kanjiapi is asked.
const pace = 30 * time.Millisecond

func TestAConnectorChecksWhetherEachKanjiIsJoyo(t *testing.T) {
	rateLimited := kanjiReply{http.StatusOK, `{"message":"Too many requests"}`}
	_, url := newFakeKanjiAPI(t, map[string][]kanjiReply{
		"捏": {{http.StatusOK, `{"kanji":"捏","grade":null}`}},
		"造": {{http.StatusOK, `{"kanji":"造","grade":5}`}},
		"嘔": {rateLimited, {http.StatusOK, `{"kanji":"嘔","grade":null}`}},
		"鬱": {{http.StatusOK, `{"kanji":"鬱","grade":8}`}},
		"凜": {{http.StatusOK, `{"kanji":"凜","grade":9}`}},
	})
	n, phone := signedInBrowserWith(t, japanese.Config{KanjiAPIURL: url, KanjiAPIPace: pace})
	n.store("kanji", kanji("造", "crear", 693), kanji("鬱", "melancolía", 2032))
	session := phone.connect()

	got := callTool[joyoJSON](t, session, "japanese_check_joyo", map[string]any{"text": "捏造、嘔吐く鬱凜々捏"})

	want := []kanjiJoyoJSON{
		{"捏", false, bptr(false), nil},
		{"造", true, bptr(true), iptr(5)},
		{"嘔", false, bptr(false), nil}, // rate limited once, then answered
		{"吐", false, bptr(false), nil}, // kanjiapi doesn't know it: 404
		{"鬱", true, bptr(true), iptr(8)},
		{"凜", false, bptr(false), iptr(9)}, // jinmeiyō, not jōyō
	}
	if !reflect.DeepEqual(got.Kanji, want) {
		t.Errorf("japanese_check_joyo =\n%+v\nwant\n%+v", got.Kanji, want)
	}
}

func TestARateLimitIsNeverReportedAsNotJoyo(t *testing.T) {
	_, url := newFakeKanjiAPI(t, map[string][]kanjiReply{
		"海": {{http.StatusOK, `{"message":"Too many requests"}`}},
		"山": {{http.StatusTooManyRequests, `{}`}},
	})
	n, phone := signedInBrowserWith(t, japanese.Config{KanjiAPIURL: url, KanjiAPIPace: pace})
	n.store("kanji", kanji("海", "mar", 491))
	session := phone.connect()

	got := callTool[joyoJSON](t, session, "japanese_check_joyo", map[string]any{"text": "海山"})

	want := []kanjiJoyoJSON{{"海", true, nil, nil}, {"山", false, nil, nil}}
	if !reflect.DeepEqual(got.Kanji, want) {
		t.Errorf("japanese_check_joyo while rate limited = %+v, want joyo null for both, never false", got.Kanji)
	}
}

func TestKanjiAPIIsAskedAtTheConfiguredPace(t *testing.T) {
	kanjiAPI, url := newFakeKanjiAPI(t, map[string][]kanjiReply{
		"一": {{http.StatusOK, `{"grade":1}`}},
		"二": {{http.StatusOK, `{"grade":1}`}},
		"三": {{http.StatusOK, `{"grade":1}`}},
	})
	_, phone := signedInBrowserWith(t, japanese.Config{KanjiAPIURL: url, KanjiAPIPace: pace})
	session := phone.connect()

	callTool[joyoJSON](t, session, "japanese_check_joyo", map[string]any{"text": "一二三"})

	if len(kanjiAPI.asked) != 3 {
		t.Fatalf("kanjiapi was asked %d times, want 3", len(kanjiAPI.asked))
	}
	for i := 1; i < len(kanjiAPI.asked); i++ {
		if gap := kanjiAPI.asked[i].Sub(kanjiAPI.asked[i-1]); gap < pace {
			t.Errorf("kanjiapi asked again after %v, want at least %v apart", gap, pace)
		}
	}
}

func TestACheckWithoutKanjiIsRefused(t *testing.T) {
	_, url := newFakeKanjiAPI(t, nil)
	_, phone := signedInBrowserWith(t, japanese.Config{KanjiAPIURL: url, KanjiAPIPace: pace})
	session := phone.connect()

	toolFails(t, session, "japanese_check_joyo", map[string]any{"text": "ささやく"})
}
