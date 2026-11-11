package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	coremodel "github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/japanese/model"
	"github.com/feriosch/nando-server/internal/japanese/utils"
)

// LookupService looks Words up outside Nando, on jisho.org, for Claude to
// decide how to store them. It serves no collection, so it is named for
// what it does.
type LookupService struct {
	jishoURL string
	client   *http.Client
}

// NewLookup returns the LookupService asking jisho.org's API at jishoURL.
func NewLookup(jishoURL string) *LookupService {
	return &LookupService{jishoURL: strings.TrimSuffix(jishoURL, "/"), client: &http.Client{Timeout: 15 * time.Second}}
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
	}}
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return err
	}
	res, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("asking %s: %w", req.URL.Host, err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("%s answered %s; try again later", req.URL.Host, res.Status)
	}
	return json.NewDecoder(res.Body).Decode(v)
}
