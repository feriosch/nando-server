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
// records each as sent, and sends it to every Push Subscription. It also runs
// every tick job. It asks the SettingsService for the Owner Time Zone.
type NotificationsService struct {
	pushSubscriptions model.PushSubscriptionsRepository
	sentNotifications model.SentNotificationsRepository
	settings          *SettingsService
	providers         []model.NotificationProvider
	tickJobs          []model.TickJob

	verifySchedulerToken func(ctx context.Context, token string) error
	vapid                VAPID
	now                  func() time.Time
}

// NewNotifications returns the NotificationsService, accepting only ticks
// whose bearer token verifySchedulerToken accepts, signing push messages with
// vapid, and with now as its clock. Core's own provider, the Weigh-in
// reminder, is already added.
func NewNotifications(
	pushSubscriptions model.PushSubscriptionsRepository,
	sentNotifications model.SentNotificationsRepository,
	settings *SettingsService,
	verifySchedulerToken func(ctx context.Context, token string) error,
	vapid VAPID,
	now func() time.Time,
) (*NotificationsService, error) {
	if verifySchedulerToken == nil {
		return nil, errors.New("verifySchedulerToken is required")
	}
	return &NotificationsService{
		pushSubscriptions: pushSubscriptions,
		sentNotifications: sentNotifications,
		settings:          settings,
		providers:         []model.NotificationProvider{weighInReminder{settings: settings.Settings}},

		verifySchedulerToken: verifySchedulerToken,
		vapid:                vapid,
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

// tick is called by Cloud Scheduler every half hour. Only the scheduler may
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

// sendDueNotifications sends every provider's due Notifications to every Push
// Subscription. A provider that fails, or a Notification that can't be
// recorded as sent, doesn't stop the others; the error is returned once they
// are done, so the scheduler retries the tick.
func (s *NotificationsService) sendDueNotifications(ctx context.Context) error {
	slot, err := s.currentSlot(ctx)
	if err != nil {
		return err
	}
	subs, err := s.pushSubscriptions.List(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, p := range s.providers {
		due, err := p.DueNotifications(ctx, slot)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, n := range due {
			// Claimed before sending: a Notification a push service failed to
			// take is lost rather than possibly sent twice.
			claimed, err := s.sentNotifications.Claim(ctx, n.ID, s.now(), s.now().Add(sentNotificationTTL))
			if err != nil {
				errs = append(errs, fmt.Errorf("record Notification %.80s as sent: %w", n.ID, err))
				continue
			}
			if !claimed {
				continue
			}
			for _, sub := range subs {
				err := deliver(ctx, s.vapid, sub, n)
				if errors.Is(err, errPushSubscriptionGone) {
					err = s.pushSubscriptions.Delete(ctx, sub.Endpoint)
				}
				if err != nil {
					log.Printf("send Notification %s: %v", n.ID, err)
				}
			}
		}
	}
	return errors.Join(errs...)
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
