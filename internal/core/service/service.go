// Package service handles Core's requests and validates them.
// It never touches the database; it reaches data only through CoreRepository.
package service

import "github.com/feriosch/nando-server/internal/core/model"

// Service handles Core's requests.
type Service struct {
	repo model.CoreRepository
}

// New returns a Service reaching data through repo.
func New(repo model.CoreRepository) *Service {
	return &Service{repo: repo}
}
