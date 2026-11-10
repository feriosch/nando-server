package service

import (
	"net/http"
	"strings"

	coreutils "github.com/feriosch/nando-server/internal/core/utils"
	"github.com/feriosch/nando-server/internal/japanese/model"
	"github.com/feriosch/nando-server/internal/japanese/utils"
)

// SearchService answers the search screen from Words and Kanji.
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

// maxWords and maxKanji are how many Words and Kanji a search returns at most.
const (
	maxWords = 50
	maxKanji = 20
)

type searchJSON struct {
	Words []utils.WordRowJSON  `json:"words"`
	Kanji []utils.KanjiRowJSON `json:"kanji"`
}

func (s *SearchService) search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		coreutils.WriteJSON(w, http.StatusOK, searchJSON{Words: []utils.WordRowJSON{}, Kanji: []utils.KanjiRowJSON{}})
		return
	}
	words, err := s.words.Search(r.Context(), q, maxWords)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	kanji, err := s.kanji.Search(r.Context(), q, maxKanji)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	coreutils.WriteJSON(w, http.StatusOK, searchJSON{
		Words: utils.ToWordRowJSONs(words),
		Kanji: utils.ToKanjiRowJSONs(kanji),
	})
}
