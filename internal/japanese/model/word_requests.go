package model

import (
	"context"
	"errors"
	"time"
)

// WordRequestKind is what the Owner asked Claude to do.
type WordRequestKind string

const (
	// AddWord asks Claude to add the word the Owner typed.
	AddWord WordRequestKind = "add"
	// ImproveWord asks Claude to improve a stored Word.
	ImproveWord WordRequestKind = "improve"
)

// WordRequestState is where a Word Request is in its life.
type WordRequestState string

const (
	// Pending waits for a Claude run to take it.
	Pending WordRequestState = "pending"
	// Working is taken by a Claude run.
	Working WordRequestState = "working"
	// Done has been added or improved.
	Done WordRequestState = "done"
	// NeedsAnswer waits for the Owner to answer Claude's question.
	NeedsAnswer WordRequestState = "needs_answer"
	// Failed was given up on.
	Failed WordRequestState = "failed"
)

// OpenStates are the states of a Word Request still waiting on Claude or on
// the Owner.
var OpenStates = []WordRequestState{Pending, Working, NeedsAnswer}

// WordRequest is the Owner's ask for Claude to add a word or improve a
// stored Word, and what came of it.
type WordRequest struct {
	ID   string
	Text string
	Kind WordRequestKind
	// Group is the Group key the Owner found the word in, for AddWord.
	Group string
	// WordID is the stored Word to improve, for ImproveWord, or the Word
	// Claude added or improved once Done.
	WordID      string
	State       WordRequestState
	Attempts    int
	Answer      string
	Summary     string
	Suggestions []string
	Question    string
	Choices     []string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// WordRequestJSON is a Word Request as the API writes it.
type WordRequestJSON struct {
	ID          string   `json:"id"`
	Text        string   `json:"text"`
	Kind        string   `json:"kind"`
	Group       *string  `json:"group"`
	WordID      *string  `json:"wordId"`
	State       string   `json:"state"`
	Attempts    int      `json:"attempts"`
	Answer      *string  `json:"answer"`
	Summary     *string  `json:"summary"`
	Suggestions []string `json:"suggestions"`
	Question    *string  `json:"question"`
	Choices     []string `json:"choices"`
}

// NewWordRequestJSON is the body of a new Word Request.
type NewWordRequestJSON struct {
	Text   string `json:"text"`
	Kind   string `json:"kind"`
	Group  string `json:"group"`
	WordID string `json:"wordId"`
}

// WordRequestChanges are the fields an update sets on a Word Request; a nil
// field is left as it is. UpdatedAt is always set.
type WordRequestChanges struct {
	State       *WordRequestState
	Answer      *string
	Attempts    *int
	DismissedAt *time.Time
	UpdatedAt   time.Time
}

// ErrWrongState is an update to a Word Request that isn't in a state the
// update applies to.
var ErrWrongState = errors.New("the Word Request isn't in a state this applies to")

// WordRequestsRepository reads and writes the word_requests collection.
type WordRequestsRepository interface {
	// InsertUnlessOpen stores request, a new Word Request, unless an open
	// one (Pending, Working or NeedsAnswer) asks for the same: the same
	// text to add, or the same Word to improve. It returns the Word Request
	// stored, or the open one, and whether it stored a new one. A unique
	// index decides, so it holds when the Owner taps twice at once.
	InsertUnlessOpen(ctx context.Context, request WordRequest) (WordRequest, bool, error)
	// Update applies changes to the Word Request with id if, at the moment
	// of writing, it is in one of the states in from, and returns it as it is
	// after. It returns Core's utils.ErrNotFound if there's no such Word
	// Request, or ErrWrongState.
	Update(ctx context.Context, id string, from []WordRequestState, changes WordRequestChanges) (WordRequest, error)
	// GetListed returns, newest first, the Word Requests the Owner hasn't
	// dismissed that are open or finished (Done or Failed) after
	// finishedAfter.
	GetListed(ctx context.Context, finishedAfter time.Time) ([]WordRequest, error)
}
