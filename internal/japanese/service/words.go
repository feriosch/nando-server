package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

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
	}, {
		Name: "japanese_list_tags",
		Description: "Lists the Word Tags and the Nuance Tags in use, each with how many times, most used first. " +
			"Check it before inventing a tag, and reuse an existing one rather than add a near-duplicate (rare, not uncommon). " +
			"Answers {\"wordTags\": [{\"tag\", \"count\"}], \"nuanceTags\": [...]}.",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		ReadOnly:    true,
		Handle:      s.listTagsTool,
	}, {
		Name: "japanese_add_word",
		Description: "Adds a new Word. word is its kanji spelling if it has one, however rare, with the reading in hiragana; " +
			"a Word with no kanji spelling is stored in kana with no hiragana. spanish is short, comma-separated senses with context in parentheses. " +
			"type maps parts of speech (noun, suru, no_adj, na_adj, i_adj, adv, verb, adj_noun, adv_noun, counter) to the form shown: " +
			"1 or 2 plain, 3 ～に, 4 ～と, 5 ～的, 6 ～的に, 7 ～たる, 8 ～を. group must be a stored Group's key. " +
			"tags are Word Tags in snake_case; check japanese_list_tags first. " +
			"Refused if a Word with the same spelling and reading is stored (another reading of a spelling is a different Word). " +
			"Add Word Links and Hints afterwards. Answers {\"id\"}.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{` +
			`"word":{"type":"string","description":"The kanji spelling, or the kana if it has none."},` +
			`"hiragana":{"type":"string","description":"The reading; leave out for a kana Word."},` +
			`"spanish":{"type":"string"},` +
			`"type":{"type":"object","additionalProperties":{"type":"integer","minimum":1,"maximum":8}},` +
			`"group":{"type":"string","description":"A Group's key, such as jlpt_n2."},` +
			`"tags":{"type":"array","items":{"type":"string"}},` +
			`"notes":{"type":"string","description":"Such as JLPT N2, or a short Spanish note."},` +
			`"sentences":{"type":"array","items":{"type":"object","properties":{` +
			`"sentence":{"type":"string"},"translation":{"type":"string","description":"In English."}},` +
			`"required":["sentence","translation"]}}},` +
			`"required":["word","spanish","type","group"]}`),
		Handle: s.addWordTool,
	}}
}

// wordFields are a Word's own fields as a Tool is sent them.
type wordFields struct {
	Word      string         `json:"word"`
	Hiragana  string         `json:"hiragana"`
	Spanish   string         `json:"spanish"`
	Type      map[string]int `json:"type"`
	Group     string         `json:"group"`
	Tags      []string       `json:"tags"`
	Notes     string         `json:"notes"`
	Sentences []struct {
		Sentence    string `json:"sentence"`
		Translation string `json:"translation"`
	} `json:"sentences"`
}

// addWordTool answers japanese_add_word.
func (s *WordsService) addWordTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var fields wordFields
	if err := json.Unmarshal(arguments, &fields); err != nil {
		return nil, err
	}
	word, err := s.checkNewWord(ctx, fields)
	if err != nil {
		return nil, err
	}
	id, err := s.words.Insert(ctx, word)
	if errors.Is(err, model.ErrWordExists) {
		return nil, fmt.Errorf("%s (%s) is already stored: look at it with japanese_get_word and improve it instead", word.Word, word.Hiragana)
	}
	if err != nil {
		return nil, err
	}
	return coremodel.CreatedJSON{ID: id}, nil
}

// Limits on what a Word may hold.
const (
	maxSpellingLength = 100
	maxSpanishLength  = 500
	maxNotesLength    = 2000
	maxTags           = 30
	maxTagLength      = 40
	maxSentences      = 10
	maxSentenceLength = 500
)

// wordTypes are the parts of speech a Word's type may have.
var wordTypes = []string{"noun", "suru", "no_adj", "na_adj", "i_adj", "adv", "verb", "adj_noun", "adv_noun", "counter"}

// snakeCase is a tag's form: lowercase English words joined by _.
var snakeCase = regexp.MustCompile(`^[a-z0-9]+(_[a-z0-9]+)*$`)

// checkNewWord checks fields as a new Word and returns it, or explains
// what's wrong.
func (s *WordsService) checkNewWord(ctx context.Context, fields wordFields) (model.Word, error) {
	word := model.Word{
		Word:     strings.TrimSpace(fields.Word),
		Hiragana: strings.TrimSpace(fields.Hiragana),
		Spanish:  strings.TrimSpace(fields.Spanish),
		Type:     fields.Type,
		Group:    fields.Group,
		Tags:     utils.OrEmptyList(fields.Tags),
		Notes:    strings.TrimSpace(fields.Notes),
	}
	switch {
	case word.Word == "" || utf8.RuneCountInString(word.Word) > maxSpellingLength:
		return word, fmt.Errorf("word is required, at most %d characters", maxSpellingLength)
	case utf8.RuneCountInString(word.Hiragana) > maxSpellingLength:
		return word, fmt.Errorf("hiragana must be at most %d characters", maxSpellingLength)
	case word.Spanish == "" || utf8.RuneCountInString(word.Spanish) > maxSpanishLength:
		return word, fmt.Errorf("spanish is required, at most %d characters", maxSpanishLength)
	case utf8.RuneCountInString(word.Notes) > maxNotesLength:
		return word, fmt.Errorf("notes must be at most %d characters", maxNotesLength)
	case len(word.Type) == 0:
		return word, fmt.Errorf("type needs at least one of %s", strings.Join(wordTypes, ", "))
	case len(word.Tags) > maxTags:
		return word, fmt.Errorf("at most %d Word Tags", maxTags)
	case len(fields.Sentences) > maxSentences:
		return word, fmt.Errorf("at most %d sentences", maxSentences)
	}
	for kind, form := range word.Type {
		if !slices.Contains(wordTypes, kind) {
			return word, fmt.Errorf("type %q isn't one of %s", kind, strings.Join(wordTypes, ", "))
		}
		if form < 1 || form > 8 {
			return word, fmt.Errorf("type %s is %d; the form must be 1 to 8", kind, form)
		}
	}
	for _, tag := range word.Tags {
		if !snakeCase.MatchString(tag) || len(tag) > maxTagLength {
			return word, fmt.Errorf("Word Tag %q must be snake_case, such as usually_kana", tag)
		}
	}
	for _, sentence := range fields.Sentences {
		sentence.Sentence, sentence.Translation = strings.TrimSpace(sentence.Sentence), strings.TrimSpace(sentence.Translation)
		if sentence.Sentence == "" || sentence.Translation == "" ||
			utf8.RuneCountInString(sentence.Sentence) > maxSentenceLength || utf8.RuneCountInString(sentence.Translation) > maxSentenceLength {
			return word, fmt.Errorf("each sentence needs its translation, each at most %d characters", maxSentenceLength)
		}
		word.Sentences = append(word.Sentences, model.Sentence(sentence))
	}
	if _, err := s.groups.GetOneByKey(ctx, word.Group); errors.Is(err, coreutils.ErrNotFound) {
		return word, fmt.Errorf("%q isn't a stored Group", word.Group)
	} else if err != nil {
		return word, err
	}
	return word, nil
}

// listTagsTool answers japanese_list_tags.
func (s *WordsService) listTagsTool(ctx context.Context, _ json.RawMessage) (any, error) {
	wordTags, nuanceTags, err := s.words.CountTags(ctx)
	if err != nil {
		return nil, err
	}
	return model.TagsJSON{WordTags: tagCountJSONs(wordTags), NuanceTags: tagCountJSONs(nuanceTags)}, nil
}

// tagCountJSONs is counts as a Tool answers them, [] if there are none.
func tagCountJSONs(counts []model.TagCount) []model.TagCountJSON {
	list := make([]model.TagCountJSON, len(counts))
	for i, c := range counts {
		list[i] = model.TagCountJSON(c)
	}
	return list
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
