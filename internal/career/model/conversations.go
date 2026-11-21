package model

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Conversation is one thread between the Owner and one other person, on
// LinkedIn or in Gmail (ADR 0010), with the copy of its messages that LinkedIn
// Syncs or Gmail Career Syncs brought. Until it is filed under a Recruiter it
// is unsorted.
type Conversation struct {
	ID bson.ObjectID `bson:"_id,omitempty"`
	// Channel is ChannelGmail for a Gmail thread, and empty for a LinkedIn
	// one, as every Conversation was before Gmail.
	Channel string `bson:"channel,omitempty"`
	// ThreadID is LinkedIn's or Gmail's id for the thread. The two never
	// look alike, so one unique index holds both.
	ThreadID string `bson:"threadId"`
	// Subject is a Gmail thread's subject; LinkedIn threads have none.
	Subject  string    `bson:"subject,omitempty"`
	URL      string    `bson:"url"`
	Person   Person    `bson:"person"`
	Messages []Message `bson:"messages"`
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
// than the message it answers. Until then, the Owner may mark it replied.
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
	// MarkedRepliedAt is when the Owner marked it replied, having sent it
	// in LinkedIn before a Sync brought their message, if they did.
	MarkedRepliedAt time.Time `bson:"markedRepliedAt,omitempty"`
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

// WaitingReplyKinds are the kinds of Suggested Reply a Recruiter waits on:
// they can also turn late.
var WaitingReplyKinds = []string{ReplyAnswer, ReplyFollowUp}

// MaxLanguageNotes is the most language notes a Suggested Reply has.
const MaxLanguageNotes = 3

// Person is the other side of a thread, as LinkedIn or Gmail names them.
type Person struct {
	Name       string `bson:"name"`
	ProfileURL string `bson:"profileUrl"`
	// Email is a Gmail thread's sender's address.
	Email string `bson:"email,omitempty"`
}

// The channels a Conversation can be on. A LinkedIn one stores no channel.
const (
	ChannelLinkedIn = "linkedin"
	ChannelGmail    = "gmail"
)

// Message is one message in a Conversation.
type Message struct {
	// LinkedInID is LinkedIn's id for the message, if the reader gave one.
	LinkedInID string `bson:"linkedInId,omitempty"`
	// GmailID is Gmail's id for a message in a Gmail thread.
	GmailID string `bson:"gmailId,omitempty"`
	// Attachments are the names of a Gmail message's attachments. Nando
	// keeps only their names (ADR 0010).
	Attachments []string `bson:"attachments,omitempty"`
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

// ThreadUpdate is what one LinkedIn Sync brought for one LinkedIn thread, or a
// Gmail Career Sync for one Gmail thread.
type ThreadUpdate struct {
	// Channel is ChannelGmail for a Gmail thread, else empty.
	Channel  string
	Subject  string
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
	// RecruiterID is the Recruiter the Conversation is filed under, or nil.
	RecruiterID *bson.ObjectID
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
	// WithSuggestedReply returns the filed Conversations whose Suggested
	// Reply is of one of kinds and not marked replied.
	WithSuggestedReply(ctx context.Context, kinds []string) ([]Conversation, error)
	// SetSuggestedReply replaces the Conversation's Suggested Reply, unless
	// it is ignored, and returns it, or Core's utils.ErrNotFound.
	SetSuggestedReply(ctx context.Context, id bson.ObjectID, reply SuggestedReply) (Conversation, error)
	// Threads returns every LinkedIn Conversation's thread id, newest message
	// time and whether it is ignored, without its messages.
	Threads(ctx context.Context) ([]Conversation, error)
	// WithWaitingReplyBefore returns the Conversations whose Suggested Reply
	// is of one of WaitingReplyKinds, not marked replied, and answers a
	// message sent before before.
	WithWaitingReplyBefore(ctx context.Context, before time.Time) ([]Conversation, error)
	// AnyAnswerFromSync reports whether a Conversation's Suggested Reply is
	// an answer to a message the Sync with syncID brought, not marked
	// replied.
	AnyAnswerFromSync(ctx context.Context, syncID bson.ObjectID) (bool, error)
	// WaitingRecruiters counts the Recruiters waiting for the Owner's reply:
	// those with a Conversation whose Suggested Reply is of one of
	// WaitingReplyKinds and answers a message sent at or after since. A Late
	// Reply isn't counted, nor one about to turn late, nor one marked
	// replied.
	WaitingRecruiters(ctx context.Context, since time.Time) (int, error)
	// MarkFollowedUp records that a follow_up Item was queued for the
	// Owner's message sent at sentAt.
	MarkFollowedUp(ctx context.Context, id bson.ObjectID, sentAt time.Time) error
	// MakeReplyLate turns the Conversation's Suggested Reply late, unless it
	// changed since it was read as reply or the Owner has since marked it
	// replied. It reports whether it did.
	MakeReplyLate(ctx context.Context, id bson.ObjectID, reply SuggestedReply) (bool, error)
	// DropReply removes the Conversation's Suggested Reply, unless it
	// changed since it was read as reply or the Owner has since marked it
	// replied. It reports whether it did.
	DropReply(ctx context.Context, id bson.ObjectID, reply SuggestedReply) (bool, error)
	// MarkReplied marks the Conversation's Suggested Reply replied at at,
	// unless it is already marked or it changed since it was read as reply.
	MarkReplied(ctx context.Context, id bson.ObjectID, reply SuggestedReply, at time.Time) error
	// UnmarkReplied clears the mark on the Conversation's Suggested Reply,
	// if any.
	UnmarkReplied(ctx context.Context, id bson.ObjectID) error
}

// ConversationJSON is a Conversation as the Tools answer it.
type ConversationJSON struct {
	ID string `json:"id"`
	// Channel is linkedin or gmail.
	Channel  string        `json:"channel"`
	ThreadID string        `json:"threadId"`
	Subject  string        `json:"subject,omitempty"`
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
	// MarkedRepliedAt is when the Owner marked it replied, or null: they
	// sent it in LinkedIn, and no Sync has brought their message yet.
	MarkedRepliedAt *time.Time `json:"markedRepliedAt"`
}

// RecruiterPageConversationJSON is a Conversation as its Recruiter's page
// shows it: its LinkedIn URL, its last messages, oldest first, and its
// Suggested Reply.
type RecruiterPageConversationJSON struct {
	ID string `json:"id"`
	// Channel is linkedin or gmail, and Subject a Gmail thread's subject.
	Channel  string        `json:"channel"`
	Subject  string        `json:"subject,omitempty"`
	URL      string        `json:"url"`
	Messages []MessageJSON `json:"messages"`
	// SuggestedReply is null when there is none.
	SuggestedReply *RecruiterPageReplyJSON `json:"suggestedReply"`
}

// RecruiterPageReplyJSON is a Suggested Reply as a Recruiter's page shows it.
type RecruiterPageReplyJSON struct {
	Text     string         `json:"text"`
	Language string         `json:"language"`
	Kind     string         `json:"kind"`
	Notes    []LanguageNote `json:"notes"`
	// MarkedRepliedAt is when the Owner marked it replied, or null.
	MarkedRepliedAt *time.Time `json:"markedRepliedAt"`
}
