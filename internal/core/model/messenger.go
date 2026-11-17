package model

import (
	"context"
	"time"
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
)

// TalkMessage is one message in a Talk.
type TalkMessage struct {
	From MessageFrom `bson:"from"`
	At   time.Time   `bson:"at"`
	Text string      `bson:"text"`
	// TelegramMessageID is the message's id in the Owner's chat with the bot.
	TelegramMessageID int64 `bson:"telegramMessageId"`
	// State is set only for the Owner's messages.
	State MessageState `bson:"state,omitempty"`
}

// Talk is the running exchange between the Owner and one Assistant.
type Talk struct {
	Assistant string `bson:"assistant"`
	// Summary is what the Talk opens with when Claude started it in place of
	// one that grew too long; empty otherwise.
	Summary  string        `bson:"summary,omitempty"`
	Messages []TalkMessage `bson:"messages"`
}

// TalksRepository reads and writes the talks collection, and remembers which
// of Telegram's updates each bot has had.
type TalksRepository interface {
	// Open is assistant's open Talk, or an empty one if it has none yet.
	Open(ctx context.Context, assistant string) (Talk, error)
	// AddOwnerMessage adds m to assistant's open Talk, opening one if there
	// is none.
	AddOwnerMessage(ctx context.Context, assistant string, m TalkMessage) error
	// MarkUpdate remembers, until expiresAt, that assistant's bot had the
	// update with updateID. It reports false if it already had it, as when
	// Telegram re-delivers an update.
	MarkUpdate(ctx context.Context, assistant string, updateID int64, expiresAt time.Time) (bool, error)
	// UnmarkUpdate forgets that assistant's bot had the update, so that
	// Telegram's re-delivery of one Nando failed to keep is kept then.
	UnmarkUpdate(ctx context.Context, assistant string, updateID int64) error
}
