package service

import (
	"context"
	"errors"
	"net/http"

	coreutils "github.com/feriosch/nando-server/internal/core/utils"
	"github.com/feriosch/nando-server/internal/japanese/model"
	"github.com/feriosch/nando-server/internal/japanese/utils"
)

// WordsService answers the Word page. It reads Groups too, for the Word's
// Group, and Kanji, for those in its spelling.
type WordsService struct {
	words  model.WordsRepository
	groups model.GroupsRepository
	kanji  model.KanjiRepository
}

// NewWords returns the WordsService.
func NewWords(words model.WordsRepository, groups model.GroupsRepository, kanji model.KanjiRepository) *WordsService {
	return &WordsService{words: words, groups: groups, kanji: kanji}
}

// Routes adds the Word routes to mux, under /api/japanese/words.
func (s *WordsService) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/japanese/words/{id}", s.getWord)
}

func (s *WordsService) getWord(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	word, err := s.words.GetOneByID(ctx, r.PathValue("id"))
	if errors.Is(err, coreutils.ErrNotFound) {
		http.Error(w, "there's no such Word", http.StatusNotFound)
		return
	}
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	group, err := s.groups.GetOneByKey(ctx, word.Group)
	if errors.Is(err, coreutils.ErrNotFound) {
		// Every Word's Group is stored; if one isn't, its key is all there is to show.
		group = model.Group{Key: word.Group, Name: word.Group}
	} else if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	j := model.WordJSON{
		ID:        word.ID,
		Word:      word.Word,
		Hiragana:  utils.OrNull(word.Hiragana),
		Spanish:   word.Spanish,
		Type:      word.Type,
		Group:     model.GroupJSON{Key: group.Key, Name: group.Name, ImageURL: utils.OrNull(group.ImageURL)},
		Tags:      utils.OrEmptyList(word.Tags),
		Notes:     utils.OrNull(word.Notes),
		Sentences: []model.SentenceJSON{},
		Hints:     []model.HintJSON{},
	}
	if j.Type == nil {
		j.Type = map[string]int{}
	}
	for _, sentence := range word.Sentences {
		j.Sentences = append(j.Sentences, model.SentenceJSON(sentence))
	}
	if j.WordLinks, err = s.wordLinkJSONs(ctx, word.WordLinks); err != nil {
		coreutils.ServerError(w, err)
		return
	}
	for _, hint := range word.Hints {
		j.Hints = append(j.Hints, model.HintJSON{Text: hint.Text, Note: utils.OrNull(hint.Note)})
	}
	kanji, err := s.kanji.GetManyBySpelling(ctx, word.Word)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	j.Kanji = utils.ToKanjiRowJSONs(kanji)
	coreutils.WriteJSON(w, http.StatusOK, j)
}

// wordLinkJSONs returns wordLinks as the Word page shows them, each with the other
// Word, leaving out any to a Word that isn't stored.
func (s *WordsService) wordLinkJSONs(ctx context.Context, wordLinks []model.WordLink) ([]model.WordLinkJSON, error) {
	list := []model.WordLinkJSON{}
	if len(wordLinks) == 0 {
		return list, nil
	}
	ids := make([]string, len(wordLinks))
	for i, wordLink := range wordLinks {
		ids[i] = wordLink.WordID
	}
	others, err := s.words.GetManyByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := map[string]model.WordRowJSON{}
	for _, row := range utils.ToWordRowJSONs(others) {
		byID[row.ID] = row
	}
	for _, wordLink := range wordLinks {
		other, ok := byID[wordLink.WordID]
		if !ok {
			continue
		}
		list = append(list, model.WordLinkJSON{
			Kind:       wordLink.Kind,
			Note:       utils.OrNull(wordLink.Note),
			NuanceTags: utils.OrEmptyList(wordLink.NuanceTags),
			Word:       other,
		})
	}
	return list, nil
}
