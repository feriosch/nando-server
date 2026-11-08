package model

import (
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
