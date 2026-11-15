package model

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Item is one thing the cloud run has to do for a Conversation, such as
// sorting it. It waits until a run claims it, and stays open until the run
// does it; a claim that lapses lets another run take it.
type Item struct {
	ID             bson.ObjectID `bson:"_id,omitempty"`
	Kind           string        `bson:"kind"`
	ConversationID bson.ObjectID `bson:"conversationId"`
	// State is ItemWaiting, ItemClaimed, ItemDone or ItemFailed.
	State string `bson:"state"`
	// Open is true while the item is waiting or claimed: a Conversation has
	// at most one open item of each kind.
	Open         bool      `bson:"open"`
	ClaimedUntil time.Time `bson:"claimedUntil,omitempty"`
	// Claims is how many times a run claimed it.
	Claims    int       `bson:"claims"`
	CreatedAt time.Time `bson:"createdAt"`
	UpdatedAt time.Time `bson:"updatedAt"`
}

// The kinds of Item.
const (
	// ItemSort is an unsorted Conversation: the run ignores it or files it
	// under a Recruiter.
	ItemSort = "sort"
	// ItemReply is a filed Conversation with a new message from its
	// Recruiter: the run updates its Opportunities and writes the Owner's
	// answer.
	ItemReply = "reply"
	// ItemLateReply is a Suggested Reply turned late, to rewrite from the
	// late templates. The tick's rules queue it.
	ItemLateReply = "late_reply"
	// ItemFollowUp is an open Opportunity silent for 14 days after the
	// Owner's message: the run writes a follow-up. The tick's rules queue
	// it.
	ItemFollowUp = "follow_up"
)

// ItemKinds are every kind of Item.
var ItemKinds = []string{ItemSort, ItemReply, ItemLateReply, ItemFollowUp}

// The states of an Item.
const (
	ItemWaiting = "waiting"
	ItemClaimed = "claimed"
	ItemDone    = "done"
	ItemFailed  = "failed"
)

// WorkRepository reads and writes the work collection: the run's Items.
type WorkRepository interface {
	// Add puts an Item of kind for the Conversation in the work at now,
	// unless one is open already.
	Add(ctx context.Context, kind string, conversationID bson.ObjectID, now time.Time) error
	// ClaimNext claims the oldest claimable Item until claimedUntil, or
	// returns Core's utils.ErrNotFound if there is none. An Item is
	// claimable while it waits, or when its claim lapsed by now after fewer
	// than maxClaims claims.
	ClaimNext(ctx context.Context, now, claimedUntil time.Time, maxClaims int) (Item, error)
	// FailUnfinished sets aside as failed, at now, every Item whose last
	// claim lapsed after maxClaims claims, and returns them.
	FailUnfinished(ctx context.Context, now time.Time, maxClaims int) ([]Item, error)
	// AnyClaimable reports whether ClaimNext would claim an Item.
	AnyClaimable(ctx context.Context, now time.Time, maxClaims int) (bool, error)
	// AnyClaimed reports whether a run holds an Item: one is claimed until
	// after now.
	AnyClaimed(ctx context.Context, now time.Time) (bool, error)
	// Complete marks done, at now, the Conversation's open Items of kinds.
	Complete(ctx context.Context, conversationID bson.ObjectID, kinds []string, now time.Time) error
}

// The JSON shapes below are what the work Tools answer with.

// WorkJSON is what career_list_work answers: the Items this run claimed, and
// what every Item needs.
type WorkJSON struct {
	Items []ItemJSON `json:"items"`
	// Looking is whether the Owner is Looking.
	Looking bool `json:"looking"`
	// Profile is the Career Profile, as Markdown.
	Profile string `json:"profile"`
	// LiveSince is the time of the first Sync. Nothing before it becomes an
	// Opportunity.
	LiveSince *time.Time `json:"liveSince"`
}

// ItemJSON is one claimed Item, with its Conversation and, if it is filed,
// its Recruiter.
type ItemJSON struct {
	ID           string           `json:"id"`
	Kind         string           `json:"kind"`
	Conversation ConversationJSON `json:"conversation"`
	// Recruiter is null while the Conversation isn't filed.
	Recruiter *RecruiterWithWorkJSON `json:"recruiter"`
}

// RecruiterWithWorkJSON is the Recruiter of an Item's Conversation, with
// their other Conversations and their Opportunities.
type RecruiterWithWorkJSON struct {
	RecruiterJSON
	Conversations []ConversationJSON `json:"conversations"`
	Opportunities []OpportunityJSON  `json:"opportunities"`
}
