package utils

import (
	"github.com/feriosch/nando-server/internal/calories/model"
	coremodel "github.com/feriosch/nando-server/internal/core/model"
)

// OrNull is s, or nil if s is "": an optional string as the API writes it.
func OrNull(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// OrEmpty is *s, or "" if s is nil: an optional string as the API was sent it.
func OrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// EntryJSON is an Entry as the API lists it.
type EntryJSON struct {
	ID             string        `json:"id"`
	Day            coremodel.Day `json:"day"`
	MealID         *string       `json:"mealId"`
	Name           string        `json:"name"`
	KcalPerServing int           `json:"kcalPerServing"`
	Servings       int           `json:"servings"`
	Kcal           int           `json:"kcal"`
	LoggedAt       string        `json:"loggedAt"`
}

// ToEntryJSONs returns entries as the API lists them.
func ToEntryJSONs(entries []model.Entry) []EntryJSON {
	list := make([]EntryJSON, len(entries))
	for i, e := range entries {
		list[i] = EntryJSON{
			ID:             e.ID,
			Day:            e.Day,
			MealID:         OrNull(e.MealID),
			Name:           e.Name,
			KcalPerServing: e.KcalPerServing,
			Servings:       e.Servings,
			Kcal:           e.Kcal(),
			LoggedAt:       e.LoggedAt.UTC().Format("2006-01-02T15:04:05Z"),
		}
	}
	return list
}
