package utils

import (
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"

	coreutils "github.com/feriosch/nando-server/internal/core/utils"
)

// ID is the id hex names in a Tool's arguments, or an error saying that no
// what, such as "Recruiter", has it.
func ID(what, hex string) (bson.ObjectID, error) {
	id, err := bson.ObjectIDFromHex(hex)
	if err != nil {
		return bson.ObjectID{}, NoSuch(what, hex)
	}
	return id, nil
}

// NoSuch is the error that no what, such as "Recruiter", has the id hex.
func NoSuch(what, hex string) error {
	return fmt.Errorf("no %s has the id %q", what, hex)
}

// Found is err, but NoSuch(what, hex) for a repository's ErrNotFound.
func Found(err error, what, hex string) error {
	if errors.Is(err, coreutils.ErrNotFound) {
		return NoSuch(what, hex)
	}
	return err
}
