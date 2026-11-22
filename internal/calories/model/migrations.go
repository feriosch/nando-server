package model

import "context"

// MigrationsRepository reads and writes the migrations collection: one
// document per one-off change to the Module's data that is done, named such
// as "weigh-ins-from-core" (ADR 0012), so that it is never done twice.
type MigrationsRepository interface {
	// Done reports whether the migration name is done.
	Done(ctx context.Context, name string) (bool, error)
	// MarkDone records that the migration name is done.
	MarkDone(ctx context.Context, name string) error
}
