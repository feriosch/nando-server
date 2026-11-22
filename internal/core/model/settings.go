package model

import (
	"context"
	"time"
)

// Settings are the Owner's preferences that Core keeps: one document.
type Settings struct {
	// TimeZone is the Owner Time Zone, an IANA name such as "Asia/Tokyo".
	TimeZone string `bson:"timeZone"`
	// HeightCm is the Owner's height; 0 means not set yet.
	HeightCm int `bson:"heightCm"`
	// WeighInReminder is the Weigh-in reminder as the Owner saved it before
	// it moved to Calories (ADR 0012): kept, never changed, for Calories to
	// take once. Its Time is "" if they never saved one.
	WeighInReminder WeighInReminder `bson:"weighInReminder"`
	GmailDigest     GmailDigest     `bson:"gmailDigest"`
}

// WeighInReminder is the weekly Notification that reminded the Owner to
// weigh in, before it moved to Calories (ADR 0012).
type WeighInReminder struct {
	Enabled bool         `bson:"enabled"`
	Weekday time.Weekday `bson:"weekday"`
	// Time is the clock time in the Owner Time Zone, "HH:MM" on the hour or
	// half hour.
	Time string `bson:"time"`
}

// GmailDigest is the daily Notification that asks the Core Assistant for
// the Gmail Digest (ADR 0009).
type GmailDigest struct {
	Enabled bool `bson:"enabled"`
	// Time is the clock time in the Owner Time Zone, "HH:MM" on the hour or
	// half hour.
	Time string `bson:"time"`
}

// DefaultSettings are the settings until the Owner first saves theirs.
var DefaultSettings = Settings{
	TimeZone:    "UTC",
	GmailDigest: GmailDigest{Enabled: false, Time: "08:00"},
}

// SettingsRepository reads and writes the settings collection.
type SettingsRepository interface {
	// Find returns the settings, or utils.ErrNotFound if none were ever saved.
	Find(ctx context.Context) (Settings, error)
	Save(ctx context.Context, settings Settings) error
}

// SettingsJSON is the settings as the API reads and writes them.
// A weighInReminder sent by a web app from before ADR 0012 is ignored.
type SettingsJSON struct {
	TimeZone    string           `json:"timeZone"`
	HeightCm    *int             `json:"heightCm"`
	GmailDigest *GmailDigestJSON `json:"gmailDigest,omitempty"`
}

// GmailDigestJSON is the Gmail Digest as the API reads and writes it.
type GmailDigestJSON struct {
	Enabled bool   `json:"enabled"`
	Time    string `json:"time"`
}
