package tests

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/feriosch/nando-server/internal/core"
	"github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/testtelegram"
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

// mustTick ticks as the scheduler and fails the test unless the tick succeeds.
func (n *nando) mustTick() {
	n.t.Helper()
	if rec := n.tick(schedulerToken); rec.Code != http.StatusNoContent {
		n.t.Fatalf("tick: status = %d, body %q", rec.Code, rec.Body)
	}
}

// provider is a Module's Notification provider that has whatever
// Notifications a test says are due.
type provider struct{ due []model.Notification }

func (p *provider) DueNotifications(context.Context, time.Time) ([]model.Notification, error) {
	return p.due, nil
}

var eveningSummary = model.Notification{
	ID:    "calories:evening-summary:2026-10-20",
	Title: "Calories",
	Body:  "Nothing logged today",
}

// noEntriesReminder is a second Notification due with eveningSummary.
var noEntriesReminder = model.Notification{ID: "calories:no-entries:2026-10-20", Title: "Calories", Body: "Nothing logged yet"}

// signedInWithBots returns Nando whose Messenger has bots for the Core and
// Career Assistants, sending to a fake Telegram, and a browser on the Owner's
// iPhone signed in, its clock set to now.
func signedInWithBots(t *testing.T, now time.Time) (*nando, *browser, *testtelegram.Telegram) {
	t.Helper()
	telegram := testtelegram.New(t)
	n := newNandoWith(t, func(cfg *core.Config) {
		cfg.Messenger = core.MessengerConfig{
			TelegramURL:   telegram.URL,
			OwnerID:       ownerTelegramID,
			WebhookSecret: webhookSecret,
			BotTokens:     map[string]string{"core": coreBotToken, "career": careerBotToken},
		}
	})
	n.core.AddAssistant(model.Assistant{Name: "career"})
	phone := n.newBrowser("iPhone")
	phone.mustEnroll()
	n.clock.Set(now)
	return n, phone, telegram
}

// received is every Notification the Owner got in Telegram so far, as the
// bots sent them: the title, a line break and the body.
func received(telegram *testtelegram.Telegram) []string {
	var texts []string
	for _, c := range telegram.CallsTo("sendMessage") {
		texts = append(texts, c.Params["text"].(string))
	}
	return texts
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

func TestWithNoBotADueNotificationWaitsUnsentAndTheTickSaysWhy(t *testing.T) {
	n := newNando(t) // local development: no Assistant has a bot
	n.core.AddNotificationProvider(&provider{due: []model.Notification{eveningSummary}})
	logs := captureLog(t)

	n.mustTick() // 204: nothing failed
	n.clock.Advance(time.Minute)
	n.mustTick()

	// Not recorded as sent: the second tick finds it still due, rather than
	// "no Notifications due".
	want := "tick: 1 Notification due, but no Assistant has a bot to send them\n"
	if got := logs.String(); strings.Count(got, want) != 2 || strings.Contains(got, "no Notifications due") {
		t.Errorf("log = %q; want %q at both ticks", got, want)
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

// weighInReminder is the Weigh-in reminder as the Core bot sends it.
const weighInReminder = "Weigh-in\nTime to log this week's weight"

func TestANotificationWithoutATitleIsSentAsItsBodyAlone(t *testing.T) {
	n, _, telegram := signedInWithBots(t, wednesday)
	n.core.AddNotificationProvider(&provider{due: []model.Notification{{ID: "calories:no-entries:2026-10-21", Body: "Nothing logged today"}}})

	n.mustTick()

	if got := received(telegram); !slices.Equal(got, []string{"Nothing logged today"}) {
		t.Errorf("received %q, want the body alone, with no blank first line", got)
	}
}

func TestWithNoBotAndAFailingProviderTheTickSaysBoth(t *testing.T) {
	n := newNando(t)
	n.core.AddNotificationProvider(brokenProvider{})
	n.core.AddNotificationProvider(&provider{due: []model.Notification{eveningSummary}})
	logs := captureLog(t)

	if rec := n.tick(schedulerToken); rec.Code != http.StatusInternalServerError {
		t.Errorf("tick: status = %d, want 500 for the broken provider", rec.Code)
	}

	if want := "tick: 1 Notification due, but no Assistant has a bot to send them, 1 error\n"; !strings.Contains(logs.String(), want) {
		t.Errorf("log = %q; want a line %q", logs, want)
	}
}

func TestWeighInReminderIsSentAtItsTimeEachWeek(t *testing.T) {
	tokyo, _ := time.LoadLocation("Asia/Tokyo")
	mondayMorning := time.Date(2026, 10, 26, 8, 0, 0, 0, tokyo) // Sunday 23:00 UTC
	n, phone, telegram := signedInWithBots(t, mondayMorning.Add(-30*time.Minute))
	phone.saveTimeZone("Asia/Tokyo")
	phone.saveWeighInReminder(reminderJSON{Enabled: true, Weekday: "monday", Time: "08:00"})

	n.mustTick() // 07:30
	if got := received(telegram); len(got) != 0 {
		t.Fatalf("at 07:30 received %q, want nothing yet", got)
	}

	n.clock.Set(mondayMorning)
	n.mustTick()
	n.clock.Advance(2 * time.Minute) // a retried tick
	n.mustTick()
	n.clock.Advance(28 * time.Minute) // 08:30
	n.mustTick()
	if got := received(telegram); !slices.Equal(got, []string{weighInReminder}) {
		t.Fatalf("on Monday by 08:30 received %q, want one %q", got, weighInReminder)
	}
	if sent := telegram.CallsTo("sendMessage"); sent[0].Token != coreBotToken {
		t.Errorf("the Weigh-in reminder was sent by %s, want the Core bot", sent[0].Token)
	}

	n.clock.Set(mondayMorning.AddDate(0, 0, 7))
	n.mustTick()
	if got := len(received(telegram)); got != 2 {
		t.Errorf("by the next Monday's 08:00 received %d reminders, want 2", got)
	}
}

func TestDisabledWeighInReminderIsNotSent(t *testing.T) {
	monday8am := time.Date(2026, 10, 26, 8, 0, 0, 0, time.UTC)
	n, phone, telegram := signedInWithBots(t, monday8am)
	phone.saveWeighInReminder(reminderJSON{Enabled: false, Weekday: "monday", Time: "08:00"})

	n.mustTick()

	if got := received(telegram); len(got) != 0 {
		t.Errorf("received %q, want nothing", got)
	}
}

func TestWeighInReminderIsSentInATimeZoneOffsetByAQuarterHour(t *testing.T) {
	kathmandu, _ := time.LoadLocation("Asia/Kathmandu") // UTC+5:45
	mondayMorning := time.Date(2026, 10, 26, 8, 0, 0, 0, kathmandu)
	n, phone, telegram := signedInWithBots(t, mondayMorning)
	phone.saveTimeZone("Asia/Kathmandu")
	phone.saveWeighInReminder(reminderJSON{Enabled: true, Weekday: "monday", Time: "08:00"})

	n.mustTick()

	if got := received(telegram); !slices.Equal(got, []string{weighInReminder}) {
		t.Errorf("at 08:00 in Kathmandu received %q, want %q", got, weighInReminder)
	}
}

func TestTicksEveryFiveMinutesSendTheWeighInReminderOnceAtItsHalfHour(t *testing.T) {
	tokyo, _ := time.LoadLocation("Asia/Tokyo")
	mondayMorning := time.Date(2026, 10, 26, 8, 0, 0, 0, tokyo)
	for _, c := range []struct {
		name  string
		first time.Duration // after 08:00, the first tick that runs
	}{
		{name: "the 08:00 tick ran", first: 0},
		{name: "the 08:00 tick never ran", first: 5 * time.Minute},
	} {
		t.Run(c.name, func(t *testing.T) {
			n, phone, telegram := signedInWithBots(t, mondayMorning.Add(-30*time.Minute))
			phone.saveTimeZone("Asia/Tokyo")
			phone.saveWeighInReminder(reminderJSON{Enabled: true, Weekday: "monday", Time: "08:00"})

			for at := c.first; at < 30*time.Minute; at += 5 * time.Minute {
				n.clock.Set(mondayMorning.Add(at))
				n.mustTick()
			}

			if got := received(telegram); !slices.Equal(got, []string{weighInReminder}) {
				t.Errorf("by 08:25 received %q, want the Weigh-in reminder once", got)
			}
		})
	}
}

// brokenProvider is a Module's provider that fails, such as when its database
// is briefly unreachable.
type brokenProvider struct{}

func (brokenProvider) DueNotifications(context.Context, time.Time) ([]model.Notification, error) {
	return nil, errors.New("database unreachable")
}

func TestABrokenProviderDoesNotStopTheOthers(t *testing.T) {
	n, _, telegram := signedInWithBots(t, wednesday)
	n.core.AddNotificationProvider(brokenProvider{})
	n.core.AddNotificationProvider(&provider{due: []model.Notification{eveningSummary}})

	rec := n.tick(schedulerToken)

	if got := received(telegram); !slices.Equal(got, []string{"Calories\nNothing logged today"}) {
		t.Errorf("received %q, want the working provider's Notification", got)
	}
	// Failing the tick has Cloud Scheduler retry it, which is safe.
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("tick with a broken provider: status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestANotificationCoreCannotRecordDoesNotStopTheOthers(t *testing.T) {
	n, _, telegram := signedInWithBots(t, wednesday)
	// An ID too big for a MongoDB document (16 MB), so recording it as sent fails.
	unrecordable := model.Notification{ID: strings.Repeat("x", 17<<20), Title: "Calories"}
	n.core.AddNotificationProvider(&provider{due: []model.Notification{unrecordable}})
	n.core.AddNotificationProvider(&provider{due: []model.Notification{eveningSummary}})

	rec := n.tick(schedulerToken)

	if got := received(telegram); !slices.Equal(got, []string{"Calories\nNothing logged today"}) {
		t.Errorf("received %q, want only the evening summary", got)
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
	n, _, telegram := signedInWithBots(t, wednesday)
	failing, next := &tickJob{err: errors.New("the database is unreachable")}, &tickJob{}
	n.core.AddTickJob(failing)
	n.core.AddTickJob(next)
	n.core.AddNotificationProvider(&provider{due: []model.Notification{eveningSummary}})

	rec := n.tick(schedulerToken)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("tick with a failing job: status = %d, want %d so that the scheduler retries it", rec.Code, http.StatusInternalServerError)
	}
	if failing.runs != 1 || next.runs != 1 || len(received(telegram)) != 1 {
		t.Errorf("the jobs ran %d and %d times and %d Notifications were sent; want each job once and the Notification sent",
			failing.runs, next.runs, len(received(telegram)))
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

func TestATickLogsWhichNotificationsItSent(t *testing.T) {
	n, _, _ := signedInWithBots(t, wednesday)
	n.core.AddNotificationProvider(&provider{due: []model.Notification{noEntriesReminder, eveningSummary}})
	logs := captureLog(t)

	n.mustTick()

	want := "tick: sent 2 Notifications in Telegram: calories:no-entries:2026-10-20 (sent), calories:evening-summary:2026-10-20 (sent)\n"
	if !strings.Contains(logs.String(), want) {
		t.Errorf("log = %q; want a line %q", logs, want)
	}
}

func TestATickWithNothingDueSaysSo(t *testing.T) {
	n, _, _ := signedInWithBots(t, wednesday)
	n.core.AddNotificationProvider(&provider{due: []model.Notification{eveningSummary}})
	n.mustTick()
	logs := captureLog(t)

	n.clock.Advance(time.Minute) // a retried tick: the evening summary was sent already
	n.mustTick()

	if got, want := logs.String(), "tick: no Notifications due\n"; !strings.HasSuffix(got, want) || strings.Contains(got, "tick: sent") {
		t.Errorf("log = %q; want only %q", got, want)
	}
}

func TestATickNeverLogsWhatANotificationSays(t *testing.T) {
	for _, refused := range []bool{false, true} {
		n, _, telegram := signedInWithBots(t, wednesday)
		if refused {
			telegram.Fail("sendMessage")
		}
		private := model.Notification{ID: "japanese:word-request-failed:abc", Title: "Couldn't add a word", Body: "ぬぺぽぴゃ: not a word Jisho knows"}
		n.core.AddNotificationProvider(&provider{due: []model.Notification{private}})
		logs := captureLog(t)

		n.mustTick()

		for _, text := range []string{private.Title, private.Body, "ぬぺぽぴゃ"} {
			if got := logs.String(); strings.Contains(got, text) {
				t.Errorf("refused %v: log = %q; gives away %q", refused, got, text)
			}
		}
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
			want: "tick: sent 1 Notification in Telegram, 1 error: calories:evening-summary:2026-10-20 (sent)\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			n, _, _ := signedInWithBots(t, wednesday)
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
