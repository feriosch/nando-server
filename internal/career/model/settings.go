package model

import "context"

// Settings are the Owner's Career settings: one document.
type Settings struct {
	// Looking is whether the Owner is looking for a new role right now.
	Looking bool `bson:"looking"`
	// GmailCareerSync is when Nando asks the Career Assistant for a Gmail
	// Career Sync (ADR 0010).
	GmailCareerSync GmailCareerSync `bson:"gmailCareerSync"`
}

// GmailCareerSync is the daily Notification that asks the Career Assistant
// to file the new mail under the Gmail label Career.
type GmailCareerSync struct {
	Enabled bool `bson:"enabled"`
	// Time is the clock time in the Owner Time Zone, "HH:MM" on the hour or
	// half hour.
	Time string `bson:"time"`
}

// DefaultSettings are the settings until the Owner first saves theirs: Not
// Looking, and the Gmail Career Sync off, at 08:00.
var DefaultSettings = Settings{Looking: false, GmailCareerSync: GmailCareerSync{Enabled: false, Time: "08:00"}}

// SettingsRepository reads and writes the settings collection.
type SettingsRepository interface {
	// Find returns the settings, or Core's utils.ErrNotFound if none were
	// ever saved.
	Find(ctx context.Context) (Settings, error)
	Save(ctx context.Context, settings Settings) error
}

// The JSON shapes below are what the API answers with and is sent.

// SettingsJSON is the settings as the API reads and writes them. A missing
// Looking is nil, so it can be rejected.
type SettingsJSON struct {
	Looking *bool `json:"looking"`
	// GmailCareerSync is always answered. A PUT may leave it out, keeping
	// the saved one, so a web app that doesn't know it yet can't turn it off.
	GmailCareerSync *GmailCareerSyncJSON `json:"gmailCareerSync,omitempty"`
}

// GmailCareerSyncJSON is the Gmail Career Sync as the API reads and writes it.
type GmailCareerSyncJSON struct {
	Enabled bool   `json:"enabled"`
	Time    string `json:"time"`
}
