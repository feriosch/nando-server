package model

import "context"

// ProfileRepository reads and writes the profile collection: the Career
// Profile, one Markdown document.
type ProfileRepository interface {
	// Find returns the Career Profile, or "" if it was never saved.
	Find(ctx context.Context) (string, error)
	// Save replaces the Career Profile.
	Save(ctx context.Context, markdown string) error
}

// ProfileJSON is the Career Profile, as its Tools read and write it.
type ProfileJSON struct {
	Markdown string `json:"markdown"`
}
