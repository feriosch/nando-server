package model

import "context"

// TickJob is work a Module (or Core itself) does at each tick, every half
// hour. Core runs every tick job without knowing what it does.
type TickJob interface {
	// RunAtTick does the work. An error fails the tick, which the scheduler
	// then retries, so it is for work that should be tried again soon.
	RunAtTick(ctx context.Context) error
}
