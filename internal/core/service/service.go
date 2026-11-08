// Package service handles Core's requests and validates them, in one service
// per file, each holding only the repositories it uses. It never touches the
// database; it reaches data only through model's repository interfaces.
package service

import (
	"encoding/json"
	"log"
	"net/http"
)

// WriteJSON answers with status and v as JSON. Modules use it too, so every
// part of the API answers alike.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response: %v", err)
	}
}

// WriteCreated answers 201 with the ID of what a POST created, as
// {"id": "…"}. Modules use it too.
func WriteCreated(w http.ResponseWriter, id string) {
	WriteJSON(w, http.StatusCreated, struct {
		ID string `json:"id"`
	}{id})
}

// ServerError logs err and answers 500 without revealing it. Modules use it
// too.
func ServerError(w http.ResponseWriter, err error) {
	log.Print(err)
	http.Error(w, "something went wrong", http.StatusInternalServerError)
}
