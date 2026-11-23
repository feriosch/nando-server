package model

import "context"

// Account is one place the Owner's money sits or is spent from, in one
// currency (ADR 0014). It is the Owner's data, never named in the code.
type Account struct {
	ID       string `bson:"_id"`
	Name     string `bson:"name"`
	Kind     Kind   `bson:"kind"`
	Currency string `bson:"currency"`
	// Institution is the bank or card's issuer, if the Owner said.
	Institution string `bson:"institution"`
	// Last4 is at most the last 4 digits, to tell two Accounts apart; never
	// a full card or account number.
	Last4 string `bson:"last4"`
	// Note is what Claude learned about the Account: its export format, its
	// billing cycle.
	Note string `bson:"note"`
	// TrackedFrom is the first month the Account is tracked in, such as
	// "2026-01".
	TrackedFrom string `bson:"trackedFrom"`
	// ClosedFrom is the first month the Account is no longer tracked in, or
	// empty while it is open.
	ClosedFrom string `bson:"closedFrom"`
	// HoldsMonth is whether a month waits for this Account's Statements to be
	// Complete: false for a card whose charge never changes and that closes
	// mid-month, such as one that only pays the rent (ADR 0014).
	HoldsMonth bool `bson:"holdsMonth"`
}

// Kind is what an Account is.
type Kind string

const (
	KindBank       Kind = "bank"
	KindCard       Kind = "card"
	KindSavings    Kind = "savings"
	KindInvestment Kind = "investment"
)

// Kinds are every Kind, as the Tools name them.
var Kinds = []Kind{KindBank, KindCard, KindSavings, KindInvestment}

// Currencies are the Owner's currencies: every Account is in one.
var Currencies = []string{"JPY", "MXN"}

// TracksIn is whether the Account is tracked in month.
func (a Account) TracksIn(month string) bool {
	return a.TrackedFrom <= month && (a.ClosedFrom == "" || month < a.ClosedFrom)
}

// AccountsRepository reads and writes the accounts collection.
type AccountsRepository interface {
	// Insert saves a new Account, and returns its ID.
	Insert(ctx context.Context, account Account) (string, error)
	// Find returns the Account with id, or Core's utils.ErrNotFound.
	Find(ctx context.Context, id string) (Account, error)
	// List returns every Account, by name.
	List(ctx context.Context) ([]Account, error)
	Replace(ctx context.Context, account Account) error
}

// The JSON shapes below are what the Tools answer with and are sent.

// AccountJSON is an Account as the Tools read and write it. A field left out
// of a save is nil, and keeps its saved value.
type AccountJSON struct {
	ID          *string `json:"id"`
	Name        *string `json:"name"`
	Kind        *string `json:"kind"`
	Currency    *string `json:"currency"`
	Institution *string `json:"institution"`
	Last4       *string `json:"last4"`
	Note        *string `json:"note"`
	TrackedFrom *string `json:"trackedFrom"`
	ClosedFrom  *string `json:"closedFrom"`
	HoldsMonth  *bool   `json:"holdsMonth"`
}
