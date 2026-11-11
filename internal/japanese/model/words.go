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
	// WordLinks are its sides of its Word Links.
	WordLinks []WordLink
	Hints     []Hint
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
	// GetManyBySearch returns up to limit Words that match q, best first: exact
	// matches, then shorter ones, then longer ones. Only what a search result
	// shows is filled in: ID, Word, Hiragana, Spanish and Group.
	GetManyBySearch(ctx context.Context, q string, limit int) ([]Word, error)
	// GetOneByID returns the Word with this ID, or Core's utils.ErrNotFound.
	GetOneByID(ctx context.Context, id string) (Word, error)
	// GetManyBySpellingOrReading returns every Word spelled text or read as
	// text, oldest first, with all its fields.
	GetManyBySpellingOrReading(ctx context.Context, text string) ([]Word, error)
	// GetManyByIDs returns the Words with these IDs that are stored, in no
	// particular order, filled in as by GetManyBySearch.
	GetManyByIDs(ctx context.Context, ids []string) ([]Word, error)
	// GetManyByKanji returns up to limit Words whose spelling contains
	// kanji (a Kanji's character), shorter ones first, filled in as by GetManyBySearch.
	GetManyByKanji(ctx context.Context, kanji string, limit int) ([]Word, error)
	// GetManyForJLPTDeck returns, oldest first, every Word studied for the JLPT,
	// as the JLPT deck file exports it: those in the JLPT Groups, and any other
	// Word with the jlpt_n1 Word Tag.
	GetManyForJLPTDeck(ctx context.Context) ([]AnkiWord, error)
}

// The JSON shapes below are what the API answers with.

// WordRowJSON is a Word as a list shows it: a search result, or a Word
// spelled with a Kanji.
type WordRowJSON struct {
	ID       string  `json:"id"`
	Word     string  `json:"word"`
	Hiragana *string `json:"hiragana"`
	Spanish  string  `json:"spanish"`
	Group    string  `json:"group"`
}

// WordsJSON is the Words a Tool found, such as every Word with a spelling
// or reading.
type WordsJSON struct {
	Words []WordJSON `json:"words"`
}

// WordJSON is a Word as its page shows it.
type WordJSON struct {
	ID        string         `json:"id"`
	Word      string         `json:"word"`
	Hiragana  *string        `json:"hiragana"`
	Spanish   string         `json:"spanish"`
	Type      map[string]int `json:"type"`
	Group     GroupJSON      `json:"group"`
	Tags      []string       `json:"tags"`
	Notes     *string        `json:"notes"`
	Sentences []SentenceJSON `json:"sentences"`
	WordLinks []WordLinkJSON `json:"wordLinks"`
	Hints     []HintJSON     `json:"hints"`
	Kanji     []KanjiRowJSON `json:"kanji"`
}

// SentenceJSON is an example sentence as the Word page shows it.
type SentenceJSON struct {
	Sentence    string `json:"sentence"`
	Translation string `json:"translation"`
}

// WordLinkJSON is a Word's side of a Word Link as its page shows it, with
// the other Word as a row.
type WordLinkJSON struct {
	Kind       string      `json:"kind"`
	Note       *string     `json:"note"`
	NuanceTags []string    `json:"nuanceTags"`
	Word       WordRowJSON `json:"word"`
}

// HintJSON is a Hint as the Word page shows it.
type HintJSON struct {
	Text string  `json:"text"`
	Note *string `json:"note"`
}
