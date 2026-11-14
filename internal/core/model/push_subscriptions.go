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

// TestSentJSON is what came of sending the test Notification: how many
// devices took it, how many were gone and removed, and which push services
// failed.
type TestSentJSON struct {
	Sent    int              `json:"sent"`
	Removed int              `json:"removed"`
	Failed  []TestFailedJSON `json:"failed"`
}

// TestFailedJSON is a push service that didn't take the test Notification,
// named by its host so that the endpoint, which identifies a device, never
// leaves the server.
type TestFailedJSON struct {
	PushService string `json:"pushService"`
	Status      int    `json:"status,omitempty"`
	Reason      string `json:"reason"`
}
