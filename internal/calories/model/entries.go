package model

import (
	"context"
	"time"

	coremodel "github.com/feriosch/nando-server/internal/core/model"
)

// Entry is one record that the Owner ate something on a Day. It keeps the
// name and kcal from when it was logged, so later changes to a Meal never
// alter it.
type Entry struct {
	ID  string        `bson:"_id"`
	Day coremodel.Day `bson:"day"`
	// MealID is the Meal it was logged from, or "" for a one-off Entry.
	MealID         string    `bson:"mealId"`
	Name           string    `bson:"name"`
	KcalPerServing int       `bson:"kcalPerServing"`
	Servings       int       `bson:"servings"`
	LoggedAt       time.Time `bson:"loggedAt"`
}

// Kcal is the Entry's kcal: its kcal per serving times its Servings.
func (e Entry) Kcal() int {
	return e.KcalPerServing * e.Servings
}

// ConsumedCalories returns the sum of the kcal of a Day's entries, or nil if
// there are none: an Unlogged Day isn't 0 kcal.
func ConsumedCalories(entries []Entry) *int {
	if len(entries) == 0 {
		return nil
	}
	kcal := 0
	for _, e := range entries {
		kcal += e.Kcal()
	}
	return &kcal
}

// EntriesRepository reads and writes the entries collection.
type EntriesRepository interface {
	// Insert stores entry under a new ID, which it returns.
	Insert(ctx context.Context, entry Entry) (string, error)
	// ListOnDay returns day's Entries in the order they were logged.
	ListOnDay(ctx context.Context, day coremodel.Day) ([]Entry, error)
	Find(ctx context.Context, id string) (Entry, error)
	// Update replaces the name, kcal per serving and Servings of the Entry
	// with entry's ID. Core's utils.ErrNotFound if there's none.
	Update(ctx context.Context, entry Entry) error
	Delete(ctx context.Context, id string) error
	// CountByMeal returns how many Entries each Meal has on the Days from
	// first to last, inclusive, by Meal ID. One-off Entries aren't counted.
	CountByMeal(ctx context.Context, first, last coremodel.Day) (map[string]int, error)
	// ConsumedByDay returns the Consumed Calories of each Day from first to
	// last, inclusive, that has Entries. An Unlogged Day isn't in it.
	ConsumedByDay(ctx context.Context, first, last coremodel.Day) (map[coremodel.Day]int, error)
}
