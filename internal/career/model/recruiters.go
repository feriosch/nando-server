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
	// Search returns the Recruiters whose name or organisation has query in
	// it, ignoring case.
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
