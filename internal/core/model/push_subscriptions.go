package model

import (
	"context"
	"time"
)

// PushSubscription is the permission and delivery address of one installed
// copy of Nando on one device.
type PushSubscription struct {
	// Endpoint is the push service URL that identifies it.
	Endpoint string `bson:"_id"`
	// P256dh and Auth are the browser's keys, base64url as the browser gives them.
	P256dh    string    `bson:"p256dh"`
	Auth      string    `bson:"auth"`
	CreatedAt time.Time `bson:"createdAt"`
}

// PushSubscriptionsRepository reads and writes the push_subscriptions
// collection.
type PushSubscriptionsRepository interface {
	// Save stores sub, replacing any with the same endpoint.
	Save(ctx context.Context, sub PushSubscription) error
	List(ctx context.Context) ([]PushSubscription, error)
	Delete(ctx context.Context, endpoint string) error
}
