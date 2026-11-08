package model

import (
	"context"
	"time"
)

// SentNotificationsRepository reads and writes the sent_notifications
// collection: the IDs of the Notifications already sent.
type SentNotificationsRepository interface {
	// Claim records that the Notification with this ID is being sent at now,
	// keeping the record until expiresAt. It returns false if the ID was
	// already claimed, so each one is sent only once.
	Claim(ctx context.Context, id string, now, expiresAt time.Time) (bool, error)
}
