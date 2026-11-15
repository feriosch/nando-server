package model

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Conversation is one LinkedIn thread between the Owner and one other person,
// with the copy of its messages that Syncs brought. Until it is filed under a
// Recruiter it is unsorted.
type Conversation struct {
	ID       bson.ObjectID `bson:"_id,omitempty"`
	ThreadID string        `bson:"threadId"`
	URL      string        `bson:"url"`
	Person   Person        `bson:"person"`
	Messages []Message     `bson:"messages"`
	// Newest is when the newest message Nando holds was sent.
	Newest time.Time `bson:"newest"`
	// Ignored is a thread that isn't about a role: it keeps no messages.
	Ignored bool `bson:"ignored"`
}

// Person is the other side of a LinkedIn thread, as LinkedIn names them.
type Person struct {
	Name       string `bson:"name"`
	ProfileURL string `bson:"profileUrl"`
}

// Message is one message in a Conversation.
type Message struct {
	// LinkedInID is LinkedIn's id for the message, if the reader gave one.
	LinkedInID string `bson:"linkedInId,omitempty"`
	// From is MessageFromOwner or MessageFromOther.
	From   string    `bson:"from"`
	SentAt time.Time `bson:"sentAt"`
	Text   string    `bson:"text"`
}

// Who sent a Message.
const (
	MessageFromOwner = "owner"
	MessageFromOther = "other"
)

// ThreadUpdate is what one Sync brought for one LinkedIn thread.
type ThreadUpdate struct {
	ThreadID string
	URL      string
	Person   Person
	Messages []Message
}

// ConversationsRepository reads and writes the conversations collection.
type ConversationsRepository interface {
	// AddMessages adds a Sync's messages to the thread's Conversation, making
	// it if it is new, and returns how many it stored.
	AddMessages(ctx context.Context, update ThreadUpdate) (int, error)
	// Threads returns every Conversation's thread id, newest message time and
	// whether it is ignored, without its messages.
	Threads(ctx context.Context) ([]Conversation, error)
}
