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

// ClaimedWordRequestsJSON is what japanese_list_word_requests answers: the
// Word Requests a Claude run has just claimed.
type ClaimedWordRequestsJSON struct {
	WordRequests []ClaimedWordRequestJSON `json:"wordRequests"`
}

// ClaimedWordRequestJSON is a Word Request as a Claude run gets it: what to
// do, and the Owner's answer to an earlier question, if any.
type ClaimedWordRequestJSON struct {
	ID       string   `json:"id"`
	Text     string   `json:"text"`
	Kind     string   `json:"kind"`
	Group    *string  `json:"group"`
	WordID   *string  `json:"wordId"`
	Question *string  `json:"question"`
	Choices  []string `json:"choices"`
	Answer   *string  `json:"answer"`
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
	WordID      *string
	Summary     *string
	Suggestions *[]string
	Question    *string
	Choices     *[]string
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
	// FailUnfinished fails every Word Request that is Working with its claim
	// run out by now after maxAttempts claims, with a summary saying no run
	// finished it.
	FailUnfinished(ctx context.Context, now time.Time, maxAttempts int) error
	// ClaimNext claims, in one write, the oldest Word Request that is
	// Pending, or Working with its claim run out by now and fewer than
	// maxAttempts claims, for a Claude run: it becomes Working until
	// claimedUntil, with one more attempt. It returns Core's
	// utils.ErrNotFound if there is none.
	ClaimNext(ctx context.Context, now, claimedUntil time.Time, maxAttempts int) (WordRequest, error)
	// AnyClaimable reports whether ClaimNext would claim a Word Request.
	AnyClaimable(ctx context.Context, now time.Time, maxAttempts int) (bool, error)
	// AnyClaimed reports whether a Claude run holds a Word Request: one is
	// Working with its claim not run out by now.
	AnyClaimed(ctx context.Context, now time.Time) (bool, error)
	// GetOneByID returns the Word Request with id, or Core's
	// utils.ErrNotFound.
	GetOneByID(ctx context.Context, id string) (WordRequest, error)
	// GetListed returns, newest first, the Word Requests the Owner hasn't
	// dismissed that are open or finished (Done or Failed) after
	// finishedAfter.
	GetListed(ctx context.Context, finishedAfter time.Time) ([]WordRequest, error)
}
