package utils

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// ExpiresAtIndex has MongoDB delete each document within about a minute after
// its expiresAt. Queries should match only Unexpired documents, so none can be
// used in between.
var ExpiresAtIndex = mongo.IndexModel{
	Keys:    bson.D{{Key: "expiresAt", Value: 1}},
	Options: options.Index().SetExpireAfterSeconds(0),
}

// Unexpired matches documents whose expiresAt is after now.
func Unexpired(now time.Time) bson.E {
	return bson.E{Key: "expiresAt", Value: bson.D{{Key: "$gt", Value: now}}}
}
