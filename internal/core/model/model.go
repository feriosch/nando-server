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
}

// CoreService is what other Modules' services may ask of Core. Its methods
// arrive with Core's first features (#8).
type CoreService interface{}
