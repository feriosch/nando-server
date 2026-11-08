// Package utils holds helpers Core's layers share, which Modules may use too.
package utils

import (
	"errors"

	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/feriosch/nando-server/internal/core/model"
)

// NotFound turns the driver's "no documents" into model.ErrNotFound, and
// returns any other err as it is.
func NotFound(err error) error {
	if errors.Is(err, mongo.ErrNoDocuments) {
		return model.ErrNotFound
	}
	return err
}

// Matched turns an update that matched no document into model.ErrNotFound,
// and returns any other err as it is.
func Matched(result *mongo.UpdateResult, err error) error {
	if err == nil && result.MatchedCount == 0 {
		return model.ErrNotFound
	}
	return err
}
