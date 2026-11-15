package utils

import (
	"context"
	"errors"
	"fmt"
	"net/http"

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

// FromPath returns what get finds for the id in r's path and true. When the
// id is no what's, such as "Opportunity", it answers 404 instead, or 500 on
// another error, and returns false.
func FromPath[T any](w http.ResponseWriter, r *http.Request, what string, get func(context.Context, bson.ObjectID) (T, error)) (T, bool) {
	var found T
	id, err := bson.ObjectIDFromHex(r.PathValue("id"))
	if err != nil {
		http.Error(w, "there's no such "+what, http.StatusNotFound)
		return found, false
	}
	found, err = get(r.Context(), id)
	switch {
	case errors.Is(err, coreutils.ErrNotFound):
		http.Error(w, "there's no such "+what, http.StatusNotFound)
		return found, false
	case err != nil:
		coreutils.ServerError(w, err)
		return found, false
	}
	return found, true
}
