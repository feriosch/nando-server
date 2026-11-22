package service

import (
	"context"
	"time"

	coremodel "github.com/feriosch/nando-server/internal/core/model"
	coreutils "github.com/feriosch/nando-server/internal/core/utils"
	"github.com/feriosch/nando-server/internal/travel/model"
)

// notificationArea is the area of Travel's Notifications, which Core sends in
// the Travel Assistant's chat.
const notificationArea = "travel"

// NotificationsService is the Module's Notification provider: it asks the
// Travel Assistant for the Daily and the Weekly Fare Check at their times in
// the settings, with Notifications that ask for work (ADR 0013).
type NotificationsService struct {
	settings *SettingsService
	trips    model.TripsRepository
}

// NewNotifications returns the NotificationsService.
func NewNotifications(settings *SettingsService, trips model.TripsRepository) *NotificationsService {
	return &NotificationsService{settings: settings, trips: trips}
}

// DueNotifications asks for the Fare Check due at slot, if any. On the Weekly
// Fare Check's weekday only the weekly one is asked for, as it covers the
// daily's work, and neither is while no Trip is Watching.
func (s *NotificationsService) DueNotifications(ctx context.Context, slot time.Time) ([]coremodel.Notification, error) {
	settings, err := s.settings.savedOrDefault(ctx)
	if err != nil {
		return nil, err
	}
	daily, weekly := settings.DailyFareCheck, settings.WeeklyFareCheck
	weeklyDay := weekly.Enabled && slot.Weekday() == weekly.Weekday
	var ask coremodel.Notification
	switch day := coremodel.DayOf(slot, slot.Location()); {
	case weeklyDay && coreutils.IsAt(slot, weekly.Time):
		ask = coremodel.Notification{
			ID:    coremodel.NotificationID(notificationArea, "weekly-fare-check", day),
			Title: "✈️ Weekly Fare Check",
			Body:  "Searching every Trip's whole window…",
			Asks:  true,
		}
	case daily.Enabled && !weeklyDay && coreutils.IsAt(slot, daily.Time):
		ask = coremodel.Notification{
			ID:    coremodel.NotificationID(notificationArea, "daily-fare-check", day),
			Title: "✈️ Daily Fare Check",
			Body:  "Checking new Price Alerts and fares…",
			Asks:  true,
		}
	default:
		return nil, nil
	}
	watching, err := s.trips.List(ctx, []model.Status{model.Watching})
	if err != nil || len(watching) == 0 {
		return nil, err
	}
	return []coremodel.Notification{ask}, nil
}
