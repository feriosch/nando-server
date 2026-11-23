package model

import "context"

// Settings are Money's settings: one document.
type Settings struct {
	// AskAbove is, per currency, the amount above which a Transaction Claude
	// can't tell is asked about; below it, Claude's best guess is kept,
	// Unconfirmed (ADR 0014).
	AskAbove Amounts `bson:"askAbove"`
}

// Amounts is one whole amount in each of the Owner's currencies.
type Amounts struct {
	JPY int `bson:"jpy"`
	MXN int `bson:"mxn"`
}

// DefaultSettings are the settings until the Owner first saves theirs.
func DefaultSettings() Settings {
	return Settings{AskAbove: Amounts{JPY: 3000, MXN: 300}}
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
	AskAbove map[string]int `json:"askAbove,omitempty"`
}
