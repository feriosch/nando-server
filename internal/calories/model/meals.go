package model

import (
	"context"
	"time"
)

// Meal is something the Owner eats repeatedly, saved so it can be logged in
// one tap. It isn't tied to any Day.
type Meal struct {
	ID   string `bson:"_id"`
	Name string `bson:"name"`
	// Kcal is per serving.
	Kcal int `bson:"kcal"`
	// Store, Emoji and ImageURL are "" when not set.
	Store    string   `bson:"store"`
	Emoji    string   `bson:"emoji"`
	ImageURL string   `bson:"imageUrl"`
	Tags     []string `bson:"tags"`
	Pinned   bool     `bson:"pinned"`
	// ArchivedAt is when the Owner deleted it; nil while it is in use.
	ArchivedAt *time.Time `bson:"archivedAt"`
	CreatedAt  time.Time  `bson:"createdAt"`
}

// MealsRepository reads and writes the meals collection.
type MealsRepository interface {
	// Insert stores meal under a new ID, which it returns.
	Insert(ctx context.Context, meal Meal) (string, error)
	// List returns the Meals that are archived, or those that aren't, by name.
	List(ctx context.Context, archived bool) ([]Meal, error)
	Find(ctx context.Context, id string) (Meal, error)
	// Update replaces what the Owner can edit of the Meal with meal's ID:
	// everything but whether it is archived. Core's utils.ErrNotFound if there's none.
	Update(ctx context.Context, meal Meal) error
	// SetArchivedAt archives the Meal with this ID at archivedAt, or restores
	// it if archivedAt is nil. Core's utils.ErrNotFound if there's none.
	SetArchivedAt(ctx context.Context, id string, archivedAt *time.Time) error
}
