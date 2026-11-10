package model

import "context"

// WeighIn is the Owner's weight for one week, Monday to Sunday in the Owner
// Time Zone. There is at most one per week; a later one replaces it.
type WeighIn struct {
	// Week is the Monday the week starts on.
	Week Day `bson:"_id"`
	// Kg is the weight to 1 decimal.
	Kg float64 `bson:"kg"`
}

// WeighInsRepository reads and writes the weigh_ins collection.
type WeighInsRepository interface {
	// Save stores weighIn, replacing any for the same week.
	Save(ctx context.Context, weighIn WeighIn) error
	// Find returns the Weigh-in for the week starting on Monday week.
	Find(ctx context.Context, week Day) (WeighIn, error)
	// FindLatest returns the newest week's Weigh-in.
	FindLatest(ctx context.Context) (WeighIn, error)
	// List returns every Weigh-in, newest first.
	List(ctx context.Context) ([]WeighIn, error)
	Delete(ctx context.Context, week Day) error
}

// WeighInJSON is a Weigh-in as the API shows it.
type WeighInJSON struct {
	Week Day     `json:"week"`
	Kg   float64 `json:"kg"`
}
