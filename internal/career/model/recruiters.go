package model

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Recruiter is a person who contacts the Owner about roles, from an agency or
// from the hiring Company itself. A Lead is worked out from their
// Opportunities, never stored.
type Recruiter struct {
	ID         bson.ObjectID `bson:"_id,omitempty"`
	Name       string        `bson:"name"`
	ProfileURL string        `bson:"profileUrl"`
	// Kind is RecruiterAgency or RecruiterInHouse.
	Kind string `bson:"kind"`
	// Organisation is their agency or Company, by name.
	Organisation string `bson:"organisation"`
	// Emails are the addresses their filed Gmail threads came from,
	// lowercase (ADR 0010).
	Emails []string `bson:"emails,omitempty"`
}

// The kinds of Recruiter.
const (
	RecruiterAgency  = "agency"
	RecruiterInHouse = "in_house"
)

// RecruitersRepository reads and writes the recruiters collection.
type RecruitersRepository interface {
	// Insert stores a new Recruiter and returns it with its id.
	Insert(ctx context.Context, recruiter Recruiter) (Recruiter, error)
	// GetByID returns the Recruiter, or Core's utils.ErrNotFound.
	GetByID(ctx context.Context, id bson.ObjectID) (Recruiter, error)
	// GiveProfileURL sets the Recruiter's profileUrl to url if they have
	// none, and leaves one they have as it is.
	GiveProfileURL(ctx context.Context, id bson.ObjectID, url string) error
	// AddEmail remembers that the Recruiter writes from email, lowercase,
	// and forgets it for any other Recruiter.
	AddEmail(ctx context.Context, id bson.ObjectID, email string) error
	// RemoveEmail forgets that the Recruiter writes from email, lowercase,
	// and returns them, or Core's utils.ErrNotFound.
	RemoveEmail(ctx context.Context, id bson.ObjectID, email string) (Recruiter, error)
	// ByEmail returns the Recruiter who writes from email, lowercase, or
	// Core's utils.ErrNotFound.
	ByEmail(ctx context.Context, email string) (Recruiter, error)
	// Search returns the Recruiters whose name or organisation has query in
	// it, ignoring case, whose profileUrl is query, or who write from query,
	// ignoring case.
	Search(ctx context.Context, query string) ([]Recruiter, error)
}

// The JSON shapes below are what the Tools and the API answer with and are
// sent.

// RecruiterJSON is a Recruiter as the Tools answer it.
type RecruiterJSON struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ProfileURL   string `json:"profileUrl"`
	Kind         string `json:"kind"`
	Organisation string `json:"organisation"`
	// Emails are the addresses Nando knows them by.
	Emails []string `json:"emails"`
}

// FiledJSON is what career_file_conversation answers: the Conversation and
// the Recruiter it is now filed under.
type FiledJSON struct {
	Conversation ConversationJSON `json:"conversation"`
	Recruiter    RecruiterJSON    `json:"recruiter"`
}

// RecruiterRefJSON names a Recruiter, for the app to link to their page.
type RecruiterRefJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// RecruiterPageJSON is a Recruiter as their page in the app shows them, with
// each of their Conversations.
type RecruiterPageJSON struct {
	RecruiterJSON
	// Conversations are newest first.
	Conversations []RecruiterPageConversationJSON `json:"conversations"`
}
