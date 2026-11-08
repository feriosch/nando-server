package service

import (
	"context"
	"fmt"
	"time"

	"github.com/feriosch/nando-server/internal/calories/model"
	coremodel "github.com/feriosch/nando-server/internal/core/model"
)

const (
	// notificationTitle is the title of every Calories Notification.
	notificationTitle = "Calories"
	// notificationURL is where every Calories Notification opens the web app.
	notificationURL = "/calories"
)

// NotificationsService is the Module's Notification provider, which Core asks
// at every tick: the no-Entries reminder that nothing is logged yet today and
// the evening summary of how today went, each at the time in the Calories
// settings. It asks the SettingsService for those settings and the
// DaysService for how today went.
type NotificationsService struct {
	settings *SettingsService
	days     *DaysService
}

// NewNotifications returns the NotificationsService.
func NewNotifications(settings *SettingsService, days *DaysService) *NotificationsService {
	return &NotificationsService{settings: settings, days: days}
}

// DueNotifications returns the Calories Notifications due at slot, which is
// in the Owner Time Zone.
func (s *NotificationsService) DueNotifications(ctx context.Context, slot time.Time) ([]coremodel.Notification, error) {
	settings, err := s.settings.savedOrDefault(ctx)
	if err != nil {
		return nil, err
	}
	reminderDue := settings.NoEntriesReminder.DueAt(slot)
	summaryDue := settings.EveningSummary.DueAt(slot)
	if !reminderDue && !summaryDue {
		return nil, nil
	}
	day := coremodel.DayOf(slot, slot.Location())
	calories, _, err := s.days.dayCalories(ctx, day)
	if err != nil {
		return nil, err
	}
	var due []coremodel.Notification
	// The evening summary of an Unlogged Day already says nothing was logged,
	// so the reminder isn't sent alongside it.
	if reminderDue && !summaryDue && calories.ConsumedCalories == nil {
		due = append(due, notification("no-entries-reminder", day, "Nothing logged yet today"))
	}
	if summaryDue {
		due = append(due, notification("evening-summary", day, eveningSummaryBody(calories)))
	}
	return due, nil
}

// notification is the Calories Notification of this kind for day.
func notification(kind string, day coremodel.Day, body string) coremodel.Notification {
	return coremodel.Notification{
		ID:    coremodel.NotificationID("calories", kind, day),
		Title: notificationTitle,
		Body:  body,
		URL:   notificationURL,
	}
}

// eveningSummaryBody is the evening summary's text for the Day. An Unlogged
// Day is not 0 kcal, so it says nothing was logged.
func eveningSummaryBody(d model.DayCalories) string {
	if d.ConsumedCalories == nil {
		return "Nothing logged today"
	}
	if d.DailyLimit == nil {
		return fmt.Sprintf("Today: %d kcal", *d.ConsumedCalories)
	}
	return fmt.Sprintf("Today: %d / %d kcal", *d.ConsumedCalories, *d.DailyLimit)
}
