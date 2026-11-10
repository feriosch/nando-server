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
	// GetManyBySpelling returns the stored Kanji in a Word's spelling, in the
	// order they first appear there, filled in as by GetManyBySearch.
	GetManyBySpelling(ctx context.Context, spelling string) ([]Kanji, error)
	// GetOneByID returns the Kanji with this ID, or Core's utils.ErrNotFound.
	GetOneByID(ctx context.Context, id string) (Kanji, error)
	// GetManyByKeywords returns the stored Kanji whose Spanish is one of keywords,
	// in no particular order, filled in as by GetManyBySearch.
	GetManyByKeywords(ctx context.Context, keywords []string) ([]Kanji, error)
}

// The JSON shapes below are what the API answers with.

// KanjiRowJSON is a Kanji as a list shows it.
type KanjiRowJSON struct {
	ID      string `json:"id"`
	Kanji   string `json:"kanji"`
	Spanish string `json:"spanish"`
}

// KanjiJSON is a Kanji as its page shows it.
type KanjiJSON struct {
	ID         string          `json:"id"`
	Kanji      string          `json:"kanji"`
	Spanish    string          `json:"spanish"`
	On         *string         `json:"on"`
	Kun        *string         `json:"kun"`
	V1         int             `json:"v1"`
	V2         *int            `json:"v2"`
	Story      *string         `json:"story"`
	Components []ComponentJSON `json:"components"`
	Words      []WordRowJSON   `json:"words"`
}

// ComponentJSON is a Component: its keyword, and the Kanji it is, or null
// for an irregular shape.
type ComponentJSON struct {
	Name  string        `json:"name"`
	Kanji *KanjiRowJSON `json:"kanji"`
}
