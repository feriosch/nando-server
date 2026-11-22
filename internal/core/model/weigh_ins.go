package model

import "context"

// WeighIn is the Owner's weight for one week, as Core kept it until Weigh-ins
// moved to Calories (ADR 0012). Core only hands what it kept to Calories, once,
// through CoreService.WeighInsLeft.
type WeighIn struct {
	// Week is the Monday the week starts on.
	Week Day `bson:"_id"`
	// Kg is the weight to 1 decimal.
	Kg float64 `bson:"kg"`
}

// WeighInsRepository reads the weigh_ins collection Core kept until ADR 0012.
type WeighInsRepository interface {
	// List returns every Weigh-in, newest first.
	List(ctx context.Context) ([]WeighIn, error)
}

// WeighInsLeft is what Core kept of Weigh-ins until they moved to Calories
// (ADR 0012): the Weigh-ins, and the Weigh-in reminder, nil if the Owner
// never saved one.
type WeighInsLeft struct {
	WeighIns []WeighIn
	Reminder *WeighInReminder
}
