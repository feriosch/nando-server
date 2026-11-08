package service

import (
	"context"
	"time"

	"github.com/feriosch/nando-server/internal/core/model"
)

// weighInReminder is Core's own Notification provider: the weekly reminder to
// weigh in, at the weekday and time in the Owner's settings.
type weighInReminder struct{ s *Service }

func (p weighInReminder) DueNotifications(ctx context.Context, slot time.Time) ([]model.Notification, error) {
	settings, err := p.s.settings(ctx)
	if err != nil {
		return nil, err
	}
	reminder := settings.WeighInReminder
	if !reminder.Enabled {
		return nil, nil
	}
	loc, err := time.LoadLocation(settings.TimeZone)
	if err != nil {
		return nil, err
	}
	local := slot.In(loc)
	if local.Weekday() != reminder.Weekday || local.Format("15:04") != reminder.Time {
		return nil, nil
	}
	return []model.Notification{{
		ID:    "core:weigh-in-reminder:" + string(model.DayOf(slot, loc).Week()),
		Title: "Weigh-in",
		Body:  "Time to log this week's weight",
		URL:   "/",
	}}, nil
}
