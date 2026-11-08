package model

import (
	"context"

	coremodel "github.com/feriosch/nando-server/internal/core/model"
)

// DailyLimit is the most kcal the Owner intends to consume in a Day, in force
// from EffectiveFrom until the next change. There is at most one per Day; a
// later change on the same Day replaces it.
type DailyLimit struct {
	// EffectiveFrom is the Day the change was made.
	EffectiveFrom coremodel.Day `bson:"_id"`
	Kcal          int           `bson:"kcal"`
}

// DailyLimits are every Daily Limit the Owner has set, oldest first.
type DailyLimits []DailyLimit

// On returns the kcal of the Daily Limit day had: the newest one in force
// from day or before, or the very first if day came before it. It returns nil
// if no Daily Limit was ever set.
func (limits DailyLimits) On(day coremodel.Day) *int {
	if len(limits) == 0 {
		return nil
	}
	kcal := limits[0].Kcal
	for _, limit := range limits[1:] {
		if limit.EffectiveFrom > day {
			break
		}
		kcal = limit.Kcal
	}
	return &kcal
}

// DailyLimitsRepository reads and writes the daily_limits collection.
type DailyLimitsRepository interface {
	// Save stores limit, replacing any made the same Day.
	Save(ctx context.Context, limit DailyLimit) error
	// List returns every Daily Limit, oldest first.
	List(ctx context.Context) (DailyLimits, error)
}
