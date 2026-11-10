package service

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	coreutils "github.com/feriosch/nando-server/internal/core/utils"
	"github.com/feriosch/nando-server/internal/japanese/model"
	"github.com/feriosch/nando-server/internal/japanese/utils"
)

// SearchService answers the search screen from Words and Kanji. It serves no
// single collection, so it is neither the Words nor the Kanji service.
type SearchService struct {
	words model.WordsRepository
	kanji model.KanjiRepository
}

// NewSearch returns the SearchService.
func NewSearch(words model.WordsRepository, kanji model.KanjiRepository) *SearchService {
	return &SearchService{words: words, kanji: kanji}
}

// Routes adds the search route to mux, at /api/japanese/search.
func (s *SearchService) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/japanese/search", s.search)
}

// maxWords and maxKanji are how many Words and Kanji a search returns at
// most, and maxQLength how many characters it is sent at most.
const (
	maxWords   = 50
	maxKanji   = 20
	maxQLength = 100
)

func (s *SearchService) search(w http.ResponseWriter, r *http.Request) {
	// NFC, so an accent or dakuten sent as a separate combining mark is
	// matched like the stored, composed letter.
	q := norm.NFC.String(strings.TrimSpace(r.URL.Query().Get("q")))
	if utf8.RuneCountInString(q) > maxQLength {
		http.Error(w, "q is longer than 100 characters", http.StatusBadRequest)
		return
	}
	if q == "" {
		coreutils.WriteJSON(w, http.StatusOK, model.SearchJSON{Words: []model.WordRowJSON{}, Kanji: []model.KanjiRowJSON{}})
		return
	}
	words, err := s.words.GetManyBySearch(r.Context(), q, maxWords)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	kanji, err := s.kanji.GetManyBySearch(r.Context(), q, maxKanji)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	coreutils.WriteJSON(w, http.StatusOK, model.SearchJSON{
		Words: utils.ToWordRowJSONs(words),
		Kanji: utils.ToKanjiRowJSONs(kanji),
	})
}
