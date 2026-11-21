package service

import (
	"context"
	"time"

	"github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/core/utils"
)

// gmailDigest is Core's daily ask for the Gmail Digest (ADR 0009): a
// Notification in the Core Assistant's chat that wakes a run, which reads the
// Owner's new mail in Gmail and answers it with the digest.
type gmailDigest struct {
	settings func(context.Context) (model.Settings, error)
}

// DueNotifications returns the ask if slot, which is in the Owner Time Zone,
// is the Gmail Digest's time.
func (p gmailDigest) DueNotifications(ctx context.Context, slot time.Time) ([]model.Notification, error) {
	settings, err := p.settings(ctx)
	if err != nil {
		return nil, err
	}
	digest := settings.GmailDigest
	if !digest.Enabled || !utils.IsAt(slot, digest.Time) {
		return nil, nil
	}
	return []model.Notification{{
		ID:    model.NotificationID(model.CoreAssistant, "gmail-digest", model.DayOf(slot, slot.Location())),
		Title: "📬 Gmail Digest",
		Body:  "Reading your new mail…",
		Asks:  true,
	}}, nil
}
