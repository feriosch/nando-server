package model

import (
	"context"
	"time"
)

// ScheduledTaskRepository reads and writes the scheduled_task collection:
// when Nando last woke the scheduled task that works through Word Requests.
// It is in the database, not in memory, as every instance of Nando wakes the
// same scheduled task.
type ScheduledTaskRepository interface {
	// ClaimFire records a fire at now unless one is recorded after
	// notSince, and reports whether it did. One write decides, so of
	// instances claiming at once only one gets it.
	ClaimFire(ctx context.Context, now, notSince time.Time) (bool, error)
}
