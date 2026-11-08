package model

import (
	"context"
	"time"
)

// CoreService is what other Modules' services may ask of Core. Modules read
// Core's data only through it, never from Core's collections.
type CoreService interface {
	// OwnerTimeZone is the time zone all of Nando's dates and clock times are in.
	OwnerTimeZone(ctx context.Context) (*time.Location, error)
	// Today is the current Day in the Owner Time Zone.
	Today(ctx context.Context) (Day, error)
	// LatestWeighIn is the Weigh-in for the newest week that has one, or
	// utils.ErrNotFound if there are none.
	LatestWeighIn(ctx context.Context) (WeighIn, error)
}
