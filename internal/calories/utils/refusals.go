package utils

import (
	"net/http"
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
