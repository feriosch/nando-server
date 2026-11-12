package tests

import (
	"net/http"
	"reflect"
	"testing"
)

// candidateKanjiJSON is a stored Kanji as GET /api/japanese/kanji?kanji=
// answers it, for one of a Drawing's Candidates.
type candidateKanjiJSON struct {
	ID      string `json:"id"`
	Kanji   string `json:"kanji"`
	Spanish string `json:"spanish"`
	V1      int    `json:"v1"`
}

func TestCandidatesThatAreStoredKanjiComeBackInTheOrderAsked(t *testing.T) {
	n, phone := signedInBrowser(t)
	water := kanji("水", "agua", 137)
	faction := kanji("派", "facción", 1045)
	n.store("kanji", water, faction, kanji("山", "montaña", 785))

	// 氷 isn't stored, so it is left out.
	got := get[[]candidateKanjiJSON](phone, "/api/japanese/kanji?kanji=派水氷")

	want := []candidateKanjiJSON{
		{ID: id(faction), Kanji: "派", Spanish: "facción", V1: 1045},
		{ID: id(water), Kanji: "水", Spanish: "agua", V1: 137},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Kanji = %+v,\nwant %+v", got, want)
	}
}

func TestACandidateAskedForTwiceComesBackOnce(t *testing.T) {
	n, phone := signedInBrowser(t)
	water := kanji("水", "agua", 137)
	ice := kanji("氷", "hielo", 138)
	n.store("kanji", water, ice)

	got := get[[]candidateKanjiJSON](phone, "/api/japanese/kanji?kanji=水氷水")

	want := []candidateKanjiJSON{
		{ID: id(water), Kanji: "水", Spanish: "agua", V1: 137},
		{ID: id(ice), Kanji: "氷", Spanish: "hielo", V1: 138},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Kanji = %+v,\nwant %+v", got, want)
	}
}

func TestNoCandidatesFindNoKanji(t *testing.T) {
	n, phone := signedInBrowser(t)
	n.store("kanji", kanji("水", "agua", 137))

	for _, path := range []string{"/api/japanese/kanji?kanji=", "/api/japanese/kanji?kanji=%20%20", "/api/japanese/kanji"} {
		if got := get[[]candidateKanjiJSON](phone, path); got == nil || len(got) != 0 {
			t.Errorf("GET %s = %#v, want []", path, got)
		}
	}
}

func TestAtMostEightCandidatesCanBeAskedFor(t *testing.T) {
	n, phone := signedInBrowser(t)
	water := kanji("水", "agua", 137)
	n.store("kanji", water)

	// 8 kanji are 24 bytes in UTF-8, but the limit counts kanji.
	got := get[[]candidateKanjiJSON](phone, "/api/japanese/kanji?kanji=派水氷次永泳承旅")
	if want := []candidateKanjiJSON{{ID: id(water), Kanji: "水", Spanish: "agua", V1: 137}}; !reflect.DeepEqual(got, want) {
		t.Errorf("8 kanji = %+v, want %+v", got, want)
	}
	phone.must(http.MethodGet, "/api/japanese/kanji?kanji=派水氷次永泳承旅採", nil, http.StatusBadRequest)
}

func TestAKanjiStoredTwiceComesBackOnce(t *testing.T) {
	n, phone := signedInBrowser(t)
	water := kanji("水", "agua", 137)
	n.store("kanji", water, kanji("水", "agua", 137)) // a duplicate left over from benkyo, stored after

	got := get[[]candidateKanjiJSON](phone, "/api/japanese/kanji?kanji=水")

	if want := []candidateKanjiJSON{{ID: id(water), Kanji: "水", Spanish: "agua", V1: 137}}; !reflect.DeepEqual(got, want) {
		t.Errorf("Kanji = %+v,\nwant %+v", got, want)
	}
}

func TestCandidatesThatAreNotKanjiAreLeftOut(t *testing.T) {
	n, phone := signedInBrowser(t)
	water := kanji("水", "agua", 137)
	n.store("kanji", water)

	got := get[[]candidateKanjiJSON](phone, "/api/japanese/kanji?kanji=あ水x")

	if want := []candidateKanjiJSON{{ID: id(water), Kanji: "水", Spanish: "agua", V1: 137}}; !reflect.DeepEqual(got, want) {
		t.Errorf("Kanji = %+v,\nwant %+v", got, want)
	}
}

func TestCandidatesThatAreNotValidUTF8FindNoKanji(t *testing.T) {
	n, phone := signedInBrowser(t)
	n.store("kanji", kanji("水", "agua", 137))

	if got := get[[]candidateKanjiJSON](phone, "/api/japanese/kanji?kanji=%FF%FE"); got == nil || len(got) != 0 {
		t.Errorf("Kanji = %#v, want []", got)
	}
}
