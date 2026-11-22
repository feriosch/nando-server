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
	// Now is the current time on Core's clock, which tests set by hand.
	Now() time.Time
	// HeightCm is the Owner's height in cm, or 0 until it is set.
	HeightCm(ctx context.Context) (int, error)
	// WeighInsLeft is what Core kept of Weigh-ins until they moved to
	// Calories (ADR 0012), for Calories to take once at startup.
	WeighInsLeft(ctx context.Context) (WeighInsLeft, error)
}
