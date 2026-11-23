package model

import "context"

// Rate is the one exchange rate Nando uses for a month, pesos in yen
// (ADR 0014): the month's average, for Transactions, and its last day's, for
// balances. Once stored, it never changes.
type Rate struct {
	// Month is the month, such as "2026-10".
	Month    string  `bson:"_id"`
	Average  float64 `bson:"average"`
	MonthEnd float64 `bson:"monthEnd"`
}

// RatesRepository reads and writes the rates collection.
type RatesRepository interface {
	// Find returns month's Rate, or Core's utils.ErrNotFound.
	Find(ctx context.Context, month string) (Rate, error)
	// Insert stores a Rate, leaving one already stored as it is.
	Insert(ctx context.Context, rate Rate) error
}
