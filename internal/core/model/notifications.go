package model

import (
	"context"
	"time"
)

// Notification is a short message delivered to every Push Subscription. Core
// sends it, either on a Module's behalf or for itself.
type Notification struct {
	// ID names this one Notification for good; make it with NotificationID,
	// or, for one that isn't once a day or week, as area:kind: and then what
	// tells it apart, such as "japanese:word-request:<id>:<state>:<ms>".
	// Core sends each ID at most once.
	ID    string
	Title string
	Body  string
	// URL is the path in the web app to open when the Owner taps it, such as
	// "/calories".
	URL string
}

// NotificationID names the Notification of this kind from area (Core or a
// Module) for day, such as "calories:evening-summary:2026-10-20". A kind sent
// once a week uses the week's Monday as day.
func NotificationID(area, kind string, day Day) string {
	return area + ":" + kind + ":" + string(day)
}

// NotificationProvider is how a Module (or Core itself) tells Core which of
// its Notifications to send. Core asks every provider at each tick.
type NotificationProvider interface {
	// DueNotifications returns the Notifications due at the tick for the half
	// hour starting at slot, such as 20:00 or 20:30. slot is in the Owner Time
	// Zone (slot.Location()).
	// A provider may return one it returned before (a retried tick); Core sends
	// each ID only once.
	DueNotifications(ctx context.Context, slot time.Time) ([]Notification, error)
}

// PushMessageJSON is what a push message carries: one Notification, as the
// web app's service worker reads it. An empty field is left out, so the
// service worker shows its default ("Nando" for the title, Home for the url).
type PushMessageJSON struct {
	Title string `json:"title,omitempty"`
	Body  string `json:"body,omitempty"`
	URL   string `json:"url,omitempty"`
}

// TestNotificationResultJSON is what came of sending the test Notification:
// how many Push Subscriptions took it, how many their push service no longer
// knew and were deleted, and which push services failed.
type TestNotificationResultJSON struct {
	Sent    int                           `json:"sent"`
	Removed int                           `json:"removed"`
	Failed  []TestNotificationFailureJSON `json:"failed"`
}

// TestNotificationFailureJSON is a push service that didn't take the test
// Notification, named by its host so that the endpoint, a secret capability
// URL, never leaves the server.
type TestNotificationFailureJSON struct {
	PushService string `json:"pushService"`
	// Status is what the push service answered, or 0 (left out) if it didn't.
	Status int    `json:"status,omitempty"`
	Reason string `json:"reason"`
}
