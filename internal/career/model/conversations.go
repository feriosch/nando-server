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
	// RecruiterID is the Recruiter it is filed under, if it is filed.
	RecruiterID *bson.ObjectID `bson:"recruiterId,omitempty"`
	// SuggestedReply is Claude's draft of the Owner's next message, if any.
	SuggestedReply *SuggestedReply `bson:"suggestedReply,omitempty"`
	// FollowedUp is when the Owner's message that a follow_up Item was last
	// queued for was sent, so no message gets two.
	FollowedUp time.Time `bson:"followedUp,omitempty"`
}

// SuggestedReply is Claude's draft of the Owner's next message in a
// Conversation. It is gone once a Sync brings a message from the Owner newer
// than the message it answers.
type SuggestedReply struct {
	Text string `bson:"text"`
	// Language is LanguageJA or LanguageEN.
	Language string `bson:"language"`
	// Kind is ReplyAnswer, ReplyFollowUp or ReplyLate.
	Kind string `bson:"kind"`
	// Answers is when the message it answers was sent.
	Answers   time.Time      `bson:"answers"`
	Notes     []LanguageNote `bson:"notes"`
	WrittenAt time.Time      `bson:"writtenAt"`
	// SyncID is the Sync that brought the message it answers, so that
	// Sync's Notification knows it led to a new answer.
	SyncID bson.ObjectID `bson:"syncId,omitempty"`
}

// LanguageNote is a mistake in the Owner's last message: what they wrote,
// what is natural, and why.
type LanguageNote struct {
	Wrote   string `bson:"wrote" json:"wrote"`
	Natural string `bson:"natural" json:"natural"`
	Why     string `bson:"why" json:"why"`
}

// The kinds of Suggested Reply.
const (
	// ReplyAnswer answers the Recruiter's last message.
	ReplyAnswer = "answer"
	// ReplyFollowUp follows up the Owner's own message after 14 days of
	// silence.
	ReplyFollowUp = "follow_up"
	// ReplyLate answers a Recruiter's message more than 30 days old.
	ReplyLate = "late"
)

// MaxLanguageNotes is the most language notes a Suggested Reply has.
const MaxLanguageNotes = 3

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
	// SyncID is the Sync that brought it.
	SyncID bson.ObjectID `bson:"syncId,omitempty"`
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

// Added is what a Sync's messages did to one Conversation.
type Added struct {
	ConversationID bson.ObjectID
	// Stored is how many new messages were stored.
	Stored int
	// Filed is whether the Conversation is filed under a Recruiter.
	Filed bool
	// Ignored is whether the thread is ignored, so nothing was stored.
	Ignored bool
	// FromOther is whether a stored message is the other person's.
	FromOther bool
}

// ConversationsRepository reads and writes the conversations collection.
type ConversationsRepository interface {
	// AddMessages adds a Sync's messages to the thread's Conversation, making
	// it if it is new, and says what it did.
	AddMessages(ctx context.Context, update ThreadUpdate) (Added, error)
	// GetByID returns the Conversation, or Core's utils.ErrNotFound.
	GetByID(ctx context.Context, id bson.ObjectID) (Conversation, error)
	// Ignore marks the Conversation as a thread that isn't about a role:
	// everything but its thread ID and newest message time is deleted. It
	// returns the Conversation, or Core's utils.ErrNotFound.
	Ignore(ctx context.Context, id bson.ObjectID) (Conversation, error)
	// File files the Conversation, unless it is ignored, under the
	// Recruiter, and returns it, or Core's utils.ErrNotFound.
	File(ctx context.Context, id, recruiterID bson.ObjectID) (Conversation, error)
	// ByRecruiter returns the Conversations filed under the Recruiter.
	ByRecruiter(ctx context.Context, recruiterID bson.ObjectID) ([]Conversation, error)
	// SetSuggestedReply replaces the Conversation's Suggested Reply, unless
	// it is ignored, and returns it, or Core's utils.ErrNotFound.
	SetSuggestedReply(ctx context.Context, id bson.ObjectID, reply SuggestedReply) (Conversation, error)
	// Threads returns every Conversation's thread id, newest message time and
	// whether it is ignored, without its messages.
	Threads(ctx context.Context) ([]Conversation, error)
	// WithReplyBefore returns the Conversations whose Suggested Reply is of
	// one of kinds and answers a message sent before before.
	WithReplyBefore(ctx context.Context, kinds []string, before time.Time) ([]Conversation, error)
	// AnyAnswerFromSync reports whether a Conversation's Suggested Reply is
	// an answer to a message the Sync with syncID brought.
	AnyAnswerFromSync(ctx context.Context, syncID bson.ObjectID) (bool, error)
	// WaitingRecruiters counts the Recruiters waiting for the Owner's reply:
	// those with a Conversation whose Suggested Reply is an answer or a
	// follow-up. A Late Reply isn't counted.
	WaitingRecruiters(ctx context.Context) (int, error)
	// MarkFollowedUp records that a follow_up Item was queued for the
	// Owner's message sent at sentAt.
	MarkFollowedUp(ctx context.Context, id bson.ObjectID, sentAt time.Time) error
	// MakeReplyLate turns the Conversation's Suggested Reply late, unless it
	// changed since it was read as reply. It reports whether it did.
	MakeReplyLate(ctx context.Context, id bson.ObjectID, reply SuggestedReply) (bool, error)
}

// ConversationJSON is a Conversation as the Tools answer it.
type ConversationJSON struct {
	ID       string        `json:"id"`
	ThreadID string        `json:"threadId"`
	URL      string        `json:"url"`
	Person   PersonJSON    `json:"person"`
	Messages []MessageJSON `json:"messages"`
	Ignored  bool          `json:"ignored"`
	// RecruiterID is the Recruiter it is filed under, or null.
	RecruiterID    *string             `json:"recruiterId"`
	SuggestedReply *SuggestedReplyJSON `json:"suggestedReply"`
}

// SuggestedReplyJSON is a Suggested Reply as the Tools answer it.
type SuggestedReplyJSON struct {
	Text     string `json:"text"`
	Language string `json:"language"`
	Kind     string `json:"kind"`
	// Answers is when the message it answers was sent.
	Answers   time.Time      `json:"answers"`
	Notes     []LanguageNote `json:"notes"`
	WrittenAt time.Time      `json:"writtenAt"`
}
