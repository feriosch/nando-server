package service

import (
	"net/http"

	coremodel "github.com/feriosch/nando-server/internal/core/model"
)

// editableDays is how many Days before today the Editable Window reaches.
const editableDays = 7

// checkEditable reports whether day is in the Editable Window: today, in the
// Owner Time Zone, or one of the 7 Days before it. If it isn't, it answers 409
// (or 500 if today can't be known) and returns false.
func (s *Service) checkEditable(w http.ResponseWriter, r *http.Request, day coremodel.Day) bool {
	today, err := s.core.Today(r.Context())
	if err != nil {
		serverError(w, err)
		return false
	}
	if day > today || day < today.AddDays(-editableDays) {
		http.Error(w, "only today and the 7 Days before it can be changed", http.StatusConflict)
		return false
	}
	return true
}
