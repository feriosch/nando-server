package model

import (
	"errors"
	"time"
)

// ErrNotFound is returned by CoreRepository when there's nothing to return,
// including when what was asked for has expired.
var ErrNotFound = errors.New("not found")

// EnrollmentCode lets the Owner register a new passkey once, before it expires.
// Only the code's hash is stored.
type EnrollmentCode struct {
	Hash      []byte    `bson:"_id"`
	ExpiresAt time.Time `bson:"expiresAt"`
}

// Ceremony is a passkey registration or sign-in between its begin and finish
// requests, which Cloud Run may send to different instances.
type Ceremony struct {
	Challenge string `bson:"_id"`
	// SessionData is go-webauthn's state for the ceremony, as JSON.
	SessionData []byte `bson:"sessionData"`
	// EnrollmentCodeHash is the code a registration was begun with, used up only
	// when the registration finishes.
	EnrollmentCodeHash []byte    `bson:"enrollmentCodeHash,omitempty"`
	ExpiresAt          time.Time `bson:"expiresAt"`
}

// Passkey is one of the Owner's passkeys. The server holds only its public key.
type Passkey struct {
	ID []byte `bson:"_id"`
	// Credential is go-webauthn's Credential as JSON.
	Credential []byte    `bson:"credential"`
	CreatedAt  time.Time `bson:"createdAt"`
}

// Session is one signed-in browser or Home Screen app. Only the hash of the
// token in its cookie is stored.
type Session struct {
	ID        string `bson:"_id"`
	TokenHash []byte `bson:"tokenHash"`
	// UserAgent is the browser's, from when it signed in, so the Owner can tell
	// sessions apart.
	UserAgent string    `bson:"userAgent"`
	CreatedAt time.Time `bson:"createdAt"`
	// RenewedAt is when ExpiresAt was last pushed back.
	RenewedAt time.Time `bson:"renewedAt"`
	ExpiresAt time.Time `bson:"expiresAt"`
}
