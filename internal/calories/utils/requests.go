package utils

import (
	"net/http"

	coremodel "github.com/feriosch/nando-server/internal/core/model"
)

// PathDay returns the Day in r's path, or answers 400 and returns false.
func PathDay(w http.ResponseWriter, r *http.Request) (coremodel.Day, bool) {
	day, err := coremodel.ParseDay(r.PathValue("day"))
	if err != nil {
		http.Error(w, "the Day must be YYYY-MM-DD", http.StatusBadRequest)
		return "", false
	}
	return day, true
}
