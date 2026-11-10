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

// ToWordRowJSONs returns words as a list shows them.
func ToWordRowJSONs(words []model.Word) []model.WordRowJSON {
	list := make([]model.WordRowJSON, len(words))
	for i, w := range words {
		list[i] = model.WordRowJSON{
			ID:       w.ID,
			Word:     w.Word,
			Hiragana: OrNull(w.Hiragana),
			Spanish:  w.Spanish,
			Group:    w.Group,
		}
	}
	return list
}

// ToKanjiRowJSONs returns kanji as a list shows them.
func ToKanjiRowJSONs(kanji []model.Kanji) []model.KanjiRowJSON {
	list := make([]model.KanjiRowJSON, len(kanji))
	for i, k := range kanji {
		list[i] = model.KanjiRowJSON{ID: k.ID, Kanji: k.Kanji, Spanish: k.Spanish}
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
