package model

import (
	"context"
	"time"
)

// ScheduledTaskRepository reads and writes the scheduled_task collection:
// when Nando last woke the scheduled task that works through Word Requests,
// and when a run was last active. It is in the database, not in memory, as
// every instance of Nando wakes the same scheduled task.
type ScheduledTaskRepository interface {
	// ClaimFire records a fire at now and reports whether it did. It doesn't
	// while a fire is in flight, one after startingSince that no run has
	// worked or stopped since, nor while a run may be between Word Requests,
	// having worked after activeSince and not stopped since. One write
	// decides, so of instances claiming at once only one gets it.
	ClaimFire(ctx context.Context, now, startingSince, activeSince time.Time) (bool, error)
	// ReleaseFire forgets the fire at firedAt, which started no run, so it
	// isn't in flight. A later fire is left as it is.
	ReleaseFire(ctx context.Context, firedAt time.Time) error
	// RecordWorking records that a run claimed or resolved a Word Request at
	// now.
	RecordWorking(ctx context.Context, now time.Time) error
	// RecordStopped records that a run found nothing left to claim at now,
	// so it stops.
	RecordStopped(ctx context.Context, now time.Time) error
}
