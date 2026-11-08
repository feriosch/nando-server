package model

import (
	"context"
	"time"

	"github.com/feriosch/nando-server/internal/core/utils"
)

// Settings are what the Calories Settings Section saves besides the Daily
// Limit: one document.
type Settings struct {
	// NoEntriesReminder reminds the Owner when today has no Entries yet.
	NoEntriesReminder NotificationSetting `bson:"noEntriesReminder"`
	// EveningSummary tells the Owner how today went.
	EveningSummary NotificationSetting `bson:"eveningSummary"`
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
}

// SettingsRepository reads and writes the settings collection.
type SettingsRepository interface {
	// Find returns the settings, or Core's utils.ErrNotFound if none were
	// ever saved.
	Find(ctx context.Context) (Settings, error)
	Save(ctx context.Context, settings Settings) error
}
