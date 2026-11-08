package service

import (
	"context"
	"fmt"
	"time"

	"github.com/feriosch/nando-server/internal/calories/model"
	coremodel "github.com/feriosch/nando-server/internal/core/model"
)

// NotificationsService is the Module's Notification provider, which Core asks
// at every tick: the reminder that nothing is logged yet today and the
// evening summary of how today went, each at the time in the Calories
// settings.
type NotificationsService struct {
	settings    *SettingsService
	entries     model.EntriesRepository
	dailyLimits model.DailyLimitsRepository
}

// NewNotifications returns the NotificationsService, which reads the Calories
// settings through settings.
func NewNotifications(settings *SettingsService, entries model.EntriesRepository, dailyLimits model.DailyLimitsRepository) *NotificationsService {
	return &NotificationsService{settings: settings, entries: entries, dailyLimits: dailyLimits}
}

// DueNotifications returns the Calories Notifications due at slot, which is
// in the Owner Time Zone.
func (s *NotificationsService) DueNotifications(ctx context.Context, slot time.Time) ([]coremodel.Notification, error) {
	settings, err := s.settings.savedOrDefault(ctx)
	if err != nil {
		return nil, err
	}
	reminderDue := isDue(settings.NoEntriesReminder, slot)
	summaryDue := isDue(settings.EveningSummary, slot)
	if !reminderDue && !summaryDue {
		return nil, nil
	}
	day := coremodel.DayOf(slot, slot.Location())
	entries, err := s.entries.ListOnDay(ctx, day)
	if err != nil {
		return nil, err
	}
	var due []coremodel.Notification
	if reminderDue && len(entries) == 0 {
		due = append(due, coremodel.Notification{
			ID:    coremodel.NotificationID("calories", "no-entries-reminder", day),
			Title: "Calories",
			Body:  "Nothing logged yet today",
			URL:   "/calories",
		})
	}
	if summaryDue {
		limits, err := s.dailyLimits.List(ctx)
		if err != nil {
			return nil, err
		}
		due = append(due, coremodel.Notification{
			ID:    coremodel.NotificationID("calories", "evening-summary", day),
			Title: "Calories",
			Body:  summary(limits.Calories(day, model.ConsumedCalories(entries))),
			URL:   "/calories",
		})
	}
	return due, nil
}

// summary is the evening summary's text for the Day.
func summary(d model.DayCalories) string {
	if d.DailyLimit == nil {
		return fmt.Sprintf("Today: %d kcal", *d.ConsumedCalories)
	}
	return fmt.Sprintf("Today: %d / %d kcal", *d.ConsumedCalories, *d.DailyLimit)
}

// isDue reports whether the Notification with this setting is due at slot.
func isDue(setting model.NotificationSetting, slot time.Time) bool {
	return setting.Enabled && slot.Format("15:04") == setting.Time
}
