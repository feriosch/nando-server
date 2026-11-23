package model

import "context"

// Settings are Money's settings: one document.
type Settings struct {
	// AskAbove is, per currency, the amount above which a Transaction Claude
	// can't tell is asked about; below it, Claude's best guess is kept,
	// Unconfirmed (ADR 0014).
	AskAbove Amounts `bson:"askAbove"`
	// Reminders are the Statement Reminders, one per app the Owner opens.
	Reminders []ReminderGroup `bson:"reminders"`
	// ReminderTime is when, in the Owner Time Zone, Statement Reminders and
	// Monthly Review asks are sent: "HH:MM" on the hour or half hour.
	ReminderTime string `bson:"reminderTime"`
}

// ReminderGroup is one app the Owner opens for Statements, such as one
// bank's app that holds a card, an account and a savings pot: its Statement
// Reminder is sent on its day of the month while any of its Accounts'
// Statements is missing.
type ReminderGroup struct {
	Name       string   `bson:"name"`
	AccountIDs []string `bson:"accountIds"`
	// Day is the day of the month, 1 to 28, so that every month has it.
	Day int `bson:"day"`
}

// Amounts is one whole amount in each of the Owner's currencies.
type Amounts struct {
	JPY int `bson:"jpy"`
	MXN int `bson:"mxn"`
}

// DefaultSettings are the settings until the Owner first saves theirs.
func DefaultSettings() Settings {
	return Settings{AskAbove: Amounts{JPY: 3000, MXN: 300}, Reminders: []ReminderGroup{}, ReminderTime: "09:00"}
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
// out of a save is nil, and keeps its saved value; within askAbove, so is a
// currency left out.
type SettingsJSON struct {
	AskAbove     map[string]int       `json:"askAbove,omitempty"`
	Reminders    *[]ReminderGroupJSON `json:"reminders,omitempty"`
	ReminderTime *string              `json:"reminderTime,omitempty"`
}

// ReminderGroupJSON is a ReminderGroup as the Tools read and write it.
type ReminderGroupJSON struct {
	Name     string   `json:"name"`
	Accounts []string `json:"accounts"`
	Day      int      `json:"day"`
}
