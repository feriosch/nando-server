package model

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Sync is one pass in which the Mac's script handed Nando new LinkedIn
// messages, or told Nando it couldn't.
type Sync struct {
	ID bson.ObjectID `bson:"_id,omitempty"`
	At time.Time     `bson:"at"`
	// Threads is how many threads it sent.
	Threads int `bson:"threads"`
	// Problem is why the script couldn't read LinkedIn, if it couldn't. A
	// Sync with a problem doesn't count towards when the next is due.
	Problem *SyncProblem `bson:"problem,omitempty"`
}

// SyncProblem is why a Sync couldn't read LinkedIn.
type SyncProblem struct {
	// Kind is ProblemSessionExpired or ProblemOther.
	Kind string `bson:"kind"`
	Text string `bson:"text,omitempty"`
}

// What kept a Sync from reading LinkedIn.
const (
	// ProblemSessionExpired is LinkedIn wanting the Owner to log in again
	// on the Mac.
	ProblemSessionExpired = "session_expired"
	ProblemOther          = "other"
)

// SyncsRepository reads and writes the syncs collection.
type SyncsRepository interface {
	// Insert stores a Sync and returns its id.
	Insert(ctx context.Context, sync Sync) (bson.ObjectID, error)
	// LatestRead returns the newest n Syncs that read LinkedIn, with no
	// problem, newest first.
	LatestRead(ctx context.Context, n int64) ([]Sync, error)
}

// The JSON shapes below are what the Sync Tools answer with and are sent.

// SyncStatusJSON is what the Mac's Sync script is told before it reads
// LinkedIn: whether a Sync is due now, how far back to read, and the threads
// Nando already holds.
type SyncStatusJSON struct {
	Due bool `json:"due"`
	// Since is the oldest Day, as YYYY-MM-DD, whose messages a Sync reads.
	Since   string       `json:"since"`
	Threads []ThreadJSON `json:"threads"`
}

// ThreadJSON is one LinkedIn thread Nando holds, for the Sync script to
// compare with LinkedIn's inbox.
type ThreadJSON struct {
	ThreadID string    `json:"threadId"`
	Newest   time.Time `json:"newest"`
	Ignored  bool      `json:"ignored"`
}

// RecordSyncJSON is what the Sync script sends: the new messages of each
// thread it opened.
type RecordSyncJSON struct {
	Threads []ThreadUpdateJSON `json:"threads"`
	// Problem is why the script couldn't read LinkedIn, if it couldn't.
	Problem *SyncProblemJSON `json:"problem"`
}

// SyncProblemJSON is why a Sync couldn't read LinkedIn.
type SyncProblemJSON struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// ThreadUpdateJSON is one thread in a Sync.
type ThreadUpdateJSON struct {
	ThreadID string        `json:"threadId"`
	URL      string        `json:"url"`
	Person   PersonJSON    `json:"person"`
	Messages []MessageJSON `json:"messages"`
}

// PersonJSON is the other side of a LinkedIn thread.
type PersonJSON struct {
	Name       string `json:"name"`
	ProfileURL string `json:"profileUrl"`
}

// MessageJSON is one LinkedIn message as the Sync script sends it.
type MessageJSON struct {
	ID     string    `json:"id"`
	From   string    `json:"from"`
	SentAt time.Time `json:"sentAt"`
	Text   string    `json:"text"`
}

// RecordedSyncJSON is what career_record_sync answers: the Sync's id and how
// many new messages it stored.
type RecordedSyncJSON struct {
	Sync   string `json:"sync"`
	Stored int    `json:"stored"`
}
