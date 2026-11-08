// Package model declares the Calories Module's data and the repository that
// stores it. The Module's service depends on it, never on the repository
// implementation.
package model

import (
	"context"
	"errors"
	"time"

	coremodel "github.com/feriosch/nando-server/internal/core/model"
)

// ErrNotFound means the Meal or Entry asked for doesn't exist.
var ErrNotFound = errors.New("not found")

// CaloriesRepository reads and writes the Calories Module's data.
type CaloriesRepository interface {
	// InsertMeal stores meal under a new ID, which it returns.
	InsertMeal(ctx context.Context, meal Meal) (string, error)
	// ListMeals returns the Meals that are archived, or those that aren't.
	ListMeals(ctx context.Context, archived bool) ([]Meal, error)
	FindMeal(ctx context.Context, id string) (Meal, error)
	// UpdateMeal replaces what the Owner can edit of the Meal with meal's ID:
	// everything but whether it is archived. ErrNotFound if there's none.
	UpdateMeal(ctx context.Context, meal Meal) error
	// SetMealArchivedAt archives the Meal with this ID at archivedAt, or
	// restores it if archivedAt is nil. ErrNotFound if there's none.
	SetMealArchivedAt(ctx context.Context, id string, archivedAt *time.Time) error

	// InsertEntry stores entry under a new ID, which it returns.
	InsertEntry(ctx context.Context, entry Entry) (string, error)
	// ListEntries returns day's Entries in the order they were logged.
	ListEntries(ctx context.Context, day coremodel.Day) ([]Entry, error)
	FindEntry(ctx context.Context, id string) (Entry, error)
	// UpdateEntry replaces the name, kcal per serving and Servings of the
	// Entry with entry's ID. ErrNotFound if there's none.
	UpdateEntry(ctx context.Context, entry Entry) error
	DeleteEntry(ctx context.Context, id string) error
	// CountEntriesByMeal returns how many Entries each Meal has on the Days
	// from first to last, inclusive, by Meal ID. One-off Entries aren't counted.
	CountEntriesByMeal(ctx context.Context, first, last coremodel.Day) (map[string]int, error)
}
