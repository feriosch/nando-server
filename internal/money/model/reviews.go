package model

import "context"

// ReviewsRepository records which months' Monthly Reviews were asked for, and
// the first month they are asked for: the one before the Module's first tick,
// so that history brought in later never floods the chat (ADR 0014).
type ReviewsRepository interface {
	// From returns the first month Reviews are asked for, setting it to
	// month if it isn't set yet.
	From(ctx context.Context, month string) (string, error)
	// Asked is whether month's Review was asked for.
	Asked(ctx context.Context, month string) (bool, error)
	// Record records that month's Review was asked for.
	Record(ctx context.Context, month string) error
}
