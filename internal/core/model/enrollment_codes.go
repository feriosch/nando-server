package model

import (
	"context"
	"time"
)

// EnrollmentCode lets the Owner register a new Passkey once, before it expires.
// Only the code's hash is stored.
type EnrollmentCode struct {
	Hash      []byte    `bson:"_id"`
	ExpiresAt time.Time `bson:"expiresAt"`
}

// EnrollmentCodesRepository reads and writes the enrollment_codes collection.
type EnrollmentCodesRepository interface {
	Insert(ctx context.Context, code EnrollmentCode) error
	// Find returns the code with this hash that is unexpired at now.
	Find(ctx context.Context, hash []byte, now time.Time) (EnrollmentCode, error)
	// Use deletes the code with this hash if it is unexpired at now,
	// returning utils.ErrNotFound if there's no such code (it was already used).
	Use(ctx context.Context, hash []byte, now time.Time) error
}
