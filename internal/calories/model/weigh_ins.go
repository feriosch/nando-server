package model

import (
	"context"

	coremodel "github.com/feriosch/nando-server/internal/core/model"
)

// WeighIn is the Owner's weight for one week, Monday to Sunday in the Owner
// Time Zone. There is at most one per week; a later one replaces it.
type WeighIn struct {
	// Week is the Monday the week starts on.
	Week coremodel.Day `bson:"_id"`
	// Kg is the weight to 1 decimal.
	Kg float64 `bson:"kg"`
}

// WeighInsRepository reads and writes the weigh_ins collection.
type WeighInsRepository interface {
	// Save stores weighIn, replacing any for the same week.
	Save(ctx context.Context, weighIn WeighIn) error
	// Find returns the Weigh-in for the week starting on Monday week.
	Find(ctx context.Context, week coremodel.Day) (WeighIn, error)
	// FindLatest returns the newest week's Weigh-in.
	FindLatest(ctx context.Context) (WeighIn, error)
	// List returns every Weigh-in, newest first.
	List(ctx context.Context) ([]WeighIn, error)
	Delete(ctx context.Context, week coremodel.Day) error
}

// The JSON shapes below are what the API answers with.

// WeighInJSON is a Weigh-in as the API shows it.
type WeighInJSON struct {
	Week coremodel.Day `json:"week"`
	Kg   float64       `json:"kg"`
}

// WeightCardJSON is the Weight Home Card: the latest Weigh-in, its BMI (once
// the Owner's height is set in Core) and its change since the week before
// (if that week has one). Each is null without it.
type WeightCardJSON struct {
	Latest *WeighInJSON `json:"latest"`
	BMI    *float64     `json:"bmi"`
	Change *float64     `json:"change"`
}

// WeighInsJSON is what the Weigh-in Tools answer: every Weigh-in, newest
// first, with the Weight Home Card's BMI and change of the latest.
type WeighInsJSON struct {
	WeighIns []WeighInJSON `json:"weighIns"`
	WeightCardJSON
}
