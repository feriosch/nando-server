package model

import (
	"context"
	"time"
)

// Session is one signed-in browser or Home Screen app. Only the hash of the
// token in its cookie is stored.
type Session struct {
	ID        string `bson:"_id"`
	TokenHash []byte `bson:"tokenHash"`
	// UserAgent is the browser's, from when it signed in, so the Owner can tell
	// Sessions apart.
	UserAgent string    `bson:"userAgent"`
	CreatedAt time.Time `bson:"createdAt"`
	// RenewedAt is when ExpiresAt was last pushed back.
	RenewedAt time.Time `bson:"renewedAt"`
	ExpiresAt time.Time `bson:"expiresAt"`
}

// SessionsRepository reads and writes the sessions collection.
type SessionsRepository interface {
	Insert(ctx context.Context, session Session) error
	// Find returns the Session with this token hash that is unexpired at now.
	Find(ctx context.Context, tokenHash []byte, now time.Time) (Session, error)
	// List returns the Sessions unexpired at now, oldest first.
	List(ctx context.Context, now time.Time) ([]Session, error)
	Renew(ctx context.Context, id string, renewedAt, expiresAt time.Time) error
	Delete(ctx context.Context, id string) error
}

// SessionJSON is a Session as the API lists it. Current marks the Session
// asking.
type SessionJSON struct {
	ID        string    `json:"id"`
	UserAgent string    `json:"userAgent"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
	Current   bool      `json:"current"`
}
