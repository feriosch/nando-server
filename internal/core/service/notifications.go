package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
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
// every tick job, and sends the Owner's test Notification on demand. It asks
// the SettingsService for the Owner Time Zone.
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

// Routes adds the tick's route to mux, /internal/tick, and the test
// Notification's, under /api/.
func (s *NotificationsService) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /internal/tick", s.tick)
	mux.HandleFunc("POST /api/push-subscriptions/test", s.sendTestNotification)
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
	var sentIDs []string
	reached := map[string]bool{}
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
			outcome := s.sendToEveryPushSubscription(ctx, subs, n)
			for _, f := range outcome.failed {
				log.Printf("send Notification %s: %v", n.ID, f.err)
			}
			sentIDs = append(sentIDs, n.ID)
			for _, endpoint := range outcome.reached {
				reached[endpoint] = true
			}
		}
	}
	if len(sentIDs) == 0 {
		log.Print("tick: no Notifications due")
	} else {
		log.Printf("tick: sent %s to %s: %s", counted(len(sentIDs), "Notification"),
			counted(len(reached), "Push Subscription"), strings.Join(sentIDs, ", "))
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

// sendOutcome is what came of sending one Notification to every Push
// Subscription.
type sendOutcome struct {
	// reached are the endpoints of the Push Subscriptions that took it.
	reached []string
	removed int
	failed  []sendFailure
}

// sendFailure is a Push Subscription that didn't take a Notification.
type sendFailure struct {
	sub model.PushSubscription
	// status is what the push service answered, or 0 if it didn't.
	status int
	// reason says what went wrong in words that never quote the endpoint.
	reason string
	// err is the whole error, for the log, with the endpoint replaced by its
	// push service's host.
	err error
}

// sendToEveryPushSubscription sends n to each of subs, deleting those whose
// push service no longer knows them, as the push service asks. One that fails
// doesn't stop the others.
func (s *NotificationsService) sendToEveryPushSubscription(ctx context.Context, subs []model.PushSubscription, n model.Notification) sendOutcome {
	var outcome sendOutcome
	for _, sub := range subs {
		err := deliver(ctx, s.vapid, sub, n)
		var answered pushServiceError
		switch {
		case err == nil:
			outcome.reached = append(outcome.reached, sub.Endpoint)
		case errors.As(err, &answered) && answered.gone():
			if err := s.pushSubscriptions.Delete(ctx, sub.Endpoint); err != nil {
				outcome.failed = append(outcome.failed, sendFailure{
					sub: sub, reason: "push service no longer knows it, and it couldn't be removed",
					err: fmt.Errorf("delete a Push Subscription its push service no longer knows: %w", err),
				})
				continue
			}
			log.Printf("removed Push Subscription at %s: %s", pushServiceHost(sub), answered.status())
			outcome.removed++
		default:
			outcome.failed = append(outcome.failed, deliveryFailure(sub, err))
		}
	}
	return outcome
}

// deliveryFailure is sub failing to take a Notification with err, which
// deliver returned. Its reason is one of a few fixed phrases, since the
// errors of an HTTP request quote its URL, the endpoint.
func deliveryFailure(sub model.PushSubscription, err error) sendFailure {
	f := sendFailure{sub: sub, reason: "couldn't reach the push service", err: withoutEndpoint(err, sub)}
	var answered pushServiceError
	switch {
	case errors.As(err, &answered):
		f.status, f.reason = answered.Status, answered.Error()
	case errors.Is(err, context.DeadlineExceeded):
		f.reason = fmt.Sprintf("push service didn't answer within %s", pushTimeout)
	}
	return f
}

// testNotification is the Notification the Owner sends from Settings to see
// that Notifications reach their devices.
var testNotification = model.Notification{Title: "Test Notification", Body: "Notifications reach this device.", URL: "/settings"}

// sendTestNotification sends the test Notification to every Push Subscription
// at once, outside the tick and without recording it as sent, and answers
// what came of it.
func (s *NotificationsService) sendTestNotification(w http.ResponseWriter, r *http.Request) {
	subs, err := s.pushSubscriptions.List(r.Context())
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	outcome := s.sendToEveryPushSubscription(r.Context(), subs, testNotification)
	result := model.TestNotificationResultJSON{
		Sent:    len(outcome.reached),
		Removed: outcome.removed,
		Failed:  make([]model.TestNotificationFailureJSON, 0, len(outcome.failed)),
	}
	for _, f := range outcome.failed {
		log.Printf("send the test Notification: %v", f.err)
		result.Failed = append(result.Failed, model.TestNotificationFailureJSON{
			PushService: pushServiceHost(f.sub), Status: f.status, Reason: f.reason,
		})
	}
	utils.WriteJSON(w, http.StatusOK, result)
}

// pushServiceHost names sub's push service, such as web.push.apple.com,
// without the rest of the endpoint.
func pushServiceHost(sub model.PushSubscription) string {
	endpoint, err := url.Parse(sub.Endpoint)
	if err != nil {
		// Never: an endpoint is checked to be an absolute URL when registered.
		return ""
	}
	return endpoint.Hostname()
}

// counted is n of thing, such as "1 Notification" or "2 Notifications".
func counted(n int, thing string) string {
	if n == 1 {
		return "1 " + thing
	}
	return fmt.Sprintf("%d %ss", n, thing)
}

// withoutEndpoint is err with sub's endpoint, a secret capability URL, which
// an HTTP request's errors quote, replaced by its push service's host.
func withoutEndpoint(err error, sub model.PushSubscription) error {
	return errors.New(strings.ReplaceAll(err.Error(), sub.Endpoint, pushServiceHost(sub)))
}
