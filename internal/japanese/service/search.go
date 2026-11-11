package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	coremodel "github.com/feriosch/nando-server/internal/core/model"
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
	q := normalize(r.URL.Query().Get("q"))
	if utf8.RuneCountInString(q) > maxQLength {
		http.Error(w, "q is longer than 100 characters", http.StatusBadRequest)
		return
	}
	if q == "" {
		coreutils.WriteJSON(w, http.StatusOK, model.SearchJSON{Words: []model.WordRowJSON{}, Kanji: []model.KanjiRowJSON{}})
		return
	}
	found, err := s.searchAll(r.Context(), q)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	coreutils.WriteJSON(w, http.StatusOK, found)
}

// normalize is q trimmed and in NFC, so an accent or dakuten sent as a
// separate combining mark is matched like the stored, composed letter.
func normalize(q string) string {
	return norm.NFC.String(strings.TrimSpace(q))
}

// searchAll finds the Words whose spelling, reading or Spanish meaning
// contains q, and the Kanji in q or whose Spanish meaning contains it.
func (s *SearchService) searchAll(ctx context.Context, q string) (model.SearchJSON, error) {
	words, err := s.words.GetManyBySearch(ctx, q, maxWords)
	if err != nil {
		return model.SearchJSON{}, err
	}
	kanji, err := s.kanji.GetManyBySearch(ctx, q, maxKanji)
	if err != nil {
		return model.SearchJSON{}, err
	}
	return model.SearchJSON{Words: utils.ToWordRowJSONs(words), Kanji: utils.ToKanjiRowJSONs(kanji)}, nil
}

// searchFields are the Word fields japanese_search can search alone.
var searchFields = map[string]string{"spelling": "word", "spanish": "spanish"}

// Tools are the SearchService's Tools for Connectors.
func (s *SearchService) Tools() []coremodel.Tool {
	return []coremodel.Tool{{
		Name: "japanese_search",
		Description: "Searches the stored Words and Kanji as the search screen does: Words whose spelling, reading or " +
			"Spanish meaning contains q, exact matches first, then shorter ones; and Kanji in q or whose Spanish meaning contains it. " +
			"Spanish ignores accents and case. With \"in\", it searches one field of the Words only, and no Kanji: " +
			"\"spelling\" finds Word Link candidates sharing a Kanji (q is the Kanji), \"spanish\" those with a similar meaning. " +
			"Answers {\"words\": [rows], \"kanji\": [rows]}, at most 50 Words and 20 Kanji.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{` +
			`"q":{"type":"string","description":"What to find, at most 100 characters."},` +
			`"in":{"type":"string","enum":["spelling","spanish"],"description":"Search only this field of the Words."}},` +
			`"required":["q"]}`),
		ReadOnly: true,
		Handle:   s.searchTool,
	}}
}

// searchTool answers japanese_search.
func (s *SearchService) searchTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		Q  string `json:"q"`
		In string `json:"in"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	q := normalize(args.Q)
	switch {
	case q == "":
		return nil, errors.New("q is required")
	case utf8.RuneCountInString(q) > maxQLength:
		return nil, errors.New("q is longer than 100 characters")
	case args.In == "":
		return s.searchAll(ctx, q)
	}
	field, ok := searchFields[args.In]
	if !ok {
		return nil, fmt.Errorf(`"in" must be "spelling" or "spanish", not %q`, args.In)
	}
	words, err := s.words.GetManyBySearchIn(ctx, q, field, maxWords)
	if err != nil {
		return nil, err
	}
	return model.SearchJSON{Words: utils.ToWordRowJSONs(words), Kanji: []model.KanjiRowJSON{}}, nil
}
