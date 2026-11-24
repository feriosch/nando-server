package model

import (
	"context"
	"time"
)

// SyncKey lets one of the Owner's own automations, such as the Health Sync
// Shortcut, call the few routes a Module opens to Sync Keys (ADR 0015). Only
// the hash of its key is stored, like a Session token's.
type SyncKey struct {
	ID        string    `bson:"_id"`
	Name      string    `bson:"name"`
	KeyHash   []byte    `bson:"keyHash"`
	CreatedAt time.Time `bson:"createdAt"`
	// LastUsedAt is when it last called a route, or nil if it never has.
	LastUsedAt *time.Time `bson:"lastUsedAt,omitempty"`
}

// SyncKeysRepository reads and writes the sync_keys collection.
type SyncKeysRepository interface {
	Insert(ctx context.Context, key SyncKey) error
	// FindByKey returns the Sync Key whose key has hash.
	FindByKey(ctx context.Context, hash []byte) (SyncKey, error)
	// List returns every Sync Key, oldest first.
	List(ctx context.Context) ([]SyncKey, error)
	// MarkUsed records that the Sync Key with id called a route at.
	MarkUsed(ctx context.Context, id string, at time.Time) error
	// Delete removes the Sync Key with id. Deleting one that doesn't exist
	// also succeeds.
	Delete(ctx context.Context, id string) error
}

// SyncKeyJSON is a Sync Key as the API lists it. LastUsedAt is null if it
// has never been used. Key is set only in the answer that creates it, the
// one time the Owner sees it.
type SyncKeyJSON struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
	Key        string     `json:"key,omitempty"`
}

// SyncKeyNameJSON is what the Owner names a new Sync Key.
type SyncKeyNameJSON struct {
	Name string `json:"name"`
}
