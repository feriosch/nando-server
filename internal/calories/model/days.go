package model

import coremodel "github.com/feriosch/nando-server/internal/core/model"

// There's no days collection: a Day's Consumed Calories are derived from its
// Entries and the Daily Limits (DayCalories, in daily_limits.go). The JSON
// shapes below are how the API answers a Day.

// CaloriesJSON is a Day's Consumed Calories against its Daily Limit, as the
// Day view, the preview, the history and the Home Card answer them.
type CaloriesJSON struct {
	Day              coremodel.Day `json:"day"`
	ConsumedCalories *int          `json:"consumedCalories"`
	DailyLimit       *int          `json:"dailyLimit"`
	Remaining        *int          `json:"remaining"`
	OverLimit        *bool         `json:"overLimit"`
}

// DayJSON is the Day view as the API answers it.
type DayJSON struct {
	CaloriesJSON
	Entries []EntryJSON `json:"entries"`
}
