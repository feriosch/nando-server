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
	// Forget deletes the Push Subscription at endpoint because its push
	// service no longer knows it, and remembers so until expiresAt.
	Forget(ctx context.Context, endpoint string, now, expiresAt time.Time) error
	// Forgotten reports whether Forget was told, before now and still
	// remembered, that endpoint's push service no longer knows it.
	Forgotten(ctx context.Context, endpoint string, now time.Time) (bool, error)
}

// PushSubscriptionJSON is a Push Subscription as PushSubscription.toJSON()
// gives it in the browser.
type PushSubscriptionJSON struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

// VAPIDPublicKeyJSON is the key the web app passes to pushManager.subscribe
// as its applicationServerKey.
type VAPIDPublicKeyJSON struct {
	PublicKey string `json:"publicKey"`
}
