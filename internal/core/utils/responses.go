package utils

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/feriosch/nando-server/internal/core/model"
)

// WriteJSON answers with status and v as JSON, so every part of the API
// answers alike.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response: %v", err)
	}
}

// WriteCreated answers 201 with the ID of what a POST created, as
// model.CreatedJSON: {"id": "…"}.
func WriteCreated(w http.ResponseWriter, id string) {
	WriteJSON(w, http.StatusCreated, model.CreatedJSON{ID: id})
}

// ServerError logs err and answers 500 without revealing it.
func ServerError(w http.ResponseWriter, err error) {
	log.Print(err)
	http.Error(w, "something went wrong", http.StatusInternalServerError)
}
