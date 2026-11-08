package model

import (
	"context"
	"time"
)

// Passkey is one of the Owner's passkeys. The server holds only its public key.
type Passkey struct {
	ID []byte `bson:"_id"`
	// Credential is go-webauthn's Credential as JSON.
	Credential []byte    `bson:"credential"`
	CreatedAt  time.Time `bson:"createdAt"`
}

// PasskeysRepository reads and writes the passkeys collection.
type PasskeysRepository interface {
	Insert(ctx context.Context, passkey Passkey) error
	Find(ctx context.Context, id []byte) (Passkey, error)
	// List returns every passkey, oldest first.
	List(ctx context.Context) ([]Passkey, error)
	Delete(ctx context.Context, id []byte) error
}
