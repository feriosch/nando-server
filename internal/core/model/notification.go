package model

import (
	"context"
	"time"
)

// Notification is a short message delivered to every Push Subscription. Core
// sends it, either on a Module's behalf or for itself.
type Notification struct {
	// ID names this one Notification for good, as "<area>:<kind>:<Day>" such
	// as "calories:evening-summary:2026-10-20". Core sends each ID at most once.
	ID    string
	Title string
	Body  string
	// URL is the path in the web app to open when the Owner taps it, such as
	// "/calories".
	URL string
}

// NotificationProvider is how a Module (or Core itself) tells Core which of
// its Notifications to send. Core asks every provider at each tick.
type NotificationProvider interface {
	// DueNotifications returns the Notifications due at the tick for the half
	// hour starting at slot, such as 20:00 or 20:30 in the Owner Time Zone.
	// A provider may return one it returned before (a retried tick); Core sends
	// each ID only once.
	DueNotifications(ctx context.Context, slot time.Time) ([]Notification, error)
}

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
