// Package model declares Core's data, one repository interface per
// collection, and the service other Modules call. Services depend on it,
// never on implementations.
package model

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned by Core's repositories when there's nothing to
// return, including when what was asked for has expired. Modules' repositories
// return it too.
var ErrNotFound = errors.New("not found")

// CoreService is what other Modules' services may ask of Core. Modules read
// Core's data only through it, never from Core's collections.
type CoreService interface {
	// OwnerTimeZone is the time zone all of Nando's dates and clock times are in.
	OwnerTimeZone(ctx context.Context) (*time.Location, error)
	// Today is the current Day in the Owner Time Zone.
	Today(ctx context.Context) (Day, error)
	// LatestWeighIn is the Weigh-in for the newest week that has one, or
	// ErrNotFound if there are none.
	LatestWeighIn(ctx context.Context) (WeighIn, error)
}
