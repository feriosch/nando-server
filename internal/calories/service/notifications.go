package service

import (
	"context"
	"time"

	"github.com/feriosch/nando-server/internal/calories/model"
	coremodel "github.com/feriosch/nando-server/internal/core/model"
)

// NotificationsService is the Module's Notification provider, which Core asks
// at every tick: the reminder that nothing is logged yet today, at the time
// in the Calories settings.
type NotificationsService struct {
	settings *SettingsService
	entries  model.EntriesRepository
}

// NewNotifications returns the NotificationsService, which reads the Calories
// settings through settings.
func NewNotifications(settings *SettingsService, entries model.EntriesRepository) *NotificationsService {
	return &NotificationsService{settings: settings, entries: entries}
}

// DueNotifications returns the Calories Notifications due at slot, which is
// in the Owner Time Zone.
func (s *NotificationsService) DueNotifications(ctx context.Context, slot time.Time) ([]coremodel.Notification, error) {
	settings, err := s.settings.savedOrDefault(ctx)
	if err != nil {
		return nil, err
	}
	reminder := settings.NoEntriesReminder
	if !reminder.Enabled || slot.Format("15:04") != reminder.Time {
		return nil, nil
	}
	day := coremodel.DayOf(slot, slot.Location())
	entries, err := s.entries.ListOnDay(ctx, day)
	if err != nil || len(entries) > 0 {
		return nil, err
	}
	return []coremodel.Notification{{
		ID:    coremodel.NotificationID("calories", "no-entries-reminder", day),
		Title: "Calories",
		Body:  "Nothing logged yet today",
		URL:   "/calories",
	}}, nil
}
