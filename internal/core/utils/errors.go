// Package utils holds helpers Core's layers share, which Modules may use too.
package utils

import (
	"errors"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

// ErrNotFound is returned by Core's and Modules' repositories when there's
// nothing to return, including when what was asked for has expired.
var ErrNotFound = errors.New("not found")

// NotFound turns the driver's "no documents" into ErrNotFound, and returns
// any other err as it is.
func NotFound(err error) error {
	if errors.Is(err, mongo.ErrNoDocuments) {
		return ErrNotFound
	}
	return err
}

// Matched turns an update that matched no document into ErrNotFound, and
// returns any other err as it is.
func Matched(result *mongo.UpdateResult, err error) error {
	if err == nil && result.MatchedCount == 0 {
		return ErrNotFound
	}
	return err
}

// Deleted turns a delete that deleted no document into ErrNotFound, and
// returns any other err as it is.
func Deleted(result *mongo.DeleteResult, err error) error {
	if err == nil && result.DeletedCount == 0 {
		return ErrNotFound
	}
	return err
}
