// Package repository is the Calories Module's data access: the only part of the
// Module that touches the database, and only the Module's own database. There
// is one repository per collection, each in the file named after it.
package repository

import (
	"errors"

	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/feriosch/nando-server/internal/calories/model"
)

// matched turns an update that matched no document into model.ErrNotFound.
func matched(result *mongo.UpdateResult, err error) error {
	if err == nil && result.MatchedCount == 0 {
		return model.ErrNotFound
	}
	return err
}

// notFound turns the driver's "no documents" into model.ErrNotFound.
func notFound(err error) error {
	if errors.Is(err, mongo.ErrNoDocuments) {
		return model.ErrNotFound
	}
	return err
}
