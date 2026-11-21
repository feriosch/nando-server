package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/core/utils"
)

// sentNotificationTTL is how long a sent Notification's ID is remembered: far
// longer than any tick is retried, and longer than a provider could still be
// returning it.
const sentNotificationTTL = 30 * 24 * time.Hour

// NotificationsService runs the tick, which only the scheduler may call. It
// sends the due Notifications: it asks every Notification provider for them,
// records each as sent, and sends it in the Messenger (ADR 0008). It also runs
// every tick job. It asks the SettingsService for the Owner Time Zone.
type NotificationsService struct {
	sentNotifications model.SentNotificationsRepository
	settings          *SettingsService
	providers         []model.NotificationProvider
	tickJobs          []model.TickJob
	// messenger sends the Notifications, in Telegram, while some Assistant
	// has a bot.
	messenger notifier

	verifySchedulerToken func(ctx context.Context, token string) error
	now                  func() time.Time
}

// notifier sends Notifications.
type notifier interface {
	// On reports whether it can send Notifications at all.
	On() bool
	// Notify sends n.
	Notify(ctx context.Context, n model.Notification) error
}

// SetMessenger has the tick send Notifications through m. Core sets it once,
// before serving.
func (s *NotificationsService) SetMessenger(m notifier) {
	s.messenger = m
}

// NewNotifications returns the NotificationsService, accepting only ticks
// whose bearer token verifySchedulerToken accepts, and with now as its clock.
// Core's own providers, the Weigh-in reminder and the Mail Digest, are
// already added.
func NewNotifications(
	sentNotifications model.SentNotificationsRepository,
	settings *SettingsService,
	verifySchedulerToken func(ctx context.Context, token string) error,
	now func() time.Time,
) (*NotificationsService, error) {
	if verifySchedulerToken == nil {
		return nil, errors.New("verifySchedulerToken is required")
	}
	return &NotificationsService{
		sentNotifications: sentNotifications,
		settings:          settings,
		providers:         []model.NotificationProvider{weighInReminder{settings: settings.Settings}, mailDigest{settings: settings.Settings}},

		verifySchedulerToken: verifySchedulerToken,
		now:                  now,
	}, nil
}

// Routes adds the tick's route to mux, /internal/tick.
func (s *NotificationsService) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /internal/tick", s.tick)
}

// AddNotificationProvider has Core ask p for its due Notifications at every
// tick. Call it only at startup, before serving: providers aren't guarded for
// concurrent use.
func (s *NotificationsService) AddNotificationProvider(p model.NotificationProvider) {
	s.providers = append(s.providers, p)
}

// AddTickJob has Core run j at every tick. Call it only at startup, before
// serving: tick jobs aren't guarded for concurrent use.
func (s *NotificationsService) AddTickJob(j model.TickJob) {
	s.tickJobs = append(s.tickJobs, j)
}

// tick is called by Cloud Scheduler every 5 minutes. Only the scheduler may
// call it.
func (s *NotificationsService) tick(w http.ResponseWriter, r *http.Request) {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || token == "" {
		http.Error(w, "a scheduler token is required", http.StatusUnauthorized)
		return
	}
	if err := s.verifySchedulerToken(r.Context(), token); err != nil {
		log.Printf("tick: rejected token: %v", err)
		http.Error(w, "not the scheduler", http.StatusUnauthorized)
		return
	}
	// A tick job that fails doesn't stop the Notifications, nor they it.
	errs := []error{s.sendDueNotifications(r.Context())}
	for _, j := range s.tickJobs {
		errs = append(errs, j.RunAtTick(r.Context()))
	}
	if err := errors.Join(errs...); err != nil {
		utils.ServerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// sendDueNotifications sends every provider's due Notifications in the
// Messenger. A provider that fails, or a Notification that can't be recorded
// as sent, doesn't stop the others; the error is returned once they are done,
// so the scheduler retries the tick. With no Assistant's bot to send them,
// nothing is sent or recorded, and the tick says so.
func (s *NotificationsService) sendDueNotifications(ctx context.Context) error {
	slot, err := s.currentSlot(ctx)
	if err != nil {
		logTick(nil, 1)
		return err
	}
	on := s.messenger != nil && s.messenger.On()
	var errs []error
	var sent []sentNotification
	unsent := 0
	for _, p := range s.providers {
		due, err := p.DueNotifications(ctx, slot)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if !on {
			unsent += len(due)
			continue
		}
		for _, n := range due {
			// Claimed before sending: a Notification Telegram failed to take
			// is lost rather than possibly sent twice.
			claimed, err := s.sentNotifications.Claim(ctx, n.ID, s.now(), s.now().Add(sentNotificationTTL))
			if err != nil {
				errs = append(errs, fmt.Errorf("record Notification %.80s as sent: %w", n.ID, err))
				continue
			}
			if !claimed {
				continue
			}
			refused := false
			if err := s.messenger.Notify(ctx, n); err != nil {
				log.Printf("send Notification %s in Telegram: %v", n.ID, err)
				refused = true
			}
			sent = append(sent, sentNotification{id: n.ID, refused: refused})
		}
	}
	if unsent > 0 {
		// Not recorded as sent: once a bot has a token, a Notification that is
		// still due goes out.
		line := "tick: " + counted(unsent, "Notification") + " due, but no Assistant has a bot to send them"
		if len(errs) > 0 {
			line += ", " + counted(len(errs), "error")
		}
		log.Print(line)
		return errors.Join(errs...)
	}
	logTick(sent, len(errs))
	return errors.Join(errs...)
}

// logTick logs the tick's one line: the Notifications it sent in Telegram,
// and how many errors it met, which ServerError logs on their own. Only a
// tick without errors says nothing was due.
func logTick(sent []sentNotification, errs int) {
	switch {
	case len(sent) == 0 && errs == 0:
		log.Print("tick: no Notifications due")
	case len(sent) == 0:
		log.Printf("tick: no Notifications sent, %s", counted(errs, "error"))
	default:
		head := "tick: sent " + counted(len(sent), "Notification") + " in Telegram"
		if errs > 0 {
			head += ", " + counted(errs, "error")
		}
		reports := make([]string, len(sent))
		for i, n := range sent {
			reports[i] = n.String()
		}
		log.Printf("%s: %s", head, strings.Join(reports, ", "))
	}
}

// sentNotification is a Notification the tick sent, and whether Telegram
// refused it.
type sentNotification struct {
	id      string
	refused bool
}

// String is how the tick's log line reports it, such as
// "core:weigh-in:2026-11-16 (sent)" or "(refused)".
func (n sentNotification) String() string {
	if n.refused {
		return n.id + " (refused)"
	}
	return n.id + " (sent)"
}

// currentSlot is the start of the half hour it is now in the Owner Time
// Zone, such as 08:00 or 08:30 there. It is worked out on the Owner's clock,
// not UTC's, so it lands on :00 or :30 even where the UTC offset is, say, +5:45.
func (s *NotificationsService) currentSlot(ctx context.Context) (time.Time, error) {
	loc, err := s.settings.OwnerTimeZone(ctx)
	if err != nil {
		return time.Time{}, err
	}
	now := s.now().In(loc)
	return time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute()/30*30, 0, 0, loc), nil
}

// counted is n of thing, such as "1 Notification" or "2 Notifications".
func counted(n int, thing string) string {
	if n == 1 {
		return "1 " + thing
	}
	return fmt.Sprintf("%d %ss", n, thing)
}
