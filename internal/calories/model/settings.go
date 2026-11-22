package model

import (
	"context"
	"time"

	"github.com/feriosch/nando-server/internal/core/utils"
)

// Settings are when the Calories Notifications are sent: one document.
type Settings struct {
	// NoEntriesReminder reminds the Owner when today has no Entries yet.
	NoEntriesReminder NotificationSetting `bson:"noEntriesReminder"`
	// EveningSummary tells the Owner how today went.
	EveningSummary NotificationSetting `bson:"eveningSummary"`
	// WeighInReminder reminds the Owner each week to weigh in (ADR 0012).
	WeighInReminder WeighInReminder `bson:"weighInReminder"`
}

// WeighInReminder is whether, and when each week, the Weigh-in reminder is
// sent.
type WeighInReminder struct {
	Enabled bool         `bson:"enabled"`
	Weekday time.Weekday `bson:"weekday"`
	// Time is the clock time in the Owner Time Zone, "HH:MM" on the hour or
	// half hour.
	Time string `bson:"time"`
}

// DueAt reports whether the reminder is switched on and set for slot, the
// half hour of a tick in the Owner Time Zone.
func (r WeighInReminder) DueAt(slot time.Time) bool {
	return r.Enabled && slot.Weekday() == r.Weekday && utils.IsAt(slot, r.Time)
}

// NotificationSetting is whether, and when each Day, one of the Calories
// Notifications is sent.
type NotificationSetting struct {
	Enabled bool `bson:"enabled"`
	// Time is the clock time in the Owner Time Zone, "HH:MM" on the hour or
	// half hour.
	Time string `bson:"time"`
}

// DueAt reports whether the Notification is switched on and set for slot, the
// half hour of a tick in the Owner Time Zone.
func (s NotificationSetting) DueAt(slot time.Time) bool {
	return s.Enabled && utils.IsAt(slot, s.Time)
}

// DefaultSettings are the settings until the Owner first saves theirs.
var DefaultSettings = Settings{
	NoEntriesReminder: NotificationSetting{Enabled: false, Time: "14:00"},
	EveningSummary:    NotificationSetting{Enabled: false, Time: "21:00"},
	WeighInReminder:   WeighInReminder{Enabled: false, Weekday: time.Monday, Time: "08:00"},
}

// SettingsRepository reads and writes the settings collection.
type SettingsRepository interface {
	// Find returns the settings, or Core's utils.ErrNotFound if none were
	// ever saved.
	Find(ctx context.Context) (Settings, error)
	Save(ctx context.Context, settings Settings) error
}

// The JSON shapes below are what the API answers with and is sent.

// SettingsJSON is the settings as the API reads and writes them. A missing
// Notification setting is nil, so it can be rejected.
type SettingsJSON struct {
	NoEntriesReminder *NotificationSettingJSON `json:"noEntriesReminder"`
	EveningSummary    *NotificationSettingJSON `json:"eveningSummary"`
	WeighInReminder   *WeighInReminderJSON     `json:"weighInReminder"`
}

// NotificationSettingJSON is one Notification's setting. Enabled is nil when
// it wasn't sent, so it can be rejected.
type NotificationSettingJSON struct {
	Enabled *bool  `json:"enabled"`
	Time    string `json:"time"`
}

// WeighInReminderJSON is the Weigh-in reminder's setting. Weekday is its API
// name, such as "monday". Enabled is nil when it wasn't sent, so it can be
// rejected.
type WeighInReminderJSON struct {
	Enabled *bool  `json:"enabled"`
	Weekday string `json:"weekday"`
	Time    string `json:"time"`
}
