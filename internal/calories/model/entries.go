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

// EntriesRepository reads and writes the entries collection.
type EntriesRepository interface {
	// Insert stores entry under a new ID, which it returns.
	Insert(ctx context.Context, entry Entry) (string, error)
	// ListOnDay returns day's Entries in the order they were logged.
	ListOnDay(ctx context.Context, day coremodel.Day) ([]Entry, error)
	Find(ctx context.Context, id string) (Entry, error)
	// Update replaces the name, kcal per serving and Servings of the Entry
	// with entry's ID. Core's ErrNotFound if there's none.
	Update(ctx context.Context, entry Entry) error
	Delete(ctx context.Context, id string) error
	// CountByMeal returns how many Entries each Meal has on the Days from
	// first to last, inclusive, by Meal ID. One-off Entries aren't counted.
	CountByMeal(ctx context.Context, first, last coremodel.Day) (map[string]int, error)
}
