package model

import (
	"context"
	"time"
)

// GmailFiling is one entry of the Filing Record: what a Gmail Filing changed
// on one thread (ADR 0011), kept so the Owner can ask for it to be undone.
type GmailFiling struct {
	ID string `bson:"_id"`
	// Day is the Day the run recorded it, in the Owner Time Zone.
	Day Day `bson:"day"`
	// At is when it was recorded; retention counts from it.
	At       time.Time `bson:"at"`
	ThreadID string    `bson:"threadId"`
	Subject  string    `bson:"subject"`
	From     string    `bson:"from"`
	// Added and Removed are the labels the run added and removed. INBOX
	// removed means the thread was archived.
	Added   []GmailLabel `bson:"added"`
	Removed []GmailLabel `bson:"removed"`
	Starred bool         `bson:"starred"`
	Reason  string       `bson:"reason"`
	// UndoneAt is when the run said it undid the change, or nil.
	UndoneAt *time.Time `bson:"undoneAt,omitempty"`
}

// GmailLabel is a Gmail label, by Gmail's ID and its name.
type GmailLabel struct {
	ID   string `bson:"id" json:"id"`
	Name string `bson:"name" json:"name"`
}

// GmailFilingsFilter picks Filing Record entries: those recorded from From to
// To, both Days included, whose subject, sender or reason contains Query, if
// it isn't "".
type GmailFilingsFilter struct {
	From, To Day
	Query    string
}

// GmailFilingsRepository reads and writes the gmail_filings collection.
type GmailFilingsRepository interface {
	// Add stores filings.
	Add(ctx context.Context, filings []GmailFiling) error
	// List returns the entries filter picks, oldest first: at most limit,
	// the newest ones, and whether more were picked.
	List(ctx context.Context, filter GmailFilingsFilter, limit int) ([]GmailFiling, bool, error)
	// MarkUndone marks the entry id undone at at, unless it already is, and
	// returns it.
	MarkUndone(ctx context.Context, id string, at time.Time) (GmailFiling, error)
	// DeleteBefore deletes the entries recorded before t.
	DeleteBefore(ctx context.Context, t time.Time) error
}

// GmailFilingJSON is a Filing Record entry as the Tools show it.
type GmailFilingJSON struct {
	ID       string       `json:"id"`
	Day      Day          `json:"day"`
	ThreadID string       `json:"threadId"`
	Subject  string       `json:"subject"`
	From     string       `json:"from"`
	Added    []GmailLabel `json:"added"`
	Removed  []GmailLabel `json:"removed"`
	Starred  bool         `json:"starred"`
	Reason   string       `json:"reason"`
	Undone   bool         `json:"undone"`
}

// JSON is f as the Tools show it.
func (f GmailFiling) JSON() GmailFilingJSON {
	return GmailFilingJSON{
		ID: f.ID, Day: f.Day, ThreadID: f.ThreadID, Subject: f.Subject, From: f.From,
		Added: f.Added, Removed: f.Removed, Starred: f.Starred, Reason: f.Reason, Undone: f.UndoneAt != nil,
	}
}
