// Package model declares Core's data, the repository that stores it, and the
// service other Modules call. Services depend on it, never on implementations.
package model

import (
	"context"
	"time"
)

// CoreRepository reads and writes Core's data. Only Core's service uses it.
type CoreRepository interface {
	InsertEnrollmentCode(ctx context.Context, code EnrollmentCode) error
	// FindEnrollmentCode returns the code with this hash that is unexpired at now.
	FindEnrollmentCode(ctx context.Context, hash []byte, now time.Time) (EnrollmentCode, error)
	// UseEnrollmentCode deletes the code with this hash if it is unexpired at
	// now, returning ErrNotFound if there's no such code (it was already used).
	UseEnrollmentCode(ctx context.Context, hash []byte, now time.Time) error

	InsertCeremony(ctx context.Context, ceremony Ceremony) error
	// TakeCeremony deletes and returns the ceremony with this challenge if it
	// is unexpired at now, so each one can finish only once.
	TakeCeremony(ctx context.Context, challenge string, now time.Time) (Ceremony, error)

	InsertPasskey(ctx context.Context, passkey Passkey) error
	FindPasskey(ctx context.Context, id []byte) (Passkey, error)
	// ListPasskeys returns every passkey, oldest first.
	ListPasskeys(ctx context.Context) ([]Passkey, error)
	DeletePasskey(ctx context.Context, id []byte) error

	InsertSession(ctx context.Context, session Session) error
	// FindSession returns the session with this token hash that is unexpired at now.
	FindSession(ctx context.Context, tokenHash []byte, now time.Time) (Session, error)
	// ListSessions returns the sessions unexpired at now, oldest first.
	ListSessions(ctx context.Context, now time.Time) ([]Session, error)
	RenewSession(ctx context.Context, id string, renewedAt, expiresAt time.Time) error
	DeleteSession(ctx context.Context, id string) error

	// FindSettings returns the settings, or ErrNotFound if none were ever saved.
	FindSettings(ctx context.Context) (Settings, error)
	SaveSettings(ctx context.Context, settings Settings) error

	// SaveWeighIn stores weighIn, replacing any for the same week.
	SaveWeighIn(ctx context.Context, weighIn WeighIn) error
	// ListWeighIns returns the newest limit Weigh-ins, newest first, or every
	// one if limit is 0.
	ListWeighIns(ctx context.Context, limit int) ([]WeighIn, error)
	DeleteWeighIn(ctx context.Context, week Day) error
}

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
	// BMI is the latest Weigh-in's body mass index to 1 decimal, or
	// ErrNotFound until there is a Weigh-in and the height is set.
	BMI(ctx context.Context) (float64, error)
}
