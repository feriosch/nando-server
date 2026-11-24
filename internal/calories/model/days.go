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

// DayTotalsJSON is a Day's Consumed Calories against its Daily Limit, and
// its Steps and Burned Calories, as the history and the Home Card answer
// them.
type DayTotalsJSON struct {
	CaloriesJSON
	ActivityTotalsJSON
}

// DayJSON is the Day view as the API answers it: its Consumed Calories
// against its Daily Limit, its Steps and Burned Calories, and what they come
// from.
type DayJSON struct {
	CaloriesJSON
	ActivityTotalsJSON
	Entries  []EntryJSON  `json:"entries"`
	Activity ActivityJSON `json:"activity"`
}
