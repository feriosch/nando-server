package service

import (
	"context"
	"errors"
	"net/http"
	"slices"

	coreutils "github.com/feriosch/nando-server/internal/core/utils"
	"github.com/feriosch/nando-server/internal/japanese/model"
	"github.com/feriosch/nando-server/internal/japanese/utils"
)

// KanjiService answers the Kanji page. It reads irregular Components, for
// the Kanji's Components, and Words, for those spelled with it.
type KanjiService struct {
	kanji     model.KanjiRepository
	irregular model.IrregularComponentsRepository
	words     model.WordsRepository
}

// NewKanji returns the KanjiService.
func NewKanji(kanji model.KanjiRepository, irregular model.IrregularComponentsRepository, words model.WordsRepository) *KanjiService {
	return &KanjiService{kanji: kanji, irregular: irregular, words: words}
}

// Routes adds the Kanji routes to mux, under /api/japanese/kanji.
func (s *KanjiService) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/japanese/kanji/{id}", s.getKanji)
}

// maxKanjiWords is how many Words spelled with it a Kanji page shows at most.
const maxKanjiWords = 50

func (s *KanjiService) getKanji(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	kanji, err := s.kanji.GetOneByID(ctx, r.PathValue("id"))
	if errors.Is(err, coreutils.ErrNotFound) {
		http.Error(w, "there's no such Kanji", http.StatusNotFound)
		return
	}
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	components, err := s.components(ctx, kanji.Components)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	words, err := s.words.GetManyByKanji(ctx, kanji.Kanji, maxKanjiWords)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	j := model.KanjiJSON{
		ID:         kanji.ID,
		Kanji:      kanji.Kanji,
		Spanish:    kanji.Spanish,
		On:         utils.OrNull(kanji.On),
		Kun:        utils.OrNull(kanji.Kun),
		V1:         kanji.V1,
		Story:      utils.OrNull(kanji.Story),
		Components: components,
		Words:      utils.ToWordRowJSONs(words),
	}
	if kanji.V2 != 0 {
		j.V2 = &kanji.V2
	}
	coreutils.WriteJSON(w, http.StatusOK, j)
}

// components returns the Components with these keywords, in order. As benkyo
// worked out a Kanji's radicals, a keyword stored as an irregular shape is
// that shape even if a Kanji has the same keyword; any other is the Kanji
// with that keyword if one is stored, or else an irregular shape too.
func (s *KanjiService) components(ctx context.Context, keywords []string) ([]model.ComponentJSON, error) {
	list := []model.ComponentJSON{}
	if len(keywords) == 0 {
		return list, nil
	}
	irregular, err := s.irregular.GetManyByKeywords(ctx, keywords)
	if err != nil {
		return nil, err
	}
	kanji, err := s.kanji.GetManyByKeywords(ctx, keywords)
	if err != nil {
		return nil, err
	}
	byKeyword := map[string]model.KanjiRowJSON{}
	for _, row := range utils.ToKanjiRowJSONs(kanji) {
		byKeyword[row.Spanish] = row
	}
	for _, keyword := range keywords {
		c := model.ComponentJSON{Name: keyword}
		if row, ok := byKeyword[keyword]; ok && !slices.Contains(irregular, keyword) {
			c.Kanji = &row
		}
		list = append(list, c)
	}
	return list, nil
}
