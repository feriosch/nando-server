package model

import "context"

// Kanji is one character the Owner studies.
type Kanji struct {
	ID string
	// Kanji is the character.
	Kanji string
	// Spanish is the meaning, in Spanish, and the Kanji's Heisig keyword.
	Spanish string
	// On and Kun are its readings; "" when not stored.
	On  string
	Kun string
	// V1 is its number in Heisig's Remembering the Kanji volume 1, and V2 in
	// volume 2, or 0 if it isn't in volume 2.
	V1 int
	V2 int
	// Story is "" when there's none.
	Story string
	// Components are the keywords of its Components: a Kanji's Spanish, or
	// an irregular shape's name.
	Components []string
}

// KanjiRepository reads the kanji collection.
type KanjiRepository interface {
	// GetManyBySearch returns up to limit Kanji that match q, best first. Only what a
	// search result shows is filled in: ID, Kanji and Spanish.
	GetManyBySearch(ctx context.Context, q string, limit int) ([]Kanji, error)
	// GetManyByCharactersIn returns the stored Kanji whose character is in text, in the
	// order they first appear there, filled in as by GetManyBySearch.
	GetManyByCharactersIn(ctx context.Context, text string) ([]Kanji, error)
	// GetOneByID returns the Kanji with this ID, or Core's utils.ErrNotFound.
	GetOneByID(ctx context.Context, id string) (Kanji, error)
	// GetManyByKeywords returns the stored Kanji whose Spanish is one of keywords,
	// in no particular order, filled in as by GetManyBySearch.
	GetManyByKeywords(ctx context.Context, keywords []string) ([]Kanji, error)
}
