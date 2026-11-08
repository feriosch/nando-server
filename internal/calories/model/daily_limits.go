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

// Latest returns the Daily Limit in force now: the newest. It returns false
// if none was ever set.
func (limits DailyLimits) Latest() (DailyLimit, bool) {
	if len(limits) == 0 {
		return DailyLimit{}, false
	}
	return limits[len(limits)-1], true
}

// Calories returns day's Consumed Calories (nil for an Unlogged Day) against
// the Daily Limit it had.
func (limits DailyLimits) Calories(day coremodel.Day, consumed *int) DayCalories {
	return DayCalories{Day: day, ConsumedCalories: consumed, DailyLimit: limits.On(day)}
}

// DayCalories is a Day's Consumed Calories against the Daily Limit it had.
type DayCalories struct {
	Day coremodel.Day
	// ConsumedCalories is nil for an Unlogged Day, which isn't 0 kcal.
	ConsumedCalories *int
	// DailyLimit is nil if none was ever set.
	DailyLimit *int
}

// Remaining is what's left of the Daily Limit, negative once over. It is nil
// for an Unlogged Day or without a Daily Limit.
func (d DayCalories) Remaining() *int {
	if d.ConsumedCalories == nil || d.DailyLimit == nil {
		return nil
	}
	remaining := *d.DailyLimit - *d.ConsumedCalories
	return &remaining
}

// OverLimit reports whether the Day went over its Daily Limit; reaching it
// isn't over. It is false without a Daily Limit, and nil for an Unlogged Day,
// which is never over or under it.
func (d DayCalories) OverLimit() *bool {
	if d.ConsumedCalories == nil {
		return nil
	}
	over := d.DailyLimit != nil && *d.ConsumedCalories > *d.DailyLimit
	return &over
}

// DailyLimitsRepository reads and writes the daily_limits collection.
type DailyLimitsRepository interface {
	// Save stores limit, replacing any made the same Day.
	Save(ctx context.Context, limit DailyLimit) error
	// List returns every Daily Limit, oldest first.
	List(ctx context.Context) (DailyLimits, error)
}
