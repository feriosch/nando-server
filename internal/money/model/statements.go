package model

import (
	"context"
	"fmt"
)

// Statement is one record of an Account for one period that the Owner sent,
// with its Transactions in it, so that saving or replacing one is a single
// write (ADR 0014).
type Statement struct {
	ID        string `bson:"_id"`
	AccountID string `bson:"accountId"`
	// From and To are the Days of the period it covers, both included.
	From string `bson:"from"`
	To   string `bson:"to"`
	// Source is what it was read from.
	Source Source `bson:"source"`
	// FileIDs are the Telegram file_ids it was read from.
	FileIDs []string `bson:"fileIds"`
	// Total is the total it states, such as a card's billed amount, and
	// ClosingBalance its balance at To; each nil when it states none. In the
	// Account's minor unit (yen, or centavos).
	Total          *int64        `bson:"total"`
	ClosingBalance *int64        `bson:"closingBalance"`
	Transactions   []Transaction `bson:"transactions"`
}

// Source is what kind of file a Statement came from.
type Source string

// Sources are every Source, as the Tools name them.
var Sources = []Source{"csv", "pdf", "screenshots"}

// Transaction is one line of a Statement, counted in the month of its Day.
type Transaction struct {
	ID   string          `bson:"id"`
	Kind TransactionKind `bson:"kind"`
	// Day is the purchase date, which decides its month.
	Day string `bson:"day"`
	// Amount is in the Account's minor unit (yen, or centavos): positive,
	// but negative for a refund, an Expense coming back.
	Amount int64 `bson:"amount"`
	// Direction is, for a Transfer only, whether the money came "in" to the
	// Account or went "out" of it.
	Direction string `bson:"direction,omitempty"`
	// RawText is the Statement's text for it, exactly as written.
	RawText string `bson:"rawText"`
	// Note is what was bought, if the Owner said.
	Note string `bson:"note"`
	// OtherAccountID is, for a Transfer, the Owner's other Account, when
	// known.
	OtherAccountID string `bson:"otherAccountId,omitempty"`
	// Duplicate marks a late line another Statement of the Account already
	// counts. It is kept, but left out of every read, so that it can count
	// instead if that copy goes, with its Statement deleted or sent again
	// without it.
	Duplicate bool `bson:"duplicate,omitempty"`
}

// Key is what tells two copies of one Transaction on two Statements apart
// from two Transactions: its Day, amount and raw text.
func (t Transaction) Key() string {
	return fmt.Sprintf("%s|%d|%s", t.Day, t.Amount, t.RawText)
}

// TransactionKind is whether a Transaction is spending, income, or money
// moving between two of the Owner's Accounts.
type TransactionKind string

const (
	Expense  TransactionKind = "expense"
	Income   TransactionKind = "income"
	Transfer TransactionKind = "transfer"
)

// TransactionKinds are every TransactionKind, as the Tools name them.
var TransactionKinds = []TransactionKind{Expense, Income, Transfer}

// TransactionFilter picks Transactions; an empty field picks any.
type TransactionFilter struct {
	AccountID string
	// FromMonth and ToMonth are the months of their Days, both included.
	FromMonth, ToMonth string
	Kind               TransactionKind
	// Text is a word the raw text or note contains, in any case.
	Text  string
	Limit int
}

// FoundTransaction is a Transaction with the Statement and Account it is in.
type FoundTransaction struct {
	Transaction `bson:"transaction"`
	StatementID string `bson:"statementId"`
	AccountID   string `bson:"accountId"`
}

// StatementsRepository reads and writes the statements collection.
type StatementsRepository interface {
	// Save inserts statement, giving it and its Transactions IDs, or
	// replaces the Account's Statement for the same period, in one write.
	Save(ctx context.Context, statement Statement) (Statement, error)
	// Find returns the Statement with id, or Core's utils.ErrNotFound.
	Find(ctx context.Context, id string) (Statement, error)
	// ForAccount returns the Account's Statements, by period, without their
	// Transactions, that end on or after fromDay and start on or before
	// toDay; an empty Day leaves that end open.
	ForAccount(ctx context.Context, accountID, fromDay, toDay string) ([]Statement, error)
	// Delete removes the Statement with id and its Transactions, or returns
	// Core's utils.ErrNotFound.
	Delete(ctx context.Context, id string) error
	AccountHasStatements(ctx context.Context, accountID string) (bool, error)
	// HasTransaction is whether a Statement of the Account other than
	// exceptStatementID already counts a Transaction on day, of amount, with
	// rawText: one that isn't a Duplicate.
	HasTransaction(ctx context.Context, accountID, exceptStatementID, day string, amount int64, rawText string) (bool, error)
	// Duplicates returns the Duplicate Transactions of the Account's
	// Statements other than exceptStatementID.
	Duplicates(ctx context.Context, accountID, exceptStatementID string) ([]FoundTransaction, error)
	// Count makes the Duplicate Transaction with id count.
	Count(ctx context.Context, id string) error
	// Transactions returns the Transactions filter picks, by Day, leaving
	// Duplicates out.
	Transactions(ctx context.Context, filter TransactionFilter) ([]FoundTransaction, error)
	// UpdateTransaction replaces the Transaction with t's ID, or returns
	// Core's utils.ErrNotFound.
	UpdateTransaction(ctx context.Context, t Transaction) error
	// FindTransaction returns the Transaction with id, or Core's
	// utils.ErrNotFound.
	FindTransaction(ctx context.Context, id string) (FoundTransaction, error)
}
