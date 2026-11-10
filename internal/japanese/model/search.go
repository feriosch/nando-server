package model

// SearchJSON is what a search answers: Words, then Kanji, as rows.
type SearchJSON struct {
	Words []WordRowJSON  `json:"words"`
	Kanji []KanjiRowJSON `json:"kanji"`
}
