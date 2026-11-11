package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	coremodel "github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/japanese/model"
	"github.com/feriosch/nando-server/internal/japanese/utils"
)

// LookupService looks Words and Kanji up outside Nando, on jisho.org and
// kanjiapi.dev, for Claude to decide how to store them. It serves no
// collection, so it is named for what it does; it reads the stored Kanji.
type LookupService struct {
	kanji       model.KanjiRepository
	jishoURL    string
	kanjiAPIURL string
	client      *http.Client

	// kanjiAPIPace is how far apart kanjiapi.dev is asked, across every
	// call on this instance; next is when it may be asked again.
	kanjiAPIPace time.Duration
	paceMu       sync.Mutex
	next         time.Time
}

// kanjiAPIAttempts is how many times kanjiapi.dev is asked about one Kanji
// before its answer counts as unknown.
const kanjiAPIAttempts = 3

// NewLookup returns the LookupService asking jisho.org's API at jishoURL and
// kanjiapi.dev's at kanjiAPIURL, at most once every kanjiAPIPace.
func NewLookup(kanji model.KanjiRepository, jishoURL, kanjiAPIURL string, kanjiAPIPace time.Duration) *LookupService {
	return &LookupService{
		kanji:        kanji,
		jishoURL:     strings.TrimSuffix(jishoURL, "/"),
		kanjiAPIURL:  strings.TrimSuffix(kanjiAPIURL, "/"),
		client:       &http.Client{Timeout: 15 * time.Second},
		kanjiAPIPace: kanjiAPIPace,
	}
}

// Tools are the LookupService's Tools for Connectors.
func (s *LookupService) Tools() []coremodel.Tool {
	return []coremodel.Tool{{
		Name: "japanese_lookup_jisho",
		Description: "Looks a word or phrase up on jisho.org, as the add-word rules need: each entry's slug, spellings with readings " +
			"(the usual one first), JLPT level (such as jlpt-n2, which decides the Group), whether it is common, and its senses with English meanings, " +
			"parts of speech, jisho's tags (such as \"Usually written using kana alone\"), see also and antonyms. " +
			"A phrase jisho doesn't list is a collocation, not an Expression. Answers {\"entries\": [...]}, best match first.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{` +
			`"keyword":{"type":"string","description":"A spelling, a reading, or an English word, as jisho.org's search takes it."}},` +
			`"required":["keyword"]}`),
		ReadOnly: true,
		Handle:   s.lookupJishoTool,
	}, {
		Name: "japanese_check_joyo",
		Description: "Checks each Kanji in a text, as the jōyō check before any write needs: whether it is a stored Kanji, " +
			"and whether it is jōyō by kanjiapi.dev's grade (1 to 8 is jōyō; none, 9 or 10 isn't). " +
			"A Word with a Kanji that isn't jōyō gets the joyogai Word Tag. joyo is null when kanjiapi.dev couldn't tell " +
			"(it rate-limits): check again later, and never take null as not jōyō. kanjiapi.dev is asked about one Kanji a second. " +
			"Answers {\"kanji\": [{\"kanji\", \"stored\", \"joyo\", \"grade\"}]}, each Kanji once.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{` +
			`"text":{"type":"string","description":"A Word's spelling, or any text with Kanji in it."}},"required":["text"]}`),
		ReadOnly: true,
		Handle:   s.checkJoyoTool,
	}}
}

// checkJoyoTool answers japanese_check_joyo.
func (s *LookupService) checkJoyoTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	var characters []string
	for _, r := range args.Text {
		if isKanji(r) && !slices.Contains(characters, string(r)) {
			characters = append(characters, string(r))
		}
	}
	if len(characters) == 0 {
		return nil, fmt.Errorf("%q has no Kanji", args.Text)
	}
	stored, err := s.kanji.GetManyBySpelling(ctx, args.Text)
	if err != nil {
		return nil, err
	}
	answer := model.JoyoJSON{}
	for _, character := range characters {
		grade, known, err := s.kanjiGrade(ctx, character)
		if err != nil {
			return nil, err
		}
		j := model.KanjiJoyoJSON{
			Kanji:  character,
			Stored: slices.ContainsFunc(stored, func(k model.Kanji) bool { return k.Kanji == character }),
			Grade:  grade,
		}
		if known {
			joyo := grade != nil && *grade >= 1 && *grade <= 8
			j.Joyo = &joyo
		}
		answer.Kanji = append(answer.Kanji, j)
	}
	return answer, nil
}

// kanjiGrade asks kanjiapi.dev for character's grade. known is false if it
// couldn't tell after kanjiAPIAttempts: a reply with no grade key, another
// status, or no reply, as when it rate-limits. grade is nil when it has
// none, or kanjiapi.dev doesn't know the character (404), which isn't jōyō.
func (s *LookupService) kanjiGrade(ctx context.Context, character string) (grade *int, known bool, err error) {
	address := s.kanjiAPIURL + "/v1/kanji/" + url.PathEscape(character)
	for range kanjiAPIAttempts {
		if err := s.waitForKanjiAPI(ctx); err != nil {
			return nil, false, err
		}
		status, body, err := s.fetch(ctx, address)
		switch {
		case err != nil:
			continue
		case status == http.StatusNotFound:
			return nil, true, nil
		case status != http.StatusOK:
			continue
		}
		var reply map[string]json.RawMessage
		if json.Unmarshal(body, &reply) != nil {
			continue
		}
		raw, ok := reply["grade"]
		if !ok {
			continue // rate limited: kanjiapi.dev answers without a grade
		}
		if err := json.Unmarshal(raw, &grade); err != nil {
			continue
		}
		return grade, true, nil
	}
	return nil, false, nil
}

// waitForKanjiAPI waits until kanjiapi.dev may be asked again, and books the
// next turn, so this instance asks it at most once every kanjiAPIPace.
func (s *LookupService) waitForKanjiAPI(ctx context.Context) error {
	s.paceMu.Lock()
	defer s.paceMu.Unlock()
	if wait := time.Until(s.next); wait > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
	s.next = time.Now().Add(s.kanjiAPIPace)
	return nil
}

// fetch GETs address and returns its status and body.
func (s *LookupService) fetch(ctx context.Context, address string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return 0, nil, err
	}
	res, err := s.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	return res.StatusCode, body, err
}

// lookupJishoTool answers japanese_lookup_jisho.
func (s *LookupService) lookupJishoTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		Keyword string `json:"keyword"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	keyword := strings.TrimSpace(args.Keyword)
	if keyword == "" {
		return nil, errors.New("keyword is required")
	}
	var found struct {
		Data []struct {
			Slug     string   `json:"slug"`
			IsCommon bool     `json:"is_common"`
			JLPT     []string `json:"jlpt"`
			Japanese []struct {
				Word    string `json:"word"`
				Reading string `json:"reading"`
			} `json:"japanese"`
			Senses []struct {
				English       []string `json:"english_definitions"`
				PartsOfSpeech []string `json:"parts_of_speech"`
				Tags          []string `json:"tags"`
				SeeAlso       []string `json:"see_also"`
				Antonyms      []string `json:"antonyms"`
			} `json:"senses"`
		} `json:"data"`
	}
	if err := s.get(ctx, s.jishoURL+"/api/v1/search/words?"+url.Values{"keyword": {keyword}}.Encode(), &found); err != nil {
		return nil, err
	}
	answer := model.JishoJSON{Entries: []model.JishoEntryJSON{}}
	for _, d := range found.Data {
		entry := model.JishoEntryJSON{Slug: d.Slug, IsCommon: d.IsCommon, JLPT: utils.OrEmptyList(d.JLPT)}
		for _, j := range d.Japanese {
			entry.Japanese = append(entry.Japanese, model.JishoJapaneseJSON(j))
		}
		for _, sense := range d.Senses {
			entry.Senses = append(entry.Senses, model.JishoSenseJSON{
				English:       utils.OrEmptyList(sense.English),
				PartsOfSpeech: utils.OrEmptyList(sense.PartsOfSpeech),
				Tags:          utils.OrEmptyList(sense.Tags),
				SeeAlso:       utils.OrEmptyList(sense.SeeAlso),
				Antonyms:      utils.OrEmptyList(sense.Antonyms),
			})
		}
		answer.Entries = append(answer.Entries, entry)
	}
	return answer, nil
}

// get GETs address and decodes its JSON into v, failing unless it answers 200.
func (s *LookupService) get(ctx context.Context, address string, v any) error {
	host := address
	if u, err := url.Parse(address); err == nil {
		host = u.Host
	}
	status, body, err := s.fetch(ctx, address)
	if err != nil {
		return fmt.Errorf("asking %s: %w", host, err)
	}
	if status != http.StatusOK {
		return fmt.Errorf("%s answered %d %s; try again later", host, status, http.StatusText(status))
	}
	return json.Unmarshal(body, v)
}
