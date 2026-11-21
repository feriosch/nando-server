package model

import (
	"context"
	"time"
)

// Notification is a short message Nando sends the Owner first, in the
// Messenger (ADR 0008). Core sends it, either on a Module's behalf or for
// itself.
type Notification struct {
	// ID names this one Notification for good; make it with NotificationID,
	// or, for one that isn't once a day or week, as area:kind: and then what
	// tells it apart, such as "japanese:word-request:<id>:<state>:<ms>".
	// Core sends each ID at most once.
	ID    string
	Title string
	Body  string
	// Asks marks a Notification that asks its area's Assistant for work, such
	// as the Mail Digest (ADR 0009): it waits in that Assistant's Talk like
	// the Owner's message and wakes a run, which answers it.
	Asks bool
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
