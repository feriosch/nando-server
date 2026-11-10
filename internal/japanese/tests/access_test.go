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

	for _, path := range []string{"/api/japanese/search?q=足", "/api/japanese/words/" + id(foot), "/api/japanese/kanji/" + id(foot), "/api/japanese/anki/jlpt.csv"} {
		if rec := stranger.do(http.MethodGet, path, nil); rec.Code != http.StatusUnauthorized {
			t.Errorf("GET %s without a Session: status = %d, want %d", path, rec.Code, http.StatusUnauthorized)
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
