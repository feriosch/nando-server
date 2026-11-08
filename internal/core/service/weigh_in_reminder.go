package service

import (
	"context"
	"time"

	"github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/core/utils"
)

// weighInReminder is Core's own Notification provider: the weekly reminder to
// weigh in, at the weekday and time in the Owner's settings.
type weighInReminder struct {
	settings func(context.Context) (model.Settings, error)
}

// DueNotifications returns the reminder if slot, which is in the Owner Time
// Zone, is its weekday and time.
func (p weighInReminder) DueNotifications(ctx context.Context, slot time.Time) ([]model.Notification, error) {
	settings, err := p.settings(ctx)
	if err != nil {
		return nil, err
	}
	reminder := settings.WeighInReminder
	if !reminder.Enabled || slot.Weekday() != reminder.Weekday || !utils.IsAt(slot, reminder.Time) {
		return nil, nil
	}
	week := model.DayOf(slot, slot.Location()).Week()
	return []model.Notification{{
		ID:    model.NotificationID("core", "weigh-in-reminder", week),
		Title: "Weigh-in",
		Body:  "Time to log this week's weight",
		URL:   "/",
	}}, nil
}
