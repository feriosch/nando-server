package utils

import (
	"regexp"
	"strings"
	"unicode"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// letters lists, for each letter a search folds, the letters it stands for,
// so that searching corazon finds corazón and the other way round.
var letters = map[rune]string{
	'a': "aáàâäã",
	'e': "eéèêë",
	'i': "iíìîï",
	'o': "oóòôöõ",
	'u': "uúùûü",
	'n': "nñ",
	'c': "cç",
}

// folded is the letter each accented letter in letters folds to.
var folded = func() map[rune]rune {
	m := map[rune]rune{}
	for base, variants := range letters {
		for _, v := range variants {
			m[v] = base
		}
	}
	return m
}()

// SearchPattern is a regular expression matching text that contains q,
// ignoring case and accents. Japanese text is matched as it is.
func SearchPattern(q string) string {
	var b strings.Builder
	for _, r := range q {
		r = unicode.ToLower(r)
		if base, ok := folded[r]; ok {
			r = base
		}
		variants, ok := letters[r]
		if !ok {
			b.WriteString(regexp.QuoteMeta(string(r)))
			continue
		}
		// Upper case too: MongoDB's "i" option may not fold accented letters.
		b.WriteString("[" + variants + strings.ToUpper(variants) + "]")
	}
	return b.String()
}

// noMatch is the length given to a field that doesn't match, longer than any.
const noMatch = 1 << 30

// MatchesAny is a filter for documents in which any of fields matches
// pattern, ignoring case.
func MatchesAny(pattern string, fields ...string) bson.D {
	or := bson.A{}
	for _, f := range fields {
		or = append(or, bson.D{{Key: f, Value: bson.D{{Key: "$regex", Value: pattern}, {Key: "$options", Value: "i"}}}})
	}
	return bson.D{{Key: "$or", Value: or}}
}

// IsExact is an expression: whether any of fields is all of pattern.
func IsExact(pattern string, fields ...string) bson.D {
	or := bson.A{}
	for _, f := range fields {
		or = append(or, regexMatch(f, "^"+pattern+"$"))
	}
	return bson.D{{Key: "$or", Value: or}}
}

// MatchedLength is an expression: the length of the shortest of fields that
// matches pattern.
func MatchedLength(pattern string, fields ...string) bson.D {
	lengths := bson.A{}
	for _, f := range fields {
		lengths = append(lengths, bson.D{{Key: "$cond", Value: bson.A{
			regexMatch(f, pattern),
			bson.D{{Key: "$strLenCP", Value: orEmpty(f)}},
			noMatch,
		}}})
	}
	return bson.D{{Key: "$min", Value: lengths}}
}

// orEmpty is an expression: field, or "" if it is missing.
func orEmpty(field string) bson.D {
	return bson.D{{Key: "$ifNull", Value: bson.A{"$" + field, ""}}}
}

func regexMatch(field, pattern string) bson.D {
	return bson.D{{Key: "$regexMatch", Value: bson.D{
		{Key: "input", Value: orEmpty(field)},
		{Key: "regex", Value: pattern},
		{Key: "options", Value: "i"},
	}}}
}
