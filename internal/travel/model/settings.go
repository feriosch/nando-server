package model

import (
	"context"
	"time"
)

// Settings are Travel's settings: one document. They hold the Owner's own
// data, such as the Companion, which the repo never names (ADR 0013).
type Settings struct {
	// Home is the airport or city codes the Owner's Trips leave from unless a
	// Trip says otherwise.
	Home []string `bson:"home"`
	// Companion is who travels with the Owner in a Party with the Companion.
	Companion Companion `bson:"companion"`
	// SerpAPIMonthlyLimit is how many Price Verdicts Nando may fetch from
	// SerpApi in a calendar month.
	SerpAPIMonthlyLimit int `bson:"serpApiMonthlyLimit"`
}

// Companion is who travels with the Owner, and what the Travel Assistant
// needs to work out their Leave Needed from their calendars.
type Companion struct {
	// Name is how the Owner calls them; empty while there is no Companion.
	Name string `bson:"name"`
	// Calendars are the Google Calendar IDs that are theirs, read only.
	Calendars []string `bson:"calendars"`
	// WorkWeekdays are the weekdays they work.
	WorkWeekdays []time.Weekday `bson:"workWeekdays"`
	// HolidayCalendar is the Google Calendar ID of the public holidays they
	// have off.
	HolidayCalendar string `bson:"holidayCalendar"`
	// UsualLeaveDays is how many days off one trip may usually take.
	UsualLeaveDays int `bson:"usualLeaveDays"`
	// RemoteDaysAbroad is how many workdays, at most 2, they may work
	// remotely from the destination.
	RemoteDaysAbroad int `bson:"remoteDaysAbroad"`
}

// MaxRemoteDaysAbroad is the most workdays the Companion may work remotely
// from the destination (ADR 0013).
const MaxRemoteDaysAbroad = 2

// DefaultSettings are the settings until the Owner first saves theirs.
func DefaultSettings() Settings {
	return Settings{
		Home: []string{},
		Companion: Companion{
			Calendars:        []string{},
			WorkWeekdays:     []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday},
			UsualLeaveDays:   7,
			RemoteDaysAbroad: MaxRemoteDaysAbroad,
		},
		SerpAPIMonthlyLimit: 250,
	}
}

// SettingsRepository reads and writes the settings collection.
type SettingsRepository interface {
	// Find returns the settings, or Core's utils.ErrNotFound if none were
	// ever saved.
	Find(ctx context.Context) (Settings, error)
	Save(ctx context.Context, settings Settings) error
}

// The JSON shapes below are what the Tools answer with and are sent.

// SettingsJSON is the settings as the Tools read and write them. A field left
// out of a save is nil, and keeps its saved value.
type SettingsJSON struct {
	Home                *[]string      `json:"home"`
	Companion           *CompanionJSON `json:"companion"`
	SerpAPIMonthlyLimit *int           `json:"serpApiMonthlyLimit"`
}

// CompanionJSON is the Companion as the Tools read and write them. Weekdays
// are their API names, such as "monday". A field left out of a save is nil,
// and keeps its saved value.
type CompanionJSON struct {
	Name             *string   `json:"name"`
	Calendars        *[]string `json:"calendars"`
	WorkWeekdays     *[]string `json:"workWeekdays"`
	HolidayCalendar  *string   `json:"holidayCalendar"`
	UsualLeaveDays   *int      `json:"usualLeaveDays"`
	RemoteDaysAbroad *int      `json:"remoteDaysAbroad"`
}
