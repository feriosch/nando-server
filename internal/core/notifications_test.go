package core_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/feriosch/nando-server/internal/core/model"
)

// schedulerToken is the only token the fake verifier accepts, standing in for
// a Google-signed OIDC token from the scheduler's service account.
const schedulerToken = "google-signed-scheduler-token"

func verifySchedulerToken(_ context.Context, token string) error {
	if token != schedulerToken {
		return errors.New("not the scheduler's token")
	}
	return nil
}

// tick calls POST /internal/tick as Cloud Scheduler does, with token as its
// bearer token ("" sends no Authorization header).
func (n *nando) tick(token string) *httptest.ResponseRecorder {
	n.t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/internal/tick", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	n.handler.ServeHTTP(rec, req)
	return rec
}

func TestTickWithoutTheSchedulersTokenIsRejected(t *testing.T) {
	n := newNando(t)

	for name, token := range map[string]string{"no token": "", "another token": "forged"} {
		if rec := n.tick(token); rec.Code != http.StatusUnauthorized {
			t.Errorf("tick with %s: status = %d, want %d", name, rec.Code, http.StatusUnauthorized)
		}
	}
	if rec := n.tick(schedulerToken); rec.Code != http.StatusNoContent {
		t.Errorf("tick with the scheduler's token: status = %d, body %q; want %d", rec.Code, rec.Body, http.StatusNoContent)
	}
}

// provider is a Module's Notification provider that has whatever
// Notifications a test says are due.
type provider struct{ due []model.Notification }

func (p *provider) DueNotifications(context.Context, time.Time) ([]model.Notification, error) {
	return p.due, nil
}

// subscribe registers push's Push Subscription, as the web app does once the
// Owner allows Notifications on this device.
func (d *device) subscribe(push *pushService) {
	d.nando.t.Helper()
	rec := d.do(http.MethodPost, "/api/push-subscriptions", mustJSON(d.nando.t, push.subscriptionJSON()))
	if rec.Code != http.StatusNoContent {
		d.nando.t.Fatalf("POST /api/push-subscriptions: status = %d, body %q", rec.Code, rec.Body)
	}
}

// mustTick ticks as the scheduler and fails the test unless the tick succeeds.
func (n *nando) mustTick() {
	n.t.Helper()
	if rec := n.tick(schedulerToken); rec.Code != http.StatusNoContent {
		n.t.Fatalf("tick: status = %d, body %q", rec.Code, rec.Body)
	}
}

var eveningSummary = model.Notification{
	ID:    "calories:evening-summary:2026-10-20",
	Title: "Calories",
	Body:  "Nothing logged today",
	URL:   "/calories",
}

func TestTickSendsDueNotificationsToEveryPushSubscription(t *testing.T) {
	n, phone := signedInPhone(t, wednesday)
	homeScreenApp, safari := newPushService(t), newPushService(t)
	phone.subscribe(homeScreenApp)
	phone.subscribe(safari)
	n.core.AddNotificationProvider(&provider{due: []model.Notification{eveningSummary}})

	n.mustTick()

	want := []pushMessage{{Title: "Calories", Body: "Nothing logged today", URL: "/calories"}}
	for name, push := range map[string]*pushService{"Home Screen app": homeScreenApp, "Safari": safari} {
		if got := push.received(); !slices.Equal(got, want) {
			t.Errorf("%s received %+v, want %+v", name, got, want)
		}
	}
}

func TestARetriedTickSendsEachNotificationOnlyOnce(t *testing.T) {
	n, phone := signedInPhone(t, wednesday)
	push := newPushService(t)
	phone.subscribe(push)
	n.core.AddNotificationProvider(&provider{due: []model.Notification{eveningSummary}})

	n.mustTick()
	n.clock.Advance(time.Minute) // Cloud Scheduler retries a tick it thinks failed
	n.mustTick()

	if got := push.received(); len(got) != 1 {
		t.Errorf("received %d push messages, want 1: %+v", len(got), got)
	}
}

func TestAPushSubscriptionThePushServiceNoLongerKnowsIsDeleted(t *testing.T) {
	for _, tc := range []struct {
		status      int
		keepsTrying bool
	}{
		{status: http.StatusGone},
		{status: http.StatusNotFound},
		{status: http.StatusInternalServerError, keepsTrying: true},
	} {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			n, phone := signedInPhone(t, wednesday)
			push := newPushService(t)
			phone.subscribe(push)
			p := &provider{due: []model.Notification{eveningSummary}}
			n.core.AddNotificationProvider(p)
			push.answer(tc.status)
			n.mustTick()

			push.answer(http.StatusCreated)
			p.due = []model.Notification{{ID: "calories:evening-summary:2026-10-21", Title: "Calories"}}
			n.clock.Advance(24 * time.Hour)
			n.mustTick()

			if got := len(push.received()) == 1; got != tc.keepsTrying {
				t.Errorf("after the push service answered %d, next Notification delivered = %v, want %v", tc.status, got, tc.keepsTrying)
			}
			if got := push.requestCount(); got != 1 && !tc.keepsTrying {
				t.Errorf("push service got %d requests, want 1", got)
			}
		})
	}
}

func TestAnUnregisteredPushSubscriptionGetsNoNotifications(t *testing.T) {
	n, phone := signedInPhone(t, wednesday)
	push := newPushService(t)
	phone.subscribe(push)
	n.core.AddNotificationProvider(&provider{due: []model.Notification{eveningSummary}})

	rec := phone.do(http.MethodDelete, "/api/push-subscriptions?endpoint="+url.QueryEscape(push.endpoint()), nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE /api/push-subscriptions: status = %d, body %q", rec.Code, rec.Body)
	}
	n.mustTick()

	if got := push.requestCount(); got != 0 {
		t.Errorf("push service got %d push messages after unregistering, want 0", got)
	}
}

func TestInvalidPushSubscriptionsAreRejected(t *testing.T) {
	_, phone := signedInPhone(t, wednesday)
	valid := newPushService(t).subscriptionJSON()
	keys := valid["keys"].(map[string]string)
	with := func(change func(sub map[string]any, keys map[string]string)) []byte {
		sub := map[string]any{"endpoint": valid["endpoint"]}
		k := map[string]string{"p256dh": keys["p256dh"], "auth": keys["auth"]}
		change(sub, k)
		sub["keys"] = k
		return mustJSON(t, sub)
	}

	for name, body := range map[string][]byte{
		"not JSON":         []byte("{"),
		"no endpoint":      with(func(sub map[string]any, _ map[string]string) { delete(sub, "endpoint") }),
		"relative URL":     with(func(sub map[string]any, _ map[string]string) { sub["endpoint"] = "/push/1" }),
		"no p256dh":        with(func(_ map[string]any, k map[string]string) { delete(k, "p256dh") }),
		"p256dh not a key": with(func(_ map[string]any, k map[string]string) { k["p256dh"] = "c2hvcnQ" }),
		"no auth":          with(func(_ map[string]any, k map[string]string) { delete(k, "auth") }),
		"auth too short":   with(func(_ map[string]any, k map[string]string) { k["auth"] = "c2hvcnQ" }),
	} {
		if rec := phone.do(http.MethodPost, "/api/push-subscriptions", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want %d", name, rec.Code, http.StatusBadRequest)
		}
	}
}

func TestPushMessagesAreSignedWithTheVAPIDKeyTheWebAppSubscribesWith(t *testing.T) {
	n, phone := signedInPhone(t, wednesday)
	rec := phone.do(http.MethodGet, "/api/push-subscriptions/vapid-public-key", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/push-subscriptions/vapid-public-key: status = %d, body %q", rec.Code, rec.Body)
	}
	var key struct {
		PublicKey string `json:"publicKey"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &key); err != nil || key.PublicKey == "" {
		t.Fatalf("vapid-public-key body %q: %v", rec.Body, err)
	}
	push := newPushService(t)
	phone.subscribe(push)
	n.core.AddNotificationProvider(&provider{due: []model.Notification{eveningSummary}})

	n.mustTick()

	if got := push.vapidKeys(); !slices.Equal(got, []string{key.PublicKey}) {
		t.Errorf("push messages signed with VAPID keys %q, want [%q]", got, key.PublicKey)
	}
}

// saveWeighInReminder sets the Weigh-in reminder, keeping the other settings.
func (d *device) saveWeighInReminder(r reminderJSON) {
	d.nando.t.Helper()
	s := d.getSettings()
	s.WeighInReminder = r
	if rec := d.do(http.MethodPut, "/api/settings", mustJSON(d.nando.t, s)); rec.Code != http.StatusNoContent {
		d.nando.t.Fatalf("PUT /api/settings with reminder %+v: status = %d, body %q", r, rec.Code, rec.Body)
	}
}

var weighInReminder = pushMessage{Title: "Weigh-in", Body: "Time to log this week's weight", URL: "/"}

func TestWeighInReminderIsSentAtItsTimeEachWeek(t *testing.T) {
	tokyo, _ := time.LoadLocation("Asia/Tokyo")
	mondayMorning := time.Date(2026, 10, 26, 8, 0, 0, 0, tokyo) // Sunday 23:00 UTC
	n, phone := signedInPhone(t, mondayMorning.Add(-30*time.Minute))
	push := newPushService(t)
	phone.subscribe(push)
	phone.saveTimeZone("Asia/Tokyo")
	phone.saveWeighInReminder(reminderJSON{Enabled: true, Weekday: "monday", Time: "08:00"})

	n.mustTick() // 07:30
	if got := push.received(); len(got) != 0 {
		t.Fatalf("at 07:30 received %+v, want nothing yet", got)
	}

	n.clock.Set(mondayMorning)
	n.mustTick()
	n.clock.Advance(2 * time.Minute) // a retried tick
	n.mustTick()
	n.clock.Advance(28 * time.Minute) // 08:30
	n.mustTick()
	if got := push.received(); !slices.Equal(got, []pushMessage{weighInReminder}) {
		t.Fatalf("on Monday by 08:30 received %+v, want one %+v", got, weighInReminder)
	}

	n.clock.Set(mondayMorning.AddDate(0, 0, 7))
	n.mustTick()
	if got := len(push.received()); got != 2 {
		t.Errorf("by the next Monday's 08:00 received %d reminders, want 2", got)
	}
}

func TestDisabledWeighInReminderIsNotSent(t *testing.T) {
	monday8am := time.Date(2026, 10, 26, 8, 0, 0, 0, time.UTC)
	n, phone := signedInPhone(t, monday8am)
	push := newPushService(t)
	phone.subscribe(push)
	phone.saveWeighInReminder(reminderJSON{Enabled: false, Weekday: "monday", Time: "08:00"})

	n.mustTick()

	if got := push.received(); len(got) != 0 {
		t.Errorf("received %+v, want nothing", got)
	}
}

// brokenProvider is a Module's provider that fails, such as when its database
// is briefly unreachable.
type brokenProvider struct{}

func (brokenProvider) DueNotifications(context.Context, time.Time) ([]model.Notification, error) {
	return nil, errors.New("database unreachable")
}

func TestABrokenProviderDoesNotStopTheOthers(t *testing.T) {
	n, phone := signedInPhone(t, wednesday)
	push := newPushService(t)
	phone.subscribe(push)
	n.core.AddNotificationProvider(brokenProvider{})
	n.core.AddNotificationProvider(&provider{due: []model.Notification{eveningSummary}})

	rec := n.tick(schedulerToken)

	if got := len(push.received()); got != 1 {
		t.Errorf("received %d push messages, want the working provider's 1", got)
	}
	// Failing the tick has Cloud Scheduler retry it, which is safe.
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("tick with a broken provider: status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}
