package model

import (
	"context"
	"time"
)

// ClaudeWakesRepository reads and writes the claude_wakes collection: when
// Nando last woke Claude to work through the Word Requests. It is kept in
// the database, not in memory, because every instance of the server wakes
// the same Claude.
type ClaudeWakesRepository interface {
	// Claim records at as when Claude was last woken, unless it was last
	// woken after since, and reports whether it did. One write decides, so
	// of instances claiming at once only one gets it.
	Claim(ctx context.Context, at, since time.Time) (bool, error)
}
