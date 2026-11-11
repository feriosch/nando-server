package model

// The JSON shapes below are what japanese_lookup_jisho answers: jisho.org's
// search results, trimmed to what deciding how to store a Word needs. They
// show no stored collection.

// JishoJSON is jisho.org's entries for a keyword, best match first.
type JishoJSON struct {
	Entries []JishoEntryJSON `json:"entries"`
}

// JishoEntryJSON is one jisho.org entry.
type JishoEntryJSON struct {
	Slug     string `json:"slug"`
	IsCommon bool   `json:"isCommon"`
	// JLPT is jisho's JLPT levels, such as ["jlpt-n2"]; [] if it has none.
	JLPT []string `json:"jlpt"`
	// Japanese is each spelling with its reading, the usual one first.
	Japanese []JishoJapaneseJSON `json:"japanese"`
	Senses   []JishoSenseJSON    `json:"senses"`
}

// JishoJapaneseJSON is a spelling and its reading. Word is "" when the entry
// has only kana.
type JishoJapaneseJSON struct {
	Word    string `json:"word"`
	Reading string `json:"reading"`
}

// JishoSenseJSON is one meaning of an entry.
type JishoSenseJSON struct {
	English       []string `json:"english"`
	PartsOfSpeech []string `json:"partsOfSpeech"`
	// Tags are jisho's notes on the sense, such as "Usually written using
	// kana alone" or "Archaic".
	Tags     []string `json:"tags"`
	SeeAlso  []string `json:"seeAlso"`
	Antonyms []string `json:"antonyms"`
}
