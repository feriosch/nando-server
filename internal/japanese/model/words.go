package model

import "context"

// Word is a Japanese word or Expression the Owner is learning.
type Word struct {
	ID string
	// Word is the kanji spelling, or the kana if there's none.
	Word string
	// Hiragana is the reading; "" when Word is already kana.
	Hiragana string
	// Spanish is the meaning, in Spanish.
	Spanish string
	// Group is the key of the Group the Word is filed in, such as "jlpt_n2".
	Group string
	// Type is the Word's parts of speech as benkyo stored them, such as
	// {"noun": 1, "suru": 1}.
	Type map[string]int
	// Tags are its Word Tags, snake_case.
	Tags []string
	// Notes is "" when there are none.
	Notes     string
	Sentences []Sentence
	// Links are its sides of its Word Links.
	Links []WordLink
	Hints []Hint
}

// WordLink is a Word's side of a Word Link to another Word.
type WordLink struct {
	// WordID is the other Word's ID.
	WordID string
	// Kind is "synonym", "antonym" or "related".
	Kind string
	// Note is about the other Word; "" when there's none.
	Note string
	// NuanceTags say how the other Word differs from this one, snake_case.
	NuanceTags []string
}

// Hint is a one-way, text-only pointer from a Word to an everyday word that
// isn't stored as a Word.
type Hint struct {
	Text string
	// Note is "" when there's none.
	Note string
}

// Sentence is an example sentence using a Word, with its Spanish translation.
type Sentence struct {
	Sentence    string
	Translation string
}

// WordsRepository reads the words collection.
type WordsRepository interface {
	// Search returns up to limit Words that match q, best first: exact
	// matches, then shorter ones, then longer ones. Only what a search result
	// shows is filled in: ID, Word, Hiragana, Spanish and Group.
	Search(ctx context.Context, q string, limit int) ([]Word, error)
	// Find returns the Word with this ID, or Core's utils.ErrNotFound.
	Find(ctx context.Context, id string) (Word, error)
	// FindMany returns the Words with these IDs that are stored, in no
	// particular order, filled in as by Search.
	FindMany(ctx context.Context, ids []string) ([]Word, error)
	// SpelledWith returns up to limit Words whose spelling contains
	// character, shorter ones first, filled in as by Search.
	SpelledWith(ctx context.Context, character string, limit int) ([]Word, error)
}
