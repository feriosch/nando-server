package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/feriosch/nando-server/internal/calories/model"
	coremodel "github.com/feriosch/nando-server/internal/core/model"
	coreutils "github.com/feriosch/nando-server/internal/core/utils"
)

const (
	// notificationTitle is the title of every Calories Notification.
	notificationTitle = "Calories"
)

// NotificationsService is the Module's Notification provider, which Core asks
// at every tick: the no-Entries reminder that nothing is logged yet today and
// the evening summary of how today went, each at the time in the Calories
// settings, the weekly Weigh-in reminder at its weekday and time, and the
// one that Steps stopped syncing. It asks the SettingsService for those
// settings, the DaysService for how today went and the ActivityService for
// its Steps and Health Syncs.
type NotificationsService struct {
	settings *SettingsService
	days     *DaysService
	activity *ActivityService
}

// NewNotifications returns the NotificationsService.
func NewNotifications(settings *SettingsService, days *DaysService, activity *ActivityService) *NotificationsService {
	return &NotificationsService{settings: settings, days: days, activity: activity}
}

// notSyncedAfter is how long without a Health Sync before the Owner is told
// their Steps stopped syncing.
const notSyncedAfter = 48 * time.Hour

// notSynced is the Notification that Steps stopped syncing, if no Health
// Sync has come for 48 hours at slot. It is named after the Day of the last
// one, so it is sent once for each gap. Before the first Health Sync ever,
// there is nothing to say.
func (s *NotificationsService) notSynced(ctx context.Context, slot time.Time) (*coremodel.Notification, error) {
	last, err := s.activity.healthSyncs.LastSyncedAt(ctx)
	if errors.Is(err, coreutils.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if slot.Sub(last) < notSyncedAfter {
		return nil, nil
	}
	last = last.In(slot.Location())
	n := notification("steps-not-synced", coremodel.DayOf(last, slot.Location()), "Steps haven't synced since "+last.Format("Mon, Jan 2"))
	return &n, nil
}

// DueNotifications returns the Calories Notifications due at slot, which is
// in the Owner Time Zone.
func (s *NotificationsService) DueNotifications(ctx context.Context, slot time.Time) ([]coremodel.Notification, error) {
	settings, err := s.settings.savedOrDefault(ctx)
	if err != nil {
		return nil, err
	}
	day := coremodel.DayOf(slot, slot.Location())
	var due []coremodel.Notification
	if settings.WeighInReminder.DueAt(slot) {
		due = append(due, notification("weigh-in-reminder", day.Week(), "Time to log this week's weight"))
	}
	notSynced, err := s.notSynced(ctx, slot)
	if err != nil {
		return nil, err
	}
	if notSynced != nil {
		due = append(due, *notSynced)
	}
	reminderDue := settings.NoEntriesReminder.DueAt(slot)
	summaryDue := settings.EveningSummary.DueAt(slot)
	if !reminderDue && !summaryDue {
		return due, nil
	}
	calories, _, err := s.days.dayCalories(ctx, day)
	if err != nil {
		return nil, err
	}
	// The evening summary of an Unlogged Day already says nothing was logged,
	// so the reminder isn't sent alongside it.
	if reminderDue && !summaryDue && calories.ConsumedCalories == nil {
		due = append(due, notification("no-entries-reminder", day, "Nothing logged yet today"))
	}
	if summaryDue {
		activity, err := s.activity.dayActivity(ctx, day)
		if err != nil {
			return nil, err
		}
		due = append(due, notification("evening-summary", day, eveningSummaryBody(calories)+activityLine(activity)))
	}
	return due, nil
}

// notification is the Calories Notification of this kind for day, or for
// the week starting on day.
func notification(kind string, day coremodel.Day, body string) coremodel.Notification {
	return coremodel.Notification{
		ID:    coremodel.NotificationID("calories", kind, day),
		Title: notificationTitle,
		Body:  body,
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

// activityLine is the evening summary's line of the Day's Steps and Burned
// Calories, after a line break, or "" for a Day with neither.
func activityLine(a model.DayActivity) string {
	var parts []string
	if steps := a.Steps(); steps != nil {
		parts = append(parts, fmt.Sprintf("%d steps", *steps))
	}
	if burned := a.BurnedCalories(); burned != nil {
		parts = append(parts, fmt.Sprintf("%d kcal burned (est.)", *burned))
	}
	if len(parts) == 0 {
		return ""
	}
	return "\n" + strings.Join(parts, " · ")
}
