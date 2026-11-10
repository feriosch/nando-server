// Package utils holds helpers the Japanese Module's layers share.
package utils

import "github.com/feriosch/nando-server/internal/japanese/model"

// OrNull is s, or nil if s is "": an optional string as the API writes it.
func OrNull(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// WordRowJSON is a Word as a list shows it: a search result, or a Word
// spelled with a Kanji.
type WordRowJSON struct {
	ID       string  `json:"id"`
	Word     string  `json:"word"`
	Hiragana *string `json:"hiragana"`
	Spanish  string  `json:"spanish"`
	Group    string  `json:"group"`
}

// ToWordRowJSONs returns words as a list shows them.
func ToWordRowJSONs(words []model.Word) []WordRowJSON {
	list := make([]WordRowJSON, len(words))
	for i, w := range words {
		list[i] = WordRowJSON{
			ID:       w.ID,
			Word:     w.Word,
			Hiragana: OrNull(w.Hiragana),
			Spanish:  w.Spanish,
			Group:    w.Group,
		}
	}
	return list
}

// KanjiRowJSON is a Kanji as a list shows it.
type KanjiRowJSON struct {
	ID      string `json:"id"`
	Kanji   string `json:"kanji"`
	Spanish string `json:"spanish"`
}

// ToKanjiRowJSONs returns kanji as a list shows them.
func ToKanjiRowJSONs(kanji []model.Kanji) []KanjiRowJSON {
	list := make([]KanjiRowJSON, len(kanji))
	for i, k := range kanji {
		list[i] = KanjiRowJSON{ID: k.ID, Kanji: k.Kanji, Spanish: k.Spanish}
	}
	return list
}

// OrEmptyList is list, or [] if it is nil, so the API never writes null for
// a list.
func OrEmptyList[T any](list []T) []T {
	if list == nil {
		return []T{}
	}
	return list
}
