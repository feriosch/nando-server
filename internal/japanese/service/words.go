package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	coremodel "github.com/feriosch/nando-server/internal/core/model"
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
	word, err := s.words.GetOneByID(r.Context(), r.PathValue("id"))
	if errors.Is(err, coreutils.ErrNotFound) {
		http.Error(w, "there's no such Word", http.StatusNotFound)
		return
	}
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	j, err := s.wordJSON(r.Context(), word)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	coreutils.WriteJSON(w, http.StatusOK, j)
}

// wordJSON is word as its page shows it, with its Group, the other Word of
// each Word Link, and the Kanji in its spelling.
func (s *WordsService) wordJSON(ctx context.Context, word model.Word) (model.WordJSON, error) {
	group, err := s.groups.GetOneByKey(ctx, word.Group)
	if errors.Is(err, coreutils.ErrNotFound) {
		// Every Word's Group is stored; if one isn't, its key is all there is to show.
		group = model.Group{Key: word.Group, Name: word.Group}
	} else if err != nil {
		return model.WordJSON{}, err
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
		return model.WordJSON{}, err
	}
	for _, hint := range word.Hints {
		j.Hints = append(j.Hints, model.HintJSON{Text: hint.Text, Note: utils.OrNull(hint.Note)})
	}
	kanji, err := s.kanji.GetManyBySpelling(ctx, word.Word)
	if err != nil {
		return model.WordJSON{}, err
	}
	j.Kanji = utils.ToKanjiRowJSONs(kanji)
	return j, nil
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

// Tools are the WordsService's Tools for Connectors.
func (s *WordsService) Tools() []coremodel.Tool {
	return []coremodel.Tool{{
		Name: "japanese_get_word",
		Description: "Gets the stored Words with an id, or with a spelling or reading (\"word\" matches either), " +
			"with every field: reading, Spanish meaning, type, Group, Word Tags, notes, example sentences, " +
			"Word Links (with the other Word's spelling, reading, Spanish meaning and Group), Hints, and the Kanji in its spelling. " +
			"Use it to see whether a Word is already stored before adding it. Answers {\"words\": [...]}, empty if none match.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{` +
			`"id":{"type":"string","description":"A Word's id."},` +
			`"word":{"type":"string","description":"A spelling or a reading, such as 足跡 or あしあと."}}}`),
		ReadOnly: true,
		Handle:   s.getWordTool,
	}}
}

// getWordTool answers japanese_get_word.
func (s *WordsService) getWordTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		ID   string `json:"id"`
		Word string `json:"word"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	var found []model.Word
	switch {
	case args.ID != "" && args.Word != "", args.ID == "" && args.Word == "":
		return nil, errors.New(`give either "id" or "word"`)
	case args.ID != "":
		word, err := s.words.GetOneByID(ctx, args.ID)
		if err != nil && !errors.Is(err, coreutils.ErrNotFound) {
			return nil, err
		}
		if err == nil {
			found = []model.Word{word}
		}
	default:
		var err error
		if found, err = s.words.GetManyBySpellingOrReading(ctx, args.Word); err != nil {
			return nil, err
		}
	}
	answer := model.WordsJSON{Words: []model.WordJSON{}}
	for _, word := range found {
		j, err := s.wordJSON(ctx, word)
		if err != nil {
			return nil, err
		}
		answer.Words = append(answer.Words, j)
	}
	return answer, nil
}
