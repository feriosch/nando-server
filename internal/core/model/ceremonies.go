package model

import (
	"context"
	"time"
)

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

// CeremoniesRepository reads and writes the ceremonies collection.
type CeremoniesRepository interface {
	Insert(ctx context.Context, ceremony Ceremony) error
	// Take deletes and returns the ceremony with this challenge if it is
	// unexpired at now, so each one can finish only once.
	Take(ctx context.Context, challenge string, now time.Time) (Ceremony, error)
}
