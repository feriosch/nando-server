package model

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// GmailPass is one Gmail Career Sync's pass (ADR 0010). Passes are kept apart
// from the LinkedIn Syncs, so they never count toward a LinkedIn Sync's
// limits or alerts.
type GmailPass struct {
	ID        bson.ObjectID `bson:"_id,omitempty"`
	StartedAt time.Time     `bson:"startedAt"`
	// FinishedAt is zero until the run finishes the pass.
	FinishedAt time.Time `bson:"finishedAt,omitempty"`
}

// GmailBacklogStart is how far back the first Gmail Career Sync reads, as the
// first LinkedIn Sync did.
var GmailBacklogStart = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

// GmailPassesRepository reads and writes the gmail_passes collection.
type GmailPassesRepository interface {
	// Start stores a new pass started at at and returns it.
	Start(ctx context.Context, at time.Time) (GmailPass, error)
	// Finish marks the pass finished at at, or returns Core's
	// utils.ErrNotFound.
	Finish(ctx context.Context, id bson.ObjectID, at time.Time) error
	// LastFinished returns the finished pass that started last, or Core's
	// utils.ErrNotFound if none has finished.
	LastFinished(ctx context.Context) (GmailPass, error)
}

// GmailPassJSON is what career_start_gmail_career_sync answers: the pass, since
// when to read the labeled threads Nando holds, and which those are.
type GmailPassJSON struct {
	PassID string       `json:"passId"`
	Since  time.Time    `json:"since"`
	Held   []ThreadJSON `json:"held"`
}
