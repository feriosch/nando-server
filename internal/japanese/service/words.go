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

type groupJSON struct {
	Key      string  `json:"key"`
	Name     string  `json:"name"`
	ImageURL *string `json:"imageUrl"`
}

type sentenceJSON struct {
	Sentence    string `json:"sentence"`
	Translation string `json:"translation"`
}

type wordLinkJSON struct {
	Kind       string            `json:"kind"`
	Note       *string           `json:"note"`
	NuanceTags []string          `json:"nuanceTags"`
	Word       utils.WordRowJSON `json:"word"`
}

type hintJSON struct {
	Text string  `json:"text"`
	Note *string `json:"note"`
}

// wordJSON is a Word as its page shows it.
type wordJSON struct {
	ID        string               `json:"id"`
	Word      string               `json:"word"`
	Hiragana  *string              `json:"hiragana"`
	Spanish   string               `json:"spanish"`
	Type      map[string]int       `json:"type"`
	Group     groupJSON            `json:"group"`
	Tags      []string             `json:"tags"`
	Notes     *string              `json:"notes"`
	Sentences []sentenceJSON       `json:"sentences"`
	WordLinks []wordLinkJSON       `json:"wordLinks"`
	Hints     []hintJSON           `json:"hints"`
	Kanji     []utils.KanjiRowJSON `json:"kanji"`
}

func (s *WordsService) getWord(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	word, err := s.words.Find(ctx, r.PathValue("id"))
	if errors.Is(err, coreutils.ErrNotFound) {
		http.Error(w, "there's no such Word", http.StatusNotFound)
		return
	}
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	group, err := s.groups.Find(ctx, word.Group)
	if errors.Is(err, coreutils.ErrNotFound) {
		// Every Word's Group is stored; if one isn't, its key is all there is to show.
		group = model.Group{Key: word.Group, Name: word.Group}
	} else if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	j := wordJSON{
		ID:        word.ID,
		Word:      word.Word,
		Hiragana:  utils.OrNull(word.Hiragana),
		Spanish:   word.Spanish,
		Type:      word.Type,
		Group:     groupJSON{Key: group.Key, Name: group.Name, ImageURL: utils.OrNull(group.ImageURL)},
		Tags:      utils.OrEmptyList(word.Tags),
		Notes:     utils.OrNull(word.Notes),
		Sentences: []sentenceJSON{},
		Hints:     []hintJSON{},
	}
	if j.Type == nil {
		j.Type = map[string]int{}
	}
	for _, sentence := range word.Sentences {
		j.Sentences = append(j.Sentences, sentenceJSON(sentence))
	}
	if j.WordLinks, err = s.wordLinks(ctx, word.Links); err != nil {
		coreutils.ServerError(w, err)
		return
	}
	for _, hint := range word.Hints {
		j.Hints = append(j.Hints, hintJSON{Text: hint.Text, Note: utils.OrNull(hint.Note)})
	}
	kanji, err := s.kanji.InText(ctx, word.Word)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	j.Kanji = utils.ToKanjiRowJSONs(kanji)
	coreutils.WriteJSON(w, http.StatusOK, j)
}

// wordLinks returns links as the Word page shows them, each with the other
// Word, leaving out any to a Word that isn't stored.
func (s *WordsService) wordLinks(ctx context.Context, links []model.WordLink) ([]wordLinkJSON, error) {
	ids := make([]string, len(links))
	for i, l := range links {
		ids[i] = l.WordID
	}
	others, err := s.words.FindMany(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := map[string]utils.WordRowJSON{}
	for _, row := range utils.ToWordRowJSONs(others) {
		byID[row.ID] = row
	}
	list := []wordLinkJSON{}
	for _, l := range links {
		other, ok := byID[l.WordID]
		if !ok {
			continue
		}
		list = append(list, wordLinkJSON{
			Kind:       l.Kind,
			Note:       utils.OrNull(l.Note),
			NuanceTags: utils.OrEmptyList(l.NuanceTags),
			Word:       other,
		})
	}
	return list, nil
}
