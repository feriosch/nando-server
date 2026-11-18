package utils

import (
	"errors"
	"net/http"

	coreutils "github.com/feriosch/nando-server/internal/core/utils"
)

// Refusal is a request the Module turns down, saying why. The API answers it
// with Status; a Tool shows the Connector's model only its Message.
type Refusal struct {
	Status  int
	Message string
}

func (r Refusal) Error() string { return r.Message }

// Invalid is a Refusal of something sent wrong, which the API answers with
// 400.
func Invalid(message string) error {
	return Refusal{Status: http.StatusBadRequest, Message: message}
}

// WriteError answers err: a Refusal with its status and message, anything
// else as a server error.
func WriteError(w http.ResponseWriter, err error) {
	var refusal Refusal
	if errors.As(err, &refusal) {
		http.Error(w, refusal.Message, refusal.Status)
		return
	}
	coreutils.ServerError(w, err)
}
