package utils

import "github.com/feriosch/nando-server/internal/calories/model"

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

// ToEntryJSONs returns entries as the API lists them.
func ToEntryJSONs(entries []model.Entry) []model.EntryJSON {
	list := make([]model.EntryJSON, len(entries))
	for i, e := range entries {
		list[i] = model.EntryJSON{
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
