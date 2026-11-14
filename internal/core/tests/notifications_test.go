package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/testpush"
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
func (d *browser) subscribe(push *testpush.Service) {
	d.nando.t.Helper()
	rec := d.do(http.MethodPost, "/api/push-subscriptions", mustJSON(d.nando.t, push.SubscriptionJSON()))
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
	n, phone := signedInBrowser(t, wednesday)
	homeScreenApp, safari := testpush.New(t), testpush.New(t)
	phone.subscribe(homeScreenApp)
	phone.subscribe(safari)
	n.core.AddNotificationProvider(&provider{due: []model.Notification{eveningSummary}})

	n.mustTick()

	want := []testpush.Notification{{Title: "Calories", Body: "Nothing logged today", URL: "/calories"}}
	for name, push := range map[string]*testpush.Service{"Home Screen app": homeScreenApp, "Safari": safari} {
		if got := push.Received(); !slices.Equal(got, want) {
			t.Errorf("%s received %+v, want %+v", name, got, want)
		}
	}
}

func TestARetriedTickSendsEachNotificationOnlyOnce(t *testing.T) {
	n, phone := signedInBrowser(t, wednesday)
	push := testpush.New(t)
	phone.subscribe(push)
	n.core.AddNotificationProvider(&provider{due: []model.Notification{eveningSummary}})

	n.mustTick()
	n.clock.Advance(time.Minute) // Cloud Scheduler retries a tick it thinks failed
	n.mustTick()

	if got := push.Received(); len(got) != 1 {
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
			n, phone := signedInBrowser(t, wednesday)
			push := testpush.New(t)
			phone.subscribe(push)
			p := &provider{due: []model.Notification{eveningSummary}}
			n.core.AddNotificationProvider(p)
			push.Answer(tc.status)
			n.mustTick()

			push.Answer(http.StatusCreated)
			p.due = []model.Notification{{ID: "calories:evening-summary:2026-10-21", Title: "Calories"}}
			n.clock.Advance(24 * time.Hour)
			n.mustTick()

			if got := len(push.Received()) == 1; got != tc.keepsTrying {
				t.Errorf("after the push service answered %d, next Notification delivered = %v, want %v", tc.status, got, tc.keepsTrying)
			}
			if got := push.RequestCount(); got != 1 && !tc.keepsTrying {
				t.Errorf("push service got %d requests, want 1", got)
			}
		})
	}
}

func TestAnUnregisteredPushSubscriptionGetsNoNotifications(t *testing.T) {
	n, phone := signedInBrowser(t, wednesday)
	push := testpush.New(t)
	phone.subscribe(push)
	n.core.AddNotificationProvider(&provider{due: []model.Notification{eveningSummary}})

	rec := phone.do(http.MethodDelete, "/api/push-subscriptions?endpoint="+url.QueryEscape(push.Endpoint()), nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE /api/push-subscriptions: status = %d, body %q", rec.Code, rec.Body)
	}
	n.mustTick()

	if got := push.RequestCount(); got != 0 {
		t.Errorf("push service got %d push messages after unregistering, want 0", got)
	}
}

func TestInvalidPushSubscriptionsAreRejected(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	valid := testpush.New(t).SubscriptionJSON()
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
	n, phone := signedInBrowser(t, wednesday)
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
	push := testpush.New(t)
	phone.subscribe(push)
	n.core.AddNotificationProvider(&provider{due: []model.Notification{eveningSummary}})

	n.mustTick()

	if got := push.VAPIDKeys(); !slices.Equal(got, []string{key.PublicKey}) {
		t.Errorf("push messages signed with VAPID keys %q, want [%q]", got, key.PublicKey)
	}
}

// saveWeighInReminder sets the Weigh-in reminder, keeping the other settings.
func (d *browser) saveWeighInReminder(r reminderJSON) {
	d.nando.t.Helper()
	s := d.getSettings()
	s.WeighInReminder = r
	if rec := d.do(http.MethodPut, "/api/settings", mustJSON(d.nando.t, s)); rec.Code != http.StatusNoContent {
		d.nando.t.Fatalf("PUT /api/settings with reminder %+v: status = %d, body %q", r, rec.Code, rec.Body)
	}
}

var weighInReminder = testpush.Notification{Title: "Weigh-in", Body: "Time to log this week's weight", URL: "/"}

func TestWeighInReminderIsSentAtItsTimeEachWeek(t *testing.T) {
	tokyo, _ := time.LoadLocation("Asia/Tokyo")
	mondayMorning := time.Date(2026, 10, 26, 8, 0, 0, 0, tokyo) // Sunday 23:00 UTC
	n, phone := signedInBrowser(t, mondayMorning.Add(-30*time.Minute))
	push := testpush.New(t)
	phone.subscribe(push)
	phone.saveTimeZone("Asia/Tokyo")
	phone.saveWeighInReminder(reminderJSON{Enabled: true, Weekday: "monday", Time: "08:00"})

	n.mustTick() // 07:30
	if got := push.Received(); len(got) != 0 {
		t.Fatalf("at 07:30 received %+v, want nothing yet", got)
	}

	n.clock.Set(mondayMorning)
	n.mustTick()
	n.clock.Advance(2 * time.Minute) // a retried tick
	n.mustTick()
	n.clock.Advance(28 * time.Minute) // 08:30
	n.mustTick()
	if got := push.Received(); !slices.Equal(got, []testpush.Notification{weighInReminder}) {
		t.Fatalf("on Monday by 08:30 received %+v, want one %+v", got, weighInReminder)
	}

	n.clock.Set(mondayMorning.AddDate(0, 0, 7))
	n.mustTick()
	if got := len(push.Received()); got != 2 {
		t.Errorf("by the next Monday's 08:00 received %d reminders, want 2", got)
	}
}

func TestDisabledWeighInReminderIsNotSent(t *testing.T) {
	monday8am := time.Date(2026, 10, 26, 8, 0, 0, 0, time.UTC)
	n, phone := signedInBrowser(t, monday8am)
	push := testpush.New(t)
	phone.subscribe(push)
	phone.saveWeighInReminder(reminderJSON{Enabled: false, Weekday: "monday", Time: "08:00"})

	n.mustTick()

	if got := push.Received(); len(got) != 0 {
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
	n, phone := signedInBrowser(t, wednesday)
	push := testpush.New(t)
	phone.subscribe(push)
	n.core.AddNotificationProvider(brokenProvider{})
	n.core.AddNotificationProvider(&provider{due: []model.Notification{eveningSummary}})

	rec := n.tick(schedulerToken)

	if got := len(push.Received()); got != 1 {
		t.Errorf("received %d push messages, want the working provider's 1", got)
	}
	// Failing the tick has Cloud Scheduler retry it, which is safe.
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("tick with a broken provider: status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestWeighInReminderIsSentInATimeZoneOffsetByAQuarterHour(t *testing.T) {
	kathmandu, _ := time.LoadLocation("Asia/Kathmandu") // UTC+5:45
	mondayMorning := time.Date(2026, 10, 26, 8, 0, 0, 0, kathmandu)
	n, phone := signedInBrowser(t, mondayMorning)
	push := testpush.New(t)
	phone.subscribe(push)
	phone.saveTimeZone("Asia/Kathmandu")
	phone.saveWeighInReminder(reminderJSON{Enabled: true, Weekday: "monday", Time: "08:00"})

	n.mustTick()

	if got := push.Received(); !slices.Equal(got, []testpush.Notification{weighInReminder}) {
		t.Errorf("at 08:00 in Kathmandu received %+v, want %+v", got, weighInReminder)
	}
}

func TestANotificationWithoutATitleOrLinkShowsTheDefaults(t *testing.T) {
	n, phone := signedInBrowser(t, wednesday)
	push := testpush.New(t)
	phone.subscribe(push)
	n.core.AddNotificationProvider(&provider{due: []model.Notification{
		{ID: "calories:nothing-logged:2026-10-21", Body: "Nothing logged today"},
	}})

	n.mustTick()

	want := []testpush.Notification{{Title: "Nando", Body: "Nothing logged today", URL: "/"}}
	if got := push.Received(); !slices.Equal(got, want) {
		t.Errorf("the service worker shows %+v, want %+v", got, want)
	}
}

func TestANotificationCoreCannotRecordDoesNotStopTheOthers(t *testing.T) {
	n, phone := signedInBrowser(t, wednesday)
	push := testpush.New(t)
	phone.subscribe(push)
	// An ID too big for a MongoDB document (16 MB), so recording it as sent fails.
	unrecordable := model.Notification{ID: strings.Repeat("x", 17<<20), Title: "Calories"}
	n.core.AddNotificationProvider(&provider{due: []model.Notification{unrecordable}})
	n.core.AddNotificationProvider(&provider{due: []model.Notification{eveningSummary}})

	rec := n.tick(schedulerToken)

	if got := push.Received(); !slices.Equal(got, []testpush.Notification{{Title: "Calories", Body: "Nothing logged today", URL: "/calories"}}) {
		t.Errorf("received %+v, want only the evening summary", got)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("tick: status = %d, want %d so the scheduler retries", rec.Code, http.StatusInternalServerError)
	}
}

// tickJob is a Module's tick job: it counts its runs and fails with err.
type tickJob struct {
	runs int
	err  error
}

func (j *tickJob) RunAtTick(context.Context) error {
	j.runs++
	return j.err
}

func TestATickJobThatFailsStopsNoNotificationNorAnotherJob(t *testing.T) {
	n, phone := signedInBrowser(t, wednesday)
	push := testpush.New(t)
	phone.subscribe(push)
	failing, next := &tickJob{err: errors.New("the database is unreachable")}, &tickJob{}
	n.core.AddTickJob(failing)
	n.core.AddTickJob(next)
	n.core.AddNotificationProvider(&provider{due: []model.Notification{eveningSummary}})

	rec := n.tick(schedulerToken)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("tick with a failing job: status = %d, want %d so that the scheduler retries it", rec.Code, http.StatusInternalServerError)
	}
	if failing.runs != 1 || next.runs != 1 || len(push.Received()) != 1 {
		t.Errorf("the jobs ran %d and %d times and %d Notifications were sent; want each job once and the Notification sent",
			failing.runs, next.runs, len(push.Received()))
	}
}

// testNotificationResultJSON is what POST /api/push-subscriptions/test answers.
type testNotificationResultJSON struct {
	Sent    int `json:"sent"`
	Removed int `json:"removed"`
	Failed  []struct {
		PushService string `json:"pushService"`
		Status      int    `json:"status"`
		Reason      string `json:"reason"`
	} `json:"failed"`
}

// sendTestNotification sends the test Notification from Settings.
func (d *browser) sendTestNotification() testNotificationResultJSON {
	d.nando.t.Helper()
	rec := d.do(http.MethodPost, "/api/push-subscriptions/test", nil)
	if rec.Code != http.StatusOK {
		d.nando.t.Fatalf("POST /api/push-subscriptions/test: status = %d, body %q; want 200", rec.Code, rec.Body)
	}
	var result testNotificationResultJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		d.nando.t.Fatalf("POST /api/push-subscriptions/test: %v; body %q", err, rec.Body)
	}
	return result
}

// theTestNotification is the test Notification as the device shows it.
var theTestNotification = testpush.Notification{Title: "Test Notification", Body: "Notifications reach this device.", URL: "/settings"}

func TestATestNotificationReachesEveryPushSubscriptionAtOnce(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	homeScreenApp, mac := testpush.New(t), testpush.New(t)
	phone.subscribe(homeScreenApp)
	phone.subscribe(mac)

	result := phone.sendTestNotification()

	if result.Sent != 2 || result.Removed != 0 || len(result.Failed) != 0 {
		t.Errorf("answer = %+v; want sent to both Push Subscriptions", result)
	}
	for name, push := range map[string]*testpush.Service{"Home Screen app": homeScreenApp, "Mac": mac} {
		if got := push.Received(); !slices.Equal(got, []testpush.Notification{theTestNotification}) {
			t.Errorf("%s received %+v; want the test Notification", name, got)
		}
	}
}

func TestATestNotificationCanBeSentAgainRightAway(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	push := testpush.New(t)
	phone.subscribe(push)

	phone.sendTestNotification()
	again := phone.sendTestNotification()

	if again.Sent != 1 {
		t.Errorf("second answer = %+v; want sent again", again)
	}
	if got := push.Received(); !slices.Equal(got, []testpush.Notification{theTestNotification, theTestNotification}) {
		t.Errorf("received %+v; want the test Notification twice", got)
	}
}

func TestATestNotificationDeletesAPushSubscriptionThePushServiceNoLongerKnows(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusGone} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			_, phone := signedInBrowser(t, wednesday)
			uninstalled, mac := testpush.New(t), testpush.New(t)
			phone.subscribe(uninstalled)
			phone.subscribe(mac)
			uninstalled.Answer(status)

			result := phone.sendTestNotification()

			if result.Sent != 1 || result.Removed != 1 || len(result.Failed) != 0 {
				t.Errorf("answer = %+v; want sent to the Mac and the unknown Push Subscription removed", result)
			}
			phone.sendTestNotification()
			if got := uninstalled.RequestCount(); got != 1 {
				t.Errorf("removed Push Subscription was sent %d push messages; want 1, then none", got)
			}
		})
	}
}

func TestATestNotificationReportsAFailingPushServiceByItsHost(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	failing := testpush.New(t)
	phone.subscribe(failing)
	failing.Answer(http.StatusInternalServerError)

	result := phone.sendTestNotification()

	endpoint, err := url.Parse(failing.Endpoint())
	if err != nil {
		t.Fatal(err)
	}
	if result.Sent != 0 || result.Removed != 0 || len(result.Failed) != 1 {
		t.Fatalf("answer = %+v; want one failure", result)
	}
	failed := result.Failed[0]
	if failed.PushService != endpoint.Hostname() || failed.Status != http.StatusInternalServerError ||
		failed.Reason != "push service answered 500 Internal Server Error" {
		t.Errorf("failure = %+v; want push service %s answering 500", failed, endpoint.Hostname())
	}
	phone.sendTestNotification()
	if got := failing.RequestCount(); got != 2 {
		t.Errorf("failing Push Subscription was sent %d push messages; want it kept and sent another", got)
	}
}

func TestATestNotificationWithNoPushSubscriptionSendsNothing(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)

	rec := phone.do(http.MethodPost, "/api/push-subscriptions/test", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/push-subscriptions/test: status = %d, body %q; want 200", rec.Code, rec.Body)
	}
	var answer map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if failed, ok := answer["failed"].([]any); answer["sent"] != 0.0 || answer["removed"] != 0.0 || !ok || len(failed) != 0 {
		t.Errorf("answer = %s; want nothing sent and an empty failed list", rec.Body)
	}
}

func TestATestNotificationNeedsASession(t *testing.T) {
	n, phone := signedInBrowser(t, wednesday)
	push := testpush.New(t)
	phone.subscribe(push)

	rec := n.newBrowser("Stranger").do(http.MethodPost, "/api/push-subscriptions/test", nil)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("POST /api/push-subscriptions/test without a Session: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if got := push.RequestCount(); got != 0 {
		t.Errorf("Push Subscription was sent %d push messages; want none", got)
	}
}

func TestATestNotificationGivesUpOnAPushServiceThatDoesNotAnswer(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	silent, mac := testpush.New(t), testpush.New(t)
	phone.subscribe(silent)
	phone.subscribe(mac)
	silent.Hang()

	start := time.Now()
	result := phone.sendTestNotification()

	if took := time.Since(start); took > 15*time.Second {
		t.Errorf("answered after %s; want within 15s", took.Round(time.Second))
	}
	if result.Sent != 1 || len(result.Failed) != 1 {
		t.Fatalf("answer = %+v; want sent to the Mac and one failure", result)
	}
	if failed := result.Failed[0]; failed.Status != 0 || failed.Reason != "push service didn't answer within 10s" {
		t.Errorf("failure = %+v; want no status and that it didn't answer", failed)
	}
}

// captureLog has the log write to the returned buffer until the test ends.
func captureLog(t *testing.T) *lockedBuffer {
	t.Helper()
	buf := &lockedBuffer{}
	prev := log.Writer()
	log.SetOutput(buf)
	t.Cleanup(func() { log.SetOutput(prev) })
	return buf
}

// lockedBuffer is a bytes.Buffer safe to write from several goroutines.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// noEntriesReminder is a second Notification due with eveningSummary.
var noEntriesReminder = model.Notification{ID: "calories:no-entries:2026-10-20", Title: "Calories", Body: "Nothing logged yet"}

func TestATickLogsWhichNotificationsItSent(t *testing.T) {
	n, phone := signedInBrowser(t, wednesday)
	homeScreenApp, safari := testpush.New(t), testpush.New(t)
	phone.subscribe(homeScreenApp)
	phone.subscribe(safari)
	n.core.AddNotificationProvider(&provider{due: []model.Notification{noEntriesReminder, eveningSummary}})
	logs := captureLog(t)

	n.mustTick()

	want := "tick: sent 2 Notifications to 2 Push Subscriptions: calories:no-entries:2026-10-20 (reached 2 of 2), calories:evening-summary:2026-10-20 (reached 2 of 2)\n"
	if !strings.Contains(logs.String(), want) {
		t.Errorf("log = %q; want a line %q", logs, want)
	}
}

func TestATickLogsANotificationThatReachedNoPushSubscription(t *testing.T) {
	n := newNando(t)
	n.core.AddNotificationProvider(&provider{due: []model.Notification{eveningSummary}})
	logs := captureLog(t)

	n.mustTick()

	want := "tick: sent 1 Notification to 0 Push Subscriptions: calories:evening-summary:2026-10-20 (reached 0 of 0)\n"
	if !strings.Contains(logs.String(), want) {
		t.Errorf("log = %q; want a line %q", logs, want)
	}
}

func TestATickWithNothingDueSaysSo(t *testing.T) {
	n, phone := signedInBrowser(t, wednesday)
	phone.subscribe(testpush.New(t))
	n.core.AddNotificationProvider(&provider{due: []model.Notification{eveningSummary}})
	n.mustTick()
	logs := captureLog(t)

	n.clock.Advance(time.Minute) // a retried tick: the evening summary was sent already
	n.mustTick()

	if got, want := logs.String(), "tick: no Notifications due\n"; !strings.HasSuffix(got, want) || strings.Contains(got, "tick: sent") {
		t.Errorf("log = %q; want only %q", got, want)
	}
}

func TestATickLogsAPushSubscriptionItRemovesByItsPushServiceHost(t *testing.T) {
	for _, status := range []int{http.StatusGone, http.StatusNotFound} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			n, phone := signedInBrowser(t, wednesday)
			uninstalled := testpush.New(t)
			phone.subscribe(uninstalled)
			uninstalled.Answer(status)
			n.core.AddNotificationProvider(&provider{due: []model.Notification{eveningSummary}})
			logs := captureLog(t)

			n.mustTick()

			endpoint, err := url.Parse(uninstalled.Endpoint())
			if err != nil {
				t.Fatal(err)
			}
			want := map[int]string{
				http.StatusGone:     "removed Push Subscription at 127.0.0.1: 410 Gone\n",
				http.StatusNotFound: "removed Push Subscription at 127.0.0.1: 404 Not Found\n",
			}[status]
			if got := logs.String(); !strings.Contains(got, want) {
				t.Errorf("log = %q; want a line %q", got, want)
			}
			if got := logs.String(); strings.Contains(got, endpoint.Path) {
				t.Errorf("log = %q; gives away the endpoint", got)
			}
		})
	}
}

func TestATickNeverLogsWhatANotificationSays(t *testing.T) {
	n, phone := signedInBrowser(t, wednesday)
	delivered, uninstalled, failing := testpush.New(t), testpush.New(t), testpush.New(t)
	phone.subscribe(delivered)
	phone.subscribe(uninstalled)
	phone.subscribe(failing)
	uninstalled.Answer(http.StatusGone)
	failing.Answer(http.StatusInternalServerError)
	private := model.Notification{ID: "japanese:word-request-failed:abc", Title: "Couldn't add a word", Body: "ぬぺぽぴゃ: not a word Jisho knows"}
	n.core.AddNotificationProvider(&provider{due: []model.Notification{private}})
	logs := captureLog(t)

	n.mustTick()

	for _, text := range []string{private.Title, private.Body, "ぬぺぽぴゃ"} {
		if got := logs.String(); strings.Contains(got, text) {
			t.Errorf("log = %q; gives away %q", got, text)
		}
	}
}

func TestATickLogsAPushServiceItCannotReachWithoutTheEndpoint(t *testing.T) {
	n, phone := signedInBrowser(t, wednesday)
	unreachable := testpush.New(t)
	phone.subscribe(unreachable)
	unreachable.Stop()
	n.core.AddNotificationProvider(&provider{due: []model.Notification{eveningSummary}})
	logs := captureLog(t)

	n.mustTick()

	endpoint, err := url.Parse(unreachable.Endpoint())
	if err != nil {
		t.Fatal(err)
	}
	got := logs.String()
	if !strings.Contains(got, "send Notification calories:evening-summary:2026-10-20: ") || !strings.Contains(got, endpoint.Hostname()) {
		t.Errorf("log = %q; want the failed send, naming the push service %s", got, endpoint.Hostname())
	}
	if strings.Contains(got, endpoint.Path) {
		t.Errorf("log = %q; gives away the endpoint", got)
	}
}

func TestATickSendsNoMoreToAPushSubscriptionItRemoved(t *testing.T) {
	n, phone := signedInBrowser(t, wednesday)
	uninstalled, mac := testpush.New(t), testpush.New(t)
	phone.subscribe(uninstalled)
	phone.subscribe(mac)
	uninstalled.Answer(http.StatusGone)
	n.core.AddNotificationProvider(&provider{due: []model.Notification{noEntriesReminder, eveningSummary}})
	logs := captureLog(t)

	n.mustTick()

	if got := uninstalled.RequestCount(); got != 1 {
		t.Errorf("removed Push Subscription was sent %d push messages; want 1", got)
	}
	got := logs.String()
	if count := strings.Count(got, "removed Push Subscription at 127.0.0.1: 410 Gone\n"); count != 1 {
		t.Errorf("log = %q; want the removal logged once, not %d times", got, count)
	}
	want := "tick: sent 2 Notifications to 2 Push Subscriptions: calories:no-entries:2026-10-20 (reached 1 of 2), calories:evening-summary:2026-10-20 (reached 1 of 1)\n"
	if !strings.Contains(got, want) {
		t.Errorf("log = %q; want a line %q", got, want)
	}
}

func TestATickWithErrorsDoesNotSayNothingWasDue(t *testing.T) {
	for _, c := range []struct {
		name string
		due  []model.Notification
		want string
	}{
		{name: "nothing sent", want: "tick: no Notifications sent, 1 error\n"},
		{
			name: "something sent",
			due:  []model.Notification{eveningSummary},
			want: "tick: sent 1 Notification to 1 Push Subscription, 1 error: calories:evening-summary:2026-10-20 (reached 1 of 1)\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			n, phone := signedInBrowser(t, wednesday)
			phone.subscribe(testpush.New(t))
			n.core.AddNotificationProvider(brokenProvider{})
			n.core.AddNotificationProvider(&provider{due: c.due})
			logs := captureLog(t)

			n.tick(schedulerToken)

			if got := logs.String(); !strings.Contains(got, c.want) || strings.Contains(got, "no Notifications due") {
				t.Errorf("log = %q; want a line %q", got, c.want)
			}
		})
	}
}

func TestATickThatCannotReadThePushSubscriptionsStillLogsItsLine(t *testing.T) {
	n, phone := signedInBrowser(t, wednesday)
	phone.subscribe(testpush.New(t))
	n.core.AddNotificationProvider(&provider{due: []model.Notification{eveningSummary}})
	// A Push Subscription whose keys aren't text, so reading them fails.
	if _, err := n.db.Collection("push_subscriptions").InsertOne(context.Background(), map[string]any{"_id": "https://broken.example/push", "p256dh": 5}); err != nil {
		t.Fatal(err)
	}
	logs := captureLog(t)

	rec := n.tick(schedulerToken)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("tick: status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if got, want := logs.String(), "tick: no Notifications sent, 1 error\n"; !strings.Contains(got, want) {
		t.Errorf("log = %q; want a line %q", got, want)
	}
}

func TestATickLogsAPushServiceThatRedirectsNowhereWithoutAnyEndpoint(t *testing.T) {
	n, phone := signedInBrowser(t, wednesday)
	redirecting, gone := testpush.New(t), testpush.New(t)
	phone.subscribe(redirecting)
	gone.Stop()
	redirecting.Redirect(gone.Endpoint())
	n.core.AddNotificationProvider(&provider{due: []model.Notification{eveningSummary}})
	logs := captureLog(t)

	n.mustTick()

	got := logs.String()
	if !strings.Contains(got, "send Notification calories:evening-summary:2026-10-20: ") {
		t.Errorf("log = %q; want the failed send", got)
	}
	if strings.Contains(got, "/push/") {
		t.Errorf("log = %q; gives away an endpoint", got)
	}
}
