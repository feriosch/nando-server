package utils

import (
	"regexp"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Containing matches a string with query in it, ignoring case.
func Containing(query string) bson.Regex {
	return bson.Regex{Pattern: regexp.QuoteMeta(query), Options: "i"}
}

// Exactly matches the string s, ignoring case.
func Exactly(s string) bson.Regex {
	return bson.Regex{Pattern: "^" + regexp.QuoteMeta(s) + "$", Options: "i"}
}

// ByName sorts what a search finds by name.
func ByName() *options.FindOptionsBuilder {
	return options.Find().SetSort(bson.D{{Key: "name", Value: 1}})
}
