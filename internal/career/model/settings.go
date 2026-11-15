package model

import "context"

// Settings are the Owner's Career settings: one document.
type Settings struct {
	// Looking is whether the Owner is looking for a new role right now.
	Looking bool `bson:"looking"`
}

// DefaultSettings are the settings until the Owner first saves theirs: Not
// Looking.
var DefaultSettings = Settings{Looking: false}

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
}
