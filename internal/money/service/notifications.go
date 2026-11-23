package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	coremodel "github.com/feriosch/nando-server/internal/core/model"
	coreutils "github.com/feriosch/nando-server/internal/core/utils"
	"github.com/feriosch/nando-server/internal/money/model"
	"github.com/feriosch/nando-server/internal/money/utils"
)

// notificationArea is the area of Money's Notifications, which Core sends in
// the Money Assistant's chat.
const notificationArea = "money"

// reminderAgain is how many Days after its day a Statement Reminder is sent
// again, while Statements are still missing.
const reminderAgain = 3

// NotificationsService is the Module's Notification provider: Statement
// Reminders, and the asks for each Complete month's Monthly Review
// (ADR 0014).
type NotificationsService struct {
	settings   *SettingsService
	accounts   model.AccountsRepository
	statements *StatementsService
	reviews    model.ReviewsRepository
}

// NewNotifications returns the NotificationsService.
func NewNotifications(settings *SettingsService, accounts model.AccountsRepository, statements *StatementsService, reviews model.ReviewsRepository) *NotificationsService {
	return &NotificationsService{settings: settings, accounts: accounts, statements: statements, reviews: reviews}
}

// DueNotifications returns, at the reminder time, the Statement Reminders due
// and the asks for Monthly Reviews of months that became Complete. Its first
// call ever sets the first month Reviews are asked for: the one before.
func (s *NotificationsService) DueNotifications(ctx context.Context, slot time.Time) ([]coremodel.Notification, error) {
	today := string(coremodel.DayOf(slot, slot.Location()))
	thisMonth := utils.MonthOf(today)
	from, err := s.reviews.From(ctx, utils.AddMonths(thisMonth, -1))
	if err != nil {
		return nil, err
	}
	settings, err := s.settings.savedOrDefault(ctx)
	if err != nil || !coreutils.IsAt(slot, settings.ReminderTime) {
		return nil, err
	}
	accounts, err := s.accounts.List(ctx)
	if err != nil {
		return nil, err
	}
	byID := map[string]model.Account{}
	for _, a := range accounts {
		byID[a.ID] = a
	}
	var due []coremodel.Notification
	for _, group := range settings.Reminders {
		n, err := s.reminder(ctx, group, byID, today)
		if err != nil {
			return nil, err
		}
		if n != nil {
			due = append(due, *n)
		}
	}
	for m := from; m < thisMonth; m = utils.AddMonths(m, 1) {
		// Asked before today: done. Asked today: asked again, in case the
		// tick is being retried, as Core sends each Notification ID once.
		askedOn, err := s.reviews.Asked(ctx, m)
		if err != nil {
			return nil, err
		}
		if askedOn != "" && askedOn != today {
			continue
		}
		month, err := s.statements.Month(ctx, m)
		if err != nil {
			return nil, err
		}
		if !month.Complete || !tracked(accounts, m) {
			continue
		}
		if err := s.reviews.Record(ctx, m, today); err != nil {
			return nil, err
		}
		due = append(due, coremodel.Notification{
			ID:    coremodel.NotificationID(notificationArea, "monthly-review", coremodel.Day(m+"-01")),
			Title: "📊 Monthly Review",
			Body:  fmt.Sprintf("%s is Complete: write its Monthly Review.", monthName(m)),
			Asks:  true,
		})
	}
	return due, nil
}

// reminder is group's Statement Reminder due today, if any: on its day, and
// again reminderAgain Days later, while Statements of its Accounts are
// missing. A month waits for the Statements reaching its end; an Account that
// doesn't hold the month, for the cycle that closed the day before the
// reminder's day.
func (s *NotificationsService) reminder(ctx context.Context, group model.ReminderGroup, accounts map[string]model.Account, today string) (*coremodel.Notification, error) {
	thisMonth := utils.MonthOf(today)
	for _, base := range []string{thisMonth, utils.AddMonths(thisMonth, -1)} {
		day := fmt.Sprintf("%s-%02d", base, group.Day)
		if today != day && today != utils.AddDays(day, reminderAgain) {
			continue
		}
		month := utils.AddMonths(base, -1)
		var missing, cycles []string
		for _, id := range group.AccountIDs {
			a, ok := accounts[id]
			if !ok || !a.TracksIn(month) {
				continue
			}
			needed := utils.LastDay(month)
			if !a.HoldsMonth {
				needed = utils.AddDays(day, -1)
			}
			reached, err := s.statements.reaches(ctx, a, needed)
			if err != nil {
				return nil, err
			}
			switch {
			case reached:
			case a.HoldsMonth:
				missing = append(missing, a.Name)
			default:
				cycles = append(cycles, a.Name)
			}
		}
		if len(missing) == 0 && len(cycles) == 0 {
			return nil, nil
		}
		var asks []string
		if len(missing) > 0 {
			asks = append(asks, fmt.Sprintf("%s's Statements of %s", monthName(month), joinNames(missing)))
		}
		if len(cycles) > 0 {
			closed, _ := time.Parse(utils.DayLayout, utils.AddDays(day, -1))
			asks = append(asks, fmt.Sprintf("the Statement of %s that closed on %s", joinNames(cycles), closed.Format("2 January")))
		}
		return &coremodel.Notification{
			ID:    coremodel.NotificationID(notificationArea, "statement-reminder:"+strings.ReplaceAll(group.Name, ":", " "), coremodel.Day(today)),
			Title: "🧾 Statements: " + group.Name,
			Body:  "When you can, send " + strings.Join(asks, ", and ") + ".",
		}, nil
	}
	return nil, nil
}

// monthName is month as the Owner reads it, such as "October 2026".
func monthName(month string) string {
	t, _ := time.Parse(utils.MonthLayout, month)
	return t.Format("January 2006")
}

// joinNames is names as a list in a sentence: "A", "A and B", "A, B and C".
func joinNames(names []string) string {
	if len(names) == 1 {
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}
