package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/feriosch/nando-server/internal/career/model"
	coremodel "github.com/feriosch/nando-server/internal/core/model"
	coreutils "github.com/feriosch/nando-server/internal/core/utils"
)

const (
	// notificationArea names the Module in its Notifications' IDs.
	notificationArea = "career"
	// waitingReminderAt is when, on waitingReminderDays, a Not Looking Owner
	// hears of the Recruiters waiting for their reply.
	waitingReminderAt = "19:00"
	// stepReminderAt is when, the evening before, the Owner hears of the
	// next Day's Steps.
	stepReminderAt = "20:00"
	// syncRepliesWithin is how long after a Sync a Looking Owner may still
	// hear of the replies its work led to: its run may wait for the Owner's
	// usage to come back.
	syncRepliesWithin = 48 * time.Hour
	// noSyncAfter is how long with no Sync that read LinkedIn before the
	// Owner hears of it.
	noSyncAfter = 36 * time.Hour
	// syncProblemsFor is how long a Sync problem is still told: well within
	// the 30 days Core remembers a sent ID for, so none is ever sent twice.
	syncProblemsFor = 7 * 24 * time.Hour
)

// waitingReminderDays are the weekdays of the Not Looking reminder.
var waitingReminderDays = []time.Weekday{time.Monday, time.Thursday}

// NotificationsService is the Module's Notification provider, which Core asks
// at every tick: Recruiters waiting for the Owner's reply, as each Sync's
// work is done when the Owner is Looking, or twice a week when not, and
// tomorrow's Steps.
type NotificationsService struct {
	settings      *SettingsService
	conversations model.ConversationsRepository
	opportunities model.OpportunitiesRepository
	companies     model.CompaniesRepository
	syncs         model.SyncsRepository
	work          model.WorkRepository
	now           func() time.Time
}

// NewNotifications returns the NotificationsService.
func NewNotifications(settings *SettingsService, conversations model.ConversationsRepository, opportunities model.OpportunitiesRepository,
	companies model.CompaniesRepository, syncs model.SyncsRepository, work model.WorkRepository, now func() time.Time) *NotificationsService {
	return &NotificationsService{
		settings: settings, conversations: conversations, opportunities: opportunities, companies: companies,
		syncs: syncs, work: work, now: now,
	}
}

// DueNotifications returns the Career Notifications due at slot, which is in
// the Owner Time Zone.
func (s *NotificationsService) DueNotifications(ctx context.Context, slot time.Time) ([]coremodel.Notification, error) {
	settings, err := s.settings.Settings(ctx)
	if err != nil {
		return nil, err
	}
	var due []coremodel.Notification
	if settings.Looking {
		if due, err = s.syncReplies(ctx); err != nil {
			return nil, err
		}
	} else if isWaitingReminderSlot(slot) {
		waiting, err := s.waitingRecruiters(ctx)
		if err != nil {
			return nil, err
		}
		if waiting > 0 {
			day := coremodel.DayOf(slot, slot.Location())
			due = append(due, waitingNotification(coremodel.NotificationID(notificationArea, "replies", day), waiting))
		}
	}
	problems, err := s.syncProblems(ctx, slot.Location())
	if err != nil {
		return nil, err
	}
	due = append(due, problems...)
	if coreutils.IsAt(slot, stepReminderAt) {
		steps, err := s.tomorrowsSteps(ctx, coremodel.DayOf(slot, slot.Location()).AddDays(1))
		if err != nil {
			return nil, err
		}
		due = append(due, steps...)
	}
	return due, nil
}

// syncProblems are the Notifications that Syncs aren't reading LinkedIn.
// That LinkedIn wants the Owner to log in again on the Mac is sent once each
// time it does: for the first Sync that reported it since the last Sync that
// read LinkedIn, as the Mac reports it every hour until the Owner logs in.
// That no Sync read LinkedIn for noSyncAfter is sent once for the last one
// that did, in the Owner Time Zone loc. Neither is sent once syncProblemsFor
// has passed.
func (s *NotificationsService) syncProblems(ctx context.Context, loc *time.Location) ([]coremodel.Notification, error) {
	var due []coremodel.Notification
	var lastRead time.Time
	latest, err := s.syncs.LatestRead(ctx, 1)
	if err != nil {
		return nil, err
	}
	if len(latest) > 0 {
		lastRead = latest[0].At
		// Before the first Sync the Module isn't live, so none is missed.
		if since := s.now().Sub(lastRead); since >= noSyncAfter && since < noSyncAfter+syncProblemsFor {
			due = append(due, coremodel.Notification{
				ID:    notificationID("no-sync", latest[0].ID.Hex()),
				Title: "No LinkedIn Sync since " + lastRead.In(loc).Weekday().String(),
				Body:  "Check that the Mac is on and its Sync job is loaded",
			})
		}
	}
	expired, err := s.syncs.FirstProblemAfter(ctx, model.ProblemSessionExpired, lastRead)
	if errors.Is(err, coreutils.ErrNotFound) {
		return due, nil
	}
	if err != nil {
		return nil, err
	}
	if s.now().Sub(expired.At) >= syncProblemsFor {
		return due, nil
	}
	return append(due, coremodel.Notification{
		ID:    notificationID("sync-problem", expired.ID.Hex()),
		Title: "Log in to LinkedIn again",
		Body:  "Open the Mac and run the Sync's login",
	}), nil
}

// tomorrowsSteps is a Notification for each Step on tomorrow in an open
// Opportunity.
func (s *NotificationsService) tomorrowsSteps(ctx context.Context, tomorrow coremodel.Day) ([]coremodel.Notification, error) {
	open, err := s.opportunities.OpenWithStepOn(ctx, string(tomorrow))
	if err != nil {
		return nil, err
	}
	var due []coremodel.Notification
	for _, o := range open {
		company, err := s.companies.GetByID(ctx, o.CompanyID)
		if err != nil {
			return nil, err
		}
		for _, step := range o.Steps {
			if step.Date != string(tomorrow) {
				continue
			}
			body := company.Name
			if step.Time != "" {
				body += ", " + step.Time
			}
			due = append(due, coremodel.Notification{
				ID:    notificationID("step", o.ID.Hex(), step.ID.Hex(), step.Date),
				Title: "Tomorrow: " + step.Kind,
				Body:  body,
			})
		}
	}
	return due, nil
}

// syncReplies is, for a Looking Owner, a Notification of the Recruiters
// waiting for each recent Sync whose Items are all done or set aside, and
// that led to at least one new answer.
func (s *NotificationsService) syncReplies(ctx context.Context) ([]coremodel.Notification, error) {
	recent, err := s.syncs.AllReadSince(ctx, s.now().Add(-syncRepliesWithin))
	if err != nil {
		return nil, err
	}
	var due []coremodel.Notification
	waiting := -1
	for _, sync := range recent {
		open, err := s.work.AnyOpenFromSync(ctx, sync.ID)
		if err != nil {
			return nil, err
		}
		answered := false
		if !open {
			if answered, err = s.conversations.AnyAnswerFromSync(ctx, sync.ID); err != nil {
				return nil, err
			}
		}
		if !answered {
			continue
		}
		if waiting < 0 {
			if waiting, err = s.waitingRecruiters(ctx); err != nil {
				return nil, err
			}
		}
		if waiting > 0 {
			due = append(due, waitingNotification(notificationID("replies", sync.ID.Hex()), waiting))
		}
	}
	return due, nil
}

// waitingRecruiters counts the Recruiters waiting for the Owner's reply now.
// A reply whose message is old enough to turn late doesn't count, even
// before the tick's rules turn it.
func (s *NotificationsService) waitingRecruiters(ctx context.Context) (int, error) {
	return s.conversations.WaitingRecruiters(ctx, s.now().Add(-lateAfter))
}

// notificationID names one of the Module's Notifications that isn't once a
// Day: career, its kind, and then what tells it apart, joined by colons.
func notificationID(kind string, parts ...string) string {
	return strings.Join(append([]string{notificationArea, kind}, parts...), ":")
}

// isWaitingReminderSlot reports whether slot is when a Not Looking Owner
// hears of the Recruiters waiting.
func isWaitingReminderSlot(slot time.Time) bool {
	for _, d := range waitingReminderDays {
		if slot.Weekday() == d && coreutils.IsAt(slot, waitingReminderAt) {
			return true
		}
	}
	return false
}

// waitingNotification is the Notification with id that waiting Recruiters
// wait for the Owner's reply.
func waitingNotification(id string, waiting int) coremodel.Notification {
	body := fmt.Sprintf("%d Recruiters are waiting for your reply", waiting)
	if waiting == 1 {
		body = "1 Recruiter is waiting for your reply"
	}
	return coremodel.Notification{ID: id, Title: "Recruiters waiting", Body: body}
}
