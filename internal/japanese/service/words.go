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
	}, {
		Name: "japanese_update_word",
		Description: "Changes only the fields given of the Word with id, leaving the rest as they are: word, hiragana, spanish, type, " +
			"tags (the whole list), notes, sentences (the whole list). An empty hiragana makes it a kana Word; empty notes removes them. " +
			"Use it to improve a Word already stored, such as moving a kana Word to its kanji spelling with usually_kana. " +
			"It never changes the Group (japanese_change_group) or Word Links and Hints (their own Tools). " +
			"Refused if the new spelling and reading are another Word's. Answers the Word as it is now.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{` +
			`"id":{"type":"string"},"word":{"type":"string"},"hiragana":{"type":"string"},"spanish":{"type":"string"},` +
			`"type":{"type":"object","additionalProperties":{"type":"integer","minimum":1,"maximum":8}},` +
			`"tags":{"type":"array","items":{"type":"string"}},"notes":{"type":"string"},` +
			`"sentences":{"type":"array","items":{"type":"object","properties":{` +
			`"sentence":{"type":"string"},"translation":{"type":"string"}},"required":["sentence","translation"]}}},` +
			`"required":["id"]}`),
		Handle: s.updateWordTool,
	}, {
		Name: "japanese_add_word_link",
		Description: "Links two stored Words both ways. from gets a side about to: kind (synonym, antonym or related), " +
			"note (in English: how the to Word differs from the from Word, no JLPT level) and nuanceTags " +
			"(how the to Word is used compared with the from Word, such as formal, written, rare, expression, part). " +
			"to gets the way back automatically, with noteBack (the same note if left out) and the opposite Nuance Tags: " +
			"formal and casual, spoken and written, expression and part swap; others stay. " +
			"A side is skipped if that Word already links to the other; change it with japanese_update_word_link. " +
			"Answers {\"fromSideWritten\", \"backSideWritten\"}.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{` +
			`"from":{"type":"string","description":"A Word's id."},"to":{"type":"string","description":"The other Word's id."},` +
			`"kind":{"type":"string","enum":["synonym","antonym","related"]},` +
			`"note":{"type":"string"},"nuanceTags":{"type":"array","items":{"type":"string"}},` +
			`"noteBack":{"type":"string","description":"The way back's note, about the from Word."}},` +
			`"required":["from","to","kind"]}`),
		Handle: s.addWordLinkTool,
	}, {
		Name: "japanese_update_word_link",
		Description: "Changes only from's side of its Word Link to to: the kind, note or nuanceTags given (nuanceTags is the whole list). " +
			"The way back is left as it is; change it with another call, from the other Word. Answers the from Word as it is now.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{` +
			`"from":{"type":"string"},"to":{"type":"string"},` +
			`"kind":{"type":"string","enum":["synonym","antonym","related"]},` +
			`"note":{"type":"string"},"nuanceTags":{"type":"array","items":{"type":"string"}}},` +
			`"required":["from","to"]}`),
		Handle: s.updateWordLinkTool,
	}, {
		Name:        "japanese_remove_word_link",
		Description: "Removes the Word Link between two Words: both sides. Answers {\"fromSideRemoved\", \"backSideRemoved\"}.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{` +
			`"from":{"type":"string"},"to":{"type":"string"}},"required":["from","to"]}`),
		Handle: s.removeWordLinkTool,
	}, {
		Name: "japanese_add_hint",
		Description: "Adds a Hint to a Word: a one-way, text-only pointer to an everyday word that isn't stored as a Word, " +
			"such as 天気, with an optional note. It has no other side. A stored Word gets a Word Link instead (japanese_add_word_link), " +
			"so text that a stored Word is spelled or read as is refused. A Hint with the same text is skipped. Answers {\"written\"}.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{` +
			`"word":{"type":"string","description":"The id of the Word that gets the Hint."},` +
			`"text":{"type":"string","description":"The everyday word, such as 天気."},"note":{"type":"string"}},` +
			`"required":["word","text"]}`),
		Handle: s.addHintTool,
	}, {
		Name:        "japanese_remove_hint",
		Description: "Removes a Word's Hint with that text.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{` +
			`"word":{"type":"string","description":"The id of the Word with the Hint."},"text":{"type":"string"}},` +
			`"required":["word","text"]}`),
		Handle: s.removeHintTool,
	}, {
		Name: "japanese_delete_word",
		Description: "Deletes a Word and every Word Link to it, on every other Word. It can't be undone: ask the Owner first. " +
			"To keep what a duplicate holds, use japanese_merge_words instead.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`),
		Destructive: true,
		Handle:      s.deleteWordTool,
	}, {
		Name: "japanese_merge_words",
		Description: "Merges two Words that are the same: keeps one and deletes the other. First set the kept Word's fields with " +
			"japanese_update_word. The removed Word's Word Links and Hints move to the kept one, except those it already has " +
			"and any link between the two; Word Links on other Words that pointed at the removed Word now point at the kept one. " +
			"It can't be undone: ask the Owner first. Answers the kept Word as it is now.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{` +
			`"keep":{"type":"string","description":"The id of the Word that stays."},` +
			`"remove":{"type":"string","description":"The id of the Word merged into it and deleted."}},` +
			`"required":["keep","remove"]}`),
		Destructive: true,
		Handle:      s.mergeWordsTool,
	}, {
		Name: "japanese_change_group",
		Description: "Moves a Word to another stored Group, such as from gaming to jlpt_n2 once jisho tags it N2. " +
			"Ask the Owner first. Answers the Word as it is now.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{` +
			`"id":{"type":"string"},"group":{"type":"string","description":"A stored Group's key."}},"required":["id","group"]}`),
		Destructive: true,
		Handle:      s.changeGroupTool,
	}}
}

// deleteWordTool answers japanese_delete_word. It removes the Word Links to
// the Word first, so that a delete that fails halfway can be run again.
func (s *WordsService) deleteWordTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	word, err := s.storedWord(ctx, args.ID)
	if err != nil {
		return nil, err
	}
	if err := s.words.RemoveWordLinksTo(ctx, word.ID); err != nil {
		return nil, err
	}
	if err := s.words.Delete(ctx, word.ID); err != nil {
		return nil, err
	}
	return model.WordDeletedJSON{Deleted: word.ID}, nil
}

// mergeWordsTool answers japanese_merge_words. Its steps are separate
// writes; each can be run again, so a merge that fails halfway is finished
// by merging again.
func (s *WordsService) mergeWordsTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		Keep   string `json:"keep"`
		Remove string `json:"remove"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	if args.Keep == args.Remove {
		return nil, errors.New("keep and remove must be two different Words")
	}
	keep, err := s.storedWord(ctx, args.Keep)
	if err != nil {
		return nil, err
	}
	remove, err := s.storedWord(ctx, args.Remove)
	if err != nil {
		return nil, err
	}
	for _, side := range remove.WordLinks {
		if side.WordID == keep.ID {
			continue
		}
		if _, err := s.words.AddWordLinkSide(ctx, keep.ID, side); err != nil {
			return nil, err
		}
	}
	for _, hint := range remove.Hints {
		if _, err := s.words.AddHint(ctx, keep.ID, hint); err != nil {
			return nil, err
		}
	}
	if err := s.words.RepointWordLinks(ctx, remove.ID, keep.ID); err != nil {
		return nil, err
	}
	if err := s.words.Delete(ctx, remove.ID); err != nil {
		return nil, err
	}
	merged, err := s.words.GetOneByID(ctx, keep.ID)
	if err != nil {
		return nil, err
	}
	return s.wordJSON(ctx, merged)
}

// changeGroupTool answers japanese_change_group.
func (s *WordsService) changeGroupTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		ID    string `json:"id"`
		Group string `json:"group"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	if _, err := s.groups.GetOneByKey(ctx, args.Group); errors.Is(err, coreutils.ErrNotFound) {
		return nil, fmt.Errorf("%q isn't a stored Group", args.Group)
	} else if err != nil {
		return nil, err
	}
	err := s.words.SetGroup(ctx, args.ID, args.Group)
	if errors.Is(err, coreutils.ErrNotFound) {
		return nil, fmt.Errorf("there's no Word with id %s", args.ID)
	}
	if err != nil {
		return nil, err
	}
	word, err := s.words.GetOneByID(ctx, args.ID)
	if err != nil {
		return nil, err
	}
	return s.wordJSON(ctx, word)
}

// storedWord is the Word with id, or an error saying there's none.
func (s *WordsService) storedWord(ctx context.Context, id string) (model.Word, error) {
	word, err := s.words.GetOneByID(ctx, id)
	if errors.Is(err, coreutils.ErrNotFound) {
		return word, fmt.Errorf("there's no Word with id %q", id)
	}
	return word, err
}

// addHintTool answers japanese_add_hint.
func (s *WordsService) addHintTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		Word string `json:"word"`
		Text string `json:"text"`
		Note string `json:"note"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	text, err := checkSpelling(args.Text)
	if err != nil {
		return nil, errors.New("text is required, at most 100 characters")
	}
	note, err := checkNotes(args.Note)
	if err != nil {
		return nil, err
	}
	if _, err := s.words.GetOneByID(ctx, args.Word); errors.Is(err, coreutils.ErrNotFound) {
		return nil, fmt.Errorf("there's no Word with id %s", args.Word)
	} else if err != nil {
		return nil, err
	}
	stored, err := s.words.GetManyBySpellingOrReading(ctx, text)
	if err != nil {
		return nil, err
	}
	if len(stored) > 0 {
		return nil, fmt.Errorf("%s is a stored Word (id %s): link it with japanese_add_word_link instead", text, stored[0].ID)
	}
	written, err := s.words.AddHint(ctx, args.Word, model.Hint{Text: text, Note: note})
	if err != nil {
		return nil, err
	}
	return model.HintWrittenJSON{Written: written}, nil
}

// removeHintTool answers japanese_remove_hint with the Word as it is now.
func (s *WordsService) removeHintTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		Word string `json:"word"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	removed, err := s.words.RemoveHint(ctx, args.Word, strings.TrimSpace(args.Text))
	if err != nil {
		return nil, err
	}
	if !removed {
		return nil, fmt.Errorf("the Word with id %s has no Hint %q", args.Word, args.Text)
	}
	word, err := s.words.GetOneByID(ctx, args.Word)
	if err != nil {
		return nil, err
	}
	return s.wordJSON(ctx, word)
}

// updateWordLinkTool answers japanese_update_word_link.
func (s *WordsService) updateWordLinkTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var given map[string]json.RawMessage
	if err := json.Unmarshal(arguments, &given); err != nil {
		return nil, err
	}
	var args struct {
		From       string   `json:"from"`
		To         string   `json:"to"`
		Kind       string   `json:"kind"`
		Note       string   `json:"note"`
		NuanceTags []string `json:"nuanceTags"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	if args.From == "" || args.To == "" {
		return nil, errors.New("from and to are required")
	}
	var changes model.WordLinkChanges
	if _, ok := given["kind"]; ok {
		if !slices.Contains(wordLinkKinds, args.Kind) {
			return nil, fmt.Errorf("kind must be one of %s", strings.Join(wordLinkKinds, ", "))
		}
		changes.Kind = &args.Kind
	}
	if _, ok := given["note"]; ok {
		note, err := checkNotes(args.Note)
		if err != nil {
			return nil, err
		}
		changes.Note = &note
	}
	if _, ok := given["nuanceTags"]; ok {
		tags, err := checkTags(args.NuanceTags)
		if err != nil {
			return nil, err
		}
		changes.NuanceTags = &tags
	}
	if changes == (model.WordLinkChanges{}) {
		return nil, errors.New("give at least one of kind, note and nuanceTags to change")
	}
	err := s.words.UpdateWordLinkSide(ctx, args.From, args.To, changes)
	if errors.Is(err, coreutils.ErrNotFound) {
		return nil, errors.New("from has no Word Link to to: add one with japanese_add_word_link")
	}
	if err != nil {
		return nil, err
	}
	word, err := s.words.GetOneByID(ctx, args.From)
	if err != nil {
		return nil, err
	}
	return s.wordJSON(ctx, word)
}

// removeWordLinkTool answers japanese_remove_word_link.
func (s *WordsService) removeWordLinkTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	if args.From == "" || args.To == "" {
		return nil, errors.New("from and to are required")
	}
	var removed model.WordLinkRemovedJSON
	var err error
	if removed.FromSideRemoved, err = s.words.RemoveWordLinkSide(ctx, args.From, args.To); err != nil {
		return nil, err
	}
	if removed.BackSideRemoved, err = s.words.RemoveWordLinkSide(ctx, args.To, args.From); err != nil {
		return nil, err
	}
	if !removed.FromSideRemoved && !removed.BackSideRemoved {
		return nil, errors.New("there's no Word Link between them")
	}
	return removed, nil
}

// wordLinkKinds are the kinds a Word Link can be.
var wordLinkKinds = []string{"synonym", "antonym", "related"}

// oppositeNuanceTags are the Nuance Tags that swap on a Word Link's way back.
var oppositeNuanceTags = map[string]string{
	"formal": "casual", "casual": "formal",
	"spoken": "written", "written": "spoken",
	"expression": "part", "part": "expression",
}

// addWordLinkTool answers japanese_add_word_link.
func (s *WordsService) addWordLinkTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		From       string   `json:"from"`
		To         string   `json:"to"`
		Kind       string   `json:"kind"`
		Note       string   `json:"note"`
		NuanceTags []string `json:"nuanceTags"`
		NoteBack   *string  `json:"noteBack"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	switch {
	case args.From == "" || args.To == "":
		return nil, errors.New("from and to are required")
	case args.From == args.To:
		return nil, errors.New("a Word can't be linked to itself")
	case !slices.Contains(wordLinkKinds, args.Kind):
		return nil, fmt.Errorf("kind must be one of %s", strings.Join(wordLinkKinds, ", "))
	}
	note, err := checkNotes(args.Note)
	if err != nil {
		return nil, err
	}
	noteBack := note
	if args.NoteBack != nil {
		if noteBack, err = checkNotes(*args.NoteBack); err != nil {
			return nil, err
		}
	}
	nuanceTags, err := checkTags(args.NuanceTags)
	if err != nil {
		return nil, err
	}
	stored, err := s.words.GetManyByIDs(ctx, []string{args.From, args.To})
	if err != nil {
		return nil, err
	}
	if len(stored) != 2 {
		return nil, errors.New("from and to must both be stored Words' ids")
	}
	backTags := make([]string, len(nuanceTags))
	for i, tag := range nuanceTags {
		backTags[i] = tag
		if opposite, ok := oppositeNuanceTags[tag]; ok {
			backTags[i] = opposite
		}
	}
	var written model.WordLinkWrittenJSON
	if written.FromSideWritten, err = s.words.AddWordLinkSide(ctx, args.From,
		model.WordLink{WordID: args.To, Kind: args.Kind, Note: note, NuanceTags: nuanceTags}); err != nil {
		return nil, err
	}
	if written.BackSideWritten, err = s.words.AddWordLinkSide(ctx, args.To,
		model.WordLink{WordID: args.From, Kind: args.Kind, Note: noteBack, NuanceTags: backTags}); err != nil {
		return nil, err
	}
	return written, nil
}

// wordFields are a Word's own fields as a Tool is sent them.
type wordFields struct {
	Word      string           `json:"word"`
	Hiragana  string           `json:"hiragana"`
	Spanish   string           `json:"spanish"`
	Type      map[string]int   `json:"type"`
	Group     string           `json:"group"`
	Tags      []string         `json:"tags"`
	Notes     string           `json:"notes"`
	Sentences []sentenceFields `json:"sentences"`
}

// sentenceFields is an example sentence as a Tool is sent it.
type sentenceFields struct {
	Sentence    string `json:"sentence"`
	Translation string `json:"translation"`
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
	var word model.Word
	var err error
	if word.Word, err = checkSpelling(fields.Word); err != nil {
		return word, err
	}
	if word.Hiragana, err = checkReading(fields.Hiragana); err != nil {
		return word, err
	}
	if word.Spanish, err = checkSpanish(fields.Spanish); err != nil {
		return word, err
	}
	if word.Type, err = checkType(fields.Type); err != nil {
		return word, err
	}
	if word.Tags, err = checkTags(fields.Tags); err != nil {
		return word, err
	}
	if word.Notes, err = checkNotes(fields.Notes); err != nil {
		return word, err
	}
	if word.Sentences, err = checkSentences(fields.Sentences); err != nil {
		return word, err
	}
	word.Group = fields.Group
	if _, err := s.groups.GetOneByKey(ctx, word.Group); errors.Is(err, coreutils.ErrNotFound) {
		return word, fmt.Errorf("%q isn't a stored Group", word.Group)
	} else if err != nil {
		return word, err
	}
	return word, nil
}

// The checks below each check one field as sent, and return it as stored.

func checkSpelling(spelling string) (string, error) {
	spelling = strings.TrimSpace(spelling)
	if spelling == "" || utf8.RuneCountInString(spelling) > maxSpellingLength {
		return "", fmt.Errorf("word is required, at most %d characters", maxSpellingLength)
	}
	return spelling, nil
}

func checkReading(reading string) (string, error) {
	reading = strings.TrimSpace(reading)
	if utf8.RuneCountInString(reading) > maxSpellingLength {
		return "", fmt.Errorf("hiragana must be at most %d characters", maxSpellingLength)
	}
	return reading, nil
}

func checkSpanish(spanish string) (string, error) {
	spanish = strings.TrimSpace(spanish)
	if spanish == "" || utf8.RuneCountInString(spanish) > maxSpanishLength {
		return "", fmt.Errorf("spanish is required, at most %d characters", maxSpanishLength)
	}
	return spanish, nil
}

func checkNotes(notes string) (string, error) {
	notes = strings.TrimSpace(notes)
	if utf8.RuneCountInString(notes) > maxNotesLength {
		return "", fmt.Errorf("notes must be at most %d characters", maxNotesLength)
	}
	return notes, nil
}

func checkType(partsOfSpeech map[string]int) (map[string]int, error) {
	if len(partsOfSpeech) == 0 {
		return nil, fmt.Errorf("type needs at least one of %s", strings.Join(wordTypes, ", "))
	}
	for kind, form := range partsOfSpeech {
		if !slices.Contains(wordTypes, kind) {
			return nil, fmt.Errorf("type %q isn't one of %s", kind, strings.Join(wordTypes, ", "))
		}
		if form < 1 || form > 8 {
			return nil, fmt.Errorf("type %s is %d; the form must be 1 to 8", kind, form)
		}
	}
	return partsOfSpeech, nil
}

func checkTags(tags []string) ([]string, error) {
	tags = utils.OrEmptyList(tags)
	if len(tags) > maxTags {
		return nil, fmt.Errorf("at most %d Word Tags", maxTags)
	}
	for _, tag := range tags {
		if !snakeCase.MatchString(tag) || len(tag) > maxTagLength {
			return nil, fmt.Errorf("Word Tag %q must be snake_case, such as usually_kana", tag)
		}
	}
	return tags, nil
}

func checkSentences(sent []sentenceFields) ([]model.Sentence, error) {
	if len(sent) > maxSentences {
		return nil, fmt.Errorf("at most %d sentences", maxSentences)
	}
	sentences := []model.Sentence{}
	for _, s := range sent {
		s.Sentence, s.Translation = strings.TrimSpace(s.Sentence), strings.TrimSpace(s.Translation)
		if s.Sentence == "" || s.Translation == "" ||
			utf8.RuneCountInString(s.Sentence) > maxSentenceLength || utf8.RuneCountInString(s.Translation) > maxSentenceLength {
			return nil, fmt.Errorf("each sentence needs its translation, each at most %d characters", maxSentenceLength)
		}
		sentences = append(sentences, model.Sentence(s))
	}
	return sentences, nil
}

// updatable are the fields japanese_update_word changes.
var updatable = []string{"word", "hiragana", "spanish", "type", "tags", "notes", "sentences"}

// updateWordTool answers japanese_update_word with the Word as it is now.
func (s *WordsService) updateWordTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var given map[string]json.RawMessage
	if err := json.Unmarshal(arguments, &given); err != nil {
		return nil, err
	}
	var id string
	if err := json.Unmarshal(given["id"], &id); err != nil || id == "" {
		return nil, errors.New("id is required")
	}
	delete(given, "id")
	if _, ok := given["group"]; ok {
		return nil, errors.New("a Word's Group is changed with japanese_change_group")
	}
	for field := range given {
		if !slices.Contains(updatable, field) {
			return nil, fmt.Errorf("%q isn't a field japanese_update_word changes: only %s; Word Links and Hints have their own Tools",
				field, strings.Join(updatable, ", "))
		}
	}
	if len(given) == 0 {
		return nil, fmt.Errorf("give at least one of %s to change", strings.Join(updatable, ", "))
	}
	var fields wordFields
	if err := json.Unmarshal(arguments, &fields); err != nil {
		return nil, err
	}
	changes, err := checkChanges(given, fields)
	if err != nil {
		return nil, err
	}
	err = s.words.Update(ctx, id, changes)
	if errors.Is(err, coreutils.ErrNotFound) {
		return nil, fmt.Errorf("there's no Word with id %s", id)
	}
	if errors.Is(err, model.ErrWordExists) {
		return nil, errors.New("another Word already has that spelling and reading: merge them with japanese_merge_words instead")
	}
	if err != nil {
		return nil, err
	}
	word, err := s.words.GetOneByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.wordJSON(ctx, word)
}

// checkChanges checks only the fields given and returns them as changes.
func checkChanges(given map[string]json.RawMessage, fields wordFields) (model.WordChanges, error) {
	var changes model.WordChanges
	sent := func(field string) bool { _, ok := given[field]; return ok }
	check := func(field string, value string, check func(string) (string, error), into **string) error {
		if !sent(field) {
			return nil
		}
		checked, err := check(value)
		*into = &checked
		return err
	}
	if err := check("word", fields.Word, checkSpelling, &changes.Word); err != nil {
		return changes, err
	}
	if err := check("hiragana", fields.Hiragana, checkReading, &changes.Hiragana); err != nil {
		return changes, err
	}
	if err := check("spanish", fields.Spanish, checkSpanish, &changes.Spanish); err != nil {
		return changes, err
	}
	if err := check("notes", fields.Notes, checkNotes, &changes.Notes); err != nil {
		return changes, err
	}
	var err error
	if sent("type") {
		if changes.Type, err = checkType(fields.Type); err != nil {
			return changes, err
		}
	}
	if sent("tags") {
		tags, err := checkTags(fields.Tags)
		if err != nil {
			return changes, err
		}
		changes.Tags = &tags
	}
	if sent("sentences") {
		sentences, err := checkSentences(fields.Sentences)
		if err != nil {
			return changes, err
		}
		changes.Sentences = &sentences
	}
	return changes, nil
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
