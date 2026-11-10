package model

import (
	"maps"
	"slices"
)

// AnkiWord is a Word as the JLPT deck file exports it, read as benkyo's
// export read it. It is its own shape, not a Word: the deck depends on details
// of how benkyo stored Words that nothing else shows.
type AnkiWord struct {
	ID string
	// Word is the kanji spelling, or the kana if there's none.
	Word string
	// Hiragana is the reading; "" when Word is already kana.
	Hiragana string
	Spanish  string
	// PartsOfSpeech are in their stored order, which is the order the deck
	// lists them in.
	PartsOfSpeech []PartOfSpeech
	// Tags are its Word Tags, snake_case, in stored order.
	Tags []string
	// Notes is "" when there are none.
	Notes string
	// Sentence and Translation are the first stored sentence and the first
	// stored translation among its example sentences, as benkyo took them, or "".
	Sentence    string
	Translation string
	// Related are its sides of its Word Links and its Hints, together in the
	// order they are stored.
	Related []AnkiRelated
}

// PartOfSpeech is one of a Word's parts of speech as benkyo stored it.
type PartOfSpeech struct {
	// Key is "noun", "suru", "no_adj", "na_adj", "i_adj", "adv", "verb",
	// "adj_noun", "adv_noun" or "counter", or anything else benkyo stored.
	Key  string
	Form Form
}

// Form is which form of a part of speech a Word has.
type Form int

// The forms benkyo stored. Any other stored value is no form at all.
const (
	NoForm    Form = 0
	PlainForm Form = 1
	// ToForm is the part of speech with と, such as 副と.
	ToForm Form = 4
	// TekiForm is the part of speech with 的, such as な形的.
	TekiForm Form = 5
)

// AnkiRelated is one of a Word's sides of its Word Links, or one of its
// Hints, as benkyo stored them. benkyo stored a Hint with a kind and Nuance
// Tags like a Word Link's, and the deck shows them.
type AnkiRelated struct {
	// Other is the other Word of a Word Link, or nil for a Hint or for a Word
	// Link to a Word that is no longer stored.
	Other *AnkiOtherWord
	// Text is a Hint's text, or "".
	Text string
	// Kind is "synonym", "antonym" or "related" as stored, and "related" when
	// none is stored.
	Kind string
	// Note is "" when there's none.
	Note       string
	NuanceTags []string
}

// AnkiOtherWord is the other Word of a Word Link, as the deck shows it.
type AnkiOtherWord struct {
	// Spelling is its kanji spelling, or its kana if it has none, as stored:
	// never turned to kana for the usually_kana Word Tag.
	Spelling string
	Group    string
	Tags     []string
}

// jlptLevels are the JLPT Groups' keys and the level each is for.
var jlptLevels = map[string]string{"jlpt_n3": "N3", "jlpt_n2": "N2", "jlpt_n1": "N1"}

// JLPTGroups are the keys of the JLPT Groups.
func JLPTGroups() []string {
	return slices.Sorted(maps.Keys(jlptLevels))
}

// JLPTLevel is the JLPT level the Word is studied for, such as "N2", or "":
// its JLPT Group's, or N1 for a Word with the jlpt_n1 Word Tag in another
// Group, as an N1 Word found while doing something else is filed.
func (w AnkiOtherWord) JLPTLevel() string {
	if level, ok := jlptLevels[w.Group]; ok {
		return level
	}
	if slices.Contains(w.Tags, "jlpt_n1") {
		return "N1"
	}
	return ""
}
