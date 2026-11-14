package tests

import (
	"net/http"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestTheJapaneseAPINeedsASession(t *testing.T) {
	n := newNando(t)
	foot := word("足", "あし", "pie", "basic")
	n.store("words", foot)
	stranger := &browser{nando: n}

	for _, path := range []string{"/api/japanese/search?q=足", "/api/japanese/words/" + id(foot), "/api/japanese/kanji/" + id(foot), "/api/japanese/kanji?kanji=足", "/api/japanese/lookup/kanji/足", "/api/japanese/anki/jlpt.csv"} {
		if rec := stranger.do(http.MethodGet, path, nil); rec.Code != http.StatusUnauthorized {
			t.Errorf("GET %s without a Session: status = %d, want %d", path, rec.Code, http.StatusUnauthorized)
		}
	}
}

func TestWordRequestsNeedASession(t *testing.T) {
	n := newNando(t)
	parked := parkedRequest("かける")
	n.store("word_requests", parked)
	stranger := &browser{nando: n}

	for _, call := range []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, "/api/japanese/word-requests", nil},
		{http.MethodPost, "/api/japanese/word-requests", map[string]any{"text": "食べた", "kind": "add", "group": "gaming"}},
		{http.MethodPost, "/api/japanese/word-requests/" + id(parked) + "/answer", map[string]any{"answer": "掛ける"}},
		{http.MethodPost, "/api/japanese/word-requests/" + id(parked) + "/retry", nil},
		{http.MethodPost, "/api/japanese/word-requests/" + id(parked) + "/dismiss", nil},
	} {
		if rec := stranger.do(call.method, call.path, call.body); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without a Session: status = %d, want %d", call.method, call.path, rec.Code, http.StatusUnauthorized)
		}
	}
}

func TestUnknownWordsAndKanjiAreNotFound(t *testing.T) {
	n, phone := signedInBrowser(t)
	foot := word("足", "あし", "pie", "basic")
	n.store("words", foot)
	n.store("kanji", kanji("足", "pie", 1308))

	for _, path := range []string{
		"/api/japanese/words/" + bson.NewObjectID().Hex(),
		"/api/japanese/words/not-an-id",
		// A Word's id isn't a Kanji's.
		"/api/japanese/kanji/" + id(foot),
		"/api/japanese/kanji/not-an-id",
	} {
		phone.must(http.MethodGet, path, nil, http.StatusNotFound)
	}
}
