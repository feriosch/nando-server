package model

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Assistant is Claude in one of the Owner's chats in the Messenger, looking
// after one Module, or Core for the Core Assistant (ADR 0008). Core knows
// which Assistants there are, but nothing of what they do.
type Assistant struct {
	// Name is short and unique, such as career, and names the Assistant's
	// webhook and bot token.
	Name string
}

// CoreAssistant is the name of the Assistant for Core, and for anything that
// belongs to no Module.
const CoreAssistant = "core"

// MessageFrom says who wrote a message in a Talk.
type MessageFrom string

const (
	FromOwner     MessageFrom = "owner"
	FromAssistant MessageFrom = "assistant"
)

// MessageState is where one of the Owner's messages stands.
type MessageState string

const (
	// MessageWaiting is a message no reply has answered yet.
	MessageWaiting MessageState = "waiting"
	// MessageAnswered is a message a reply has answered.
	MessageAnswered MessageState = "answered"
	// MessageFailed is a message MaxClaims runs claimed without answering.
	MessageFailed MessageState = "failed"
)

// Photo is one photo the Owner sent, as Telegram keeps it.
type Photo struct {
	// ID is Telegram's file_id for the size Nando chose: the bot that got
	// the photo downloads it by this ID.
	ID     string `bson:"id"`
	Width  int    `bson:"width"`
	Height int    `bson:"height"`
}

// File is one file the Owner sent, such as a PDF, as Telegram keeps it.
type File struct {
	// ID is Telegram's file_id: the bot that got the file downloads it by
	// this ID.
	ID   string `bson:"id"`
	Name string `bson:"name"`
	// Size is in bytes, as Telegram reported it.
	Size int64 `bson:"size"`
}

// TalkMessage is one message in a Talk.
type TalkMessage struct {
	From MessageFrom `bson:"from"`
	At   time.Time   `bson:"at"`
	Text string      `bson:"text"`
	// Photos are the message's photos. Nando keeps only Telegram's IDs for
	// them, never the files.
	Photos []Photo `bson:"photos,omitempty"`
	// Files are the message's files, such as a PDF. Nando keeps only
	// Telegram's IDs for them, never the files.
	Files []File `bson:"files,omitempty"`
	// Note is what the run that wrote this reply left for later runs, such as
	// the ID of a draft the Owner was shown (ADR 0009). It is never sent to
	// the Owner. Set only on the first message of an Assistant's reply.
	Note string `bson:"note,omitempty"`
	// TelegramMessageID is the message's id in the Owner's chat with the bot.
	TelegramMessageID int64 `bson:"telegramMessageId"`
	// State is set only for the Owner's messages.
	State MessageState `bson:"state,omitempty"`
}

// Talk is the running exchange between the Owner and one Assistant.
type Talk struct {
	ID bson.ObjectID `bson:"_id,omitempty"`
	// Version counts the Talk's changes, so that a change made from what was
	// read is saved only if nothing changed the Talk since.
	Version   int    `bson:"version,omitempty"`
	Assistant string `bson:"assistant"`
	// Open is true for each Assistant's one open Talk.
	Open bool `bson:"open"`
	// Summary is what the Talk opens with when Claude started it in place of
	// one that grew too long; empty otherwise.
	Summary  string        `bson:"summary,omitempty"`
	Messages []TalkMessage `bson:"messages"`
	// ClaimedUntil is when a run's claim on the Talk lapses; zero if none
	// ever claimed it.
	ClaimedUntil time.Time `bson:"claimedUntil,omitempty"`
	// Claims counts the claims since the Talk's last reply.
	Claims int `bson:"claims,omitempty"`
	// ClaimedUpTo is the Telegram message ID of the Owner's last message when
	// the Talk was last claimed: a reply answers the messages up to it.
	ClaimedUpTo int64 `bson:"claimedUpTo,omitempty"`
	// NoticedFor is the Telegram message ID of the oldest waiting message
	// when the Owner was last told that no run started.
	NoticedFor int64 `bson:"noticedFor,omitempty"`
}

// LastOwnerMessageID is the Telegram message ID of the Owner's latest message
// in the Talk, or 0.
func (t Talk) LastOwnerMessageID() int64 {
	var id int64
	for _, m := range t.Messages {
		if m.From == FromOwner {
			id = max(id, m.TelegramMessageID)
		}
	}
	return id
}

// OldestWaiting is the Talk's oldest waiting message, and false if none waits.
func (t Talk) OldestWaiting() (TalkMessage, bool) {
	for _, m := range t.Messages {
		if m.State == MessageWaiting {
			return m, true
		}
	}
	return TalkMessage{}, false
}

// Waiting reports whether some message in the Talk waits for a reply.
func (t Talk) Waiting() bool {
	for _, m := range t.Messages {
		if m.State == MessageWaiting {
			return true
		}
	}
	return false
}

// MaxClaims is how many claims a Talk's waiting messages get without a reply
// before they fail.
const MaxClaims = 3

// Claimable reports whether a run may claim the Talk at now: some message
// waits, no claim holds it, and it has claims left.
func (t Talk) Claimable(now time.Time) bool {
	return t.Waiting() && !t.ClaimedUntil.After(now) && t.Claims < MaxClaims
}

// ErrTalkChanged is a Talk changed by someone else since it was read: read it
// again and redo the change.
var ErrTalkChanged = errors.New("the Talk changed since it was read")

// TalksRepository reads and writes the talks collection, and remembers which
// of Telegram's updates each bot has had.
type TalksRepository interface {
	// Open is assistant's open Talk, or an empty one if it has none yet.
	Open(ctx context.Context, assistant string) (Talk, error)
	// AddMessage adds m to assistant's open Talk, opening one if there
	// is none.
	AddMessage(ctx context.Context, assistant string, m TalkMessage) error
	// Save replaces talk, an open Talk read earlier, with its Version one
	// higher. It returns ErrTalkChanged if the Talk changed since it was read.
	Save(ctx context.Context, talk Talk) error
	// Insert stores talk, a new open Talk. It returns ErrTalkChanged if its
	// Assistant has an open Talk already.
	Insert(ctx context.Context, talk Talk) error
	// MarkUpdate remembers, until expiresAt, that assistant's bot had the
	// update with updateID. It reports false if it already had it, as when
	// Telegram re-delivers an update.
	MarkUpdate(ctx context.Context, assistant string, updateID int64, expiresAt time.Time) (bool, error)
	// UnmarkUpdate forgets that assistant's bot had the update, so that
	// Telegram's re-delivery of one Nando failed to keep is kept then.
	UnmarkUpdate(ctx context.Context, assistant string, updateID int64) error
}
