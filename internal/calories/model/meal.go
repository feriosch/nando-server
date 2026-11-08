package model

import "time"

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
