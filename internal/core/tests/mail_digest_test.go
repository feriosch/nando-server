package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/feriosch/nando-server/internal/core"
	"github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/testtelegram"
	"github.com/feriosch/nando-server/internal/testtrigger"
)

// mailDigestJSON is the Mail Digest in the settings (ADR 0009).
type mailDigestJSON struct {
	Enabled bool   `json:"enabled"`
	Time    string `json:"time"`
}

// mailDigest is the saved Mail Digest, as GET /api/settings answers it.
func (d *browser) mailDigest() mailDigestJSON {
	d.nando.t.Helper()
	rec := d.do(http.MethodGet, "/api/settings", nil)
	var s struct {
		MailDigest *mailDigestJSON `json:"mailDigest"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &s) != nil || s.MailDigest == nil {
		d.nando.t.Fatalf("GET /api/settings: status = %d, body %q; want settings with mailDigest", rec.Code, rec.Body)
	}
	return *s.MailDigest
}

// putMailDigest sends the settings with digest, keeping the others, and
// returns the answer.
func (d *browser) putMailDigest(digest any) *httptest.ResponseRecorder {
	d.nando.t.Helper()
	s := d.getSettings()
	body := map[string]any{"timeZone": s.TimeZone, "heightCm": s.HeightCm, "weighInReminder": s.WeighInReminder, "mailDigest": digest}
	return d.do(http.MethodPut, "/api/settings", mustJSON(d.nando.t, body))
}

// saveMailDigest sets the Mail Digest, keeping the other settings.
func (d *browser) saveMailDigest(digest mailDigestJSON) {
	d.nando.t.Helper()
	if rec := d.putMailDigest(digest); rec.Code != http.StatusNoContent {
		d.nando.t.Fatalf("PUT /api/settings with Mail Digest %+v: status = %d, body %q", digest, rec.Code, rec.Body)
	}
}

func TestTheMailDigestIsOffUntilTheOwnerTurnsItOn(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)

	if got, want := phone.mailDigest(), (mailDigestJSON{Enabled: false, Time: "08:00"}); got != want {
		t.Errorf("Mail Digest = %+v, want %+v", got, want)
	}
}

func TestTheMailDigestIsSavedAndKeptBySettingsSavedWithoutIt(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	on := mailDigestJSON{Enabled: true, Time: "07:30"}
	phone.saveMailDigest(on)
	if got := phone.mailDigest(); got != on {
		t.Fatalf("Mail Digest after saving = %+v, want %+v", got, on)
	}

	// A web app that doesn't know the Mail Digest yet sends the rest only.
	phone.saveTimeZone("Asia/Tokyo")

	if got := phone.mailDigest(); got != on {
		t.Errorf("Mail Digest after saving the time zone = %+v, want it kept as %+v", got, on)
	}
}

func TestSettingsSavedBeforeTheMailDigestHaveItOffAtItsDefaultTime(t *testing.T) {
	n, phone := signedInBrowser(t, wednesday)
	// As saved before the Mail Digest existed: no mailDigest field at all.
	if _, err := n.db.Collection("settings").InsertOne(context.Background(), bson.D{
		{Key: "_id", Value: "settings"},
		{Key: "timeZone", Value: "UTC"},
		{Key: "heightCm", Value: 175},
		{Key: "weighInReminder", Value: bson.D{{Key: "enabled", Value: false}, {Key: "weekday", Value: 1}, {Key: "time", Value: "08:00"}}},
	}); err != nil {
		t.Fatal(err)
	}

	want := mailDigestJSON{Enabled: false, Time: "08:00"}
	if got := phone.mailDigest(); got != want {
		t.Fatalf("Mail Digest = %+v, want %+v", got, want)
	}
	// A web app that sends back the Mail Digest it was given can still save.
	if rec := phone.putMailDigest(phone.mailDigest()); rec.Code != http.StatusNoContent {
		t.Errorf("PUT the settings as given: status = %d, body %q; want 204", rec.Code, rec.Body)
	}
}

func TestAMailDigestTimeOffTheHalfHourIsRefused(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)

	for _, at := range []string{"07:15", "24:00", "8:00", ""} {
		if rec := phone.putMailDigest(mailDigestJSON{Enabled: true, Time: at}); rec.Code != http.StatusBadRequest {
			t.Errorf("PUT a Mail Digest at %q: status = %d, want 400", at, rec.Code)
		}
	}
}

// mailDigestAsk is the Mail Digest's ask as the Core bot sends it.
const mailDigestAsk = "📬 Mail Digest\nReading your new mail…"

// digestNando is a Nando with the Messenger on, the Owner signed in, and the
// Mail Digest on at 08:00 in Tokyo, at 07:30 that day, which it returns.
func digestNando(t *testing.T) (*nando, *testtelegram.Telegram, *testtrigger.Trigger, time.Time) {
	t.Helper()
	n, telegram, trigger := messengerNandoWithTrigger(t)
	tokyo, _ := time.LoadLocation("Asia/Tokyo")
	morning := time.Date(2026, 11, 20, 8, 0, 0, 0, tokyo)
	n.clock.Set(morning.Add(-30 * time.Minute))
	phone := n.newBrowser("iPhone")
	phone.mustEnroll()
	phone.saveTimeZone("Asia/Tokyo")
	phone.saveMailDigest(mailDigestJSON{Enabled: true, Time: "08:00"})
	return n, telegram, trigger, morning
}

func TestTheMailDigestIsAskedOfTheCoreAssistantOnceAtItsTimeAndWakesARun(t *testing.T) {
	n, telegram, trigger, morning := digestNando(t)

	n.mustTick() // 07:30
	if sent := telegram.CallsTo("sendMessage"); len(sent) != 0 {
		t.Fatalf("at 07:30 sent %+v; want nothing yet", sent)
	}

	n.clock.Set(morning)
	n.mustTick()
	n.clock.Advance(2 * time.Minute) // a retried tick
	n.mustTick()

	sent := telegram.CallsTo("sendMessage")
	if len(sent) != 1 || sent[0].Token != coreBotToken || sent[0].Params["text"] != mailDigestAsk {
		t.Fatalf("sendMessage calls = %+v; want the Core bot asking for the Mail Digest once", sent)
	}
	if fires := trigger.Received(); len(fires) != 1 {
		t.Errorf("fires = %d, want 1: the ask wakes a run", len(fires))
	}
}

func TestARunClaimsTheMailDigestFromNandoAndOneReplyAnswersItWithTheOwnersMessages(t *testing.T) {
	n, telegram, _, morning := digestNando(t)
	n.clock.Set(morning)
	n.mustTick()
	// Telegram numbers the bot's and the Owner's messages in one sequence.
	n.mustPostUpdate("core", ownerSays(5001, 100, "and anything from the landlord?"))

	r := n.startRun()
	got := r.claim("")
	if got.Assistant != "core" || len(got.Messages) != 2 ||
		got.Messages[0].From != "nando" || !got.Messages[0].Waiting || got.Messages[0].Text != mailDigestAsk ||
		got.Messages[1].From != "owner" || !got.Messages[1].Waiting {
		t.Fatalf("claim = %+v; want Nando's ask, then the Owner's message, both waiting", got)
	}
	r.reply("core", "3 new since yesterday, and nothing from the landlord.")

	if again := r.claim("core"); len(again.Messages) != 0 {
		t.Errorf("claim after the reply = %+v; want nothing waiting", again)
	}
	for _, c := range telegram.CallsTo("setMessageReaction") {
		if c.Params["message_id"] != float64(100) {
			t.Errorf("setMessageReaction on message %v; want reactions only on the Owner's message", c.Params["message_id"])
		}
	}
}

func TestAReplyAnswersTheMailDigestAlone(t *testing.T) {
	n, _, _, morning := digestNando(t)
	n.clock.Set(morning)
	n.mustTick()

	r := n.startRun()
	if got := r.claim(""); len(got.Messages) != 1 || got.Messages[0].From != "nando" {
		t.Fatalf("claim = %+v; want Nando's ask", got)
	}
	r.reply("core", "No new mail since yesterday.")

	if again := r.claim("core"); len(again.Messages) != 0 {
		t.Errorf("claim after the reply = %+v; want nothing waiting", again)
	}
}

func TestAMailDigestNoRunAnsweredFailsWithoutAskingTheOwnerToResend(t *testing.T) {
	n, telegram, _, morning := digestNando(t)
	n.clock.Set(morning)
	n.mustTick()

	r := n.startRun()
	for range 3 { // every claim lapses with no reply
		if got := r.claim("core"); len(got.Messages) == 0 {
			t.Fatal("claim found nothing; want the Mail Digest's ask")
		}
		n.clock.Advance(16 * time.Minute)
	}
	n.mustTick()

	sent := telegram.CallsTo("sendMessage")
	if len(sent) == 0 || sent[len(sent)-1].Params["text"] != "Sorry, something went wrong and I couldn't do that. Ask me if you still want it." {
		t.Errorf("sendMessage calls = %+v; want the last to be the line for a failed ask", sent)
	}
	if reactions := telegram.CallsTo("setMessageReaction"); len(reactions) != 0 {
		t.Errorf("setMessageReaction calls = %+v; want none for Nando's own message", reactions)
	}
}

func TestTheMailDigestIsNeverAskedOfAnotherAssistantsBot(t *testing.T) {
	telegram := testtelegram.New(t)
	n := newNandoWith(t, func(cfg *core.Config) {
		cfg.Messenger = core.MessengerConfig{
			TelegramURL: telegram.URL, OwnerID: ownerTelegramID, WebhookSecret: webhookSecret,
			BotTokens: map[string]string{"career": careerBotToken}, // no Core bot yet
		}
	})
	n.core.AddAssistant(model.Assistant{Name: "career"})
	n.clock.Set(time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC))
	phone := n.newBrowser("iPhone")
	phone.mustEnroll()
	phone.saveMailDigest(mailDigestJSON{Enabled: true, Time: "08:00"})

	n.mustTick()

	if sent := telegram.CallsTo("sendMessage"); len(sent) != 0 {
		t.Errorf("sendMessage calls = %+v; want nothing: only the Core Assistant can answer the Mail Digest", sent)
	}
	if talk := n.talk("career"); len(talk.Messages) != 0 {
		t.Errorf("Career Talk = %+v; want the ask kept out of it", talk.Messages)
	}
}

func TestAReplyNeverAnswersAMailDigestStoredAfterItsClaim(t *testing.T) {
	n, _, _, morning := digestNando(t)
	n.clock.Set(morning.Add(-time.Minute))
	n.mustPostUpdate("core", ownerSays(5001, 100, "what's on today?"))
	r := n.startRun()
	if got := r.claim(""); len(got.Messages) != 1 {
		t.Fatalf("claim = %+v; want the Owner's message alone", got.Messages)
	}

	// The ask is stored after the claim, though Telegram numbered it before the
	// Owner's message: the race between sending it and storing it.
	n.clock.Set(morning)
	n.mustTick()
	r.reply("core", "Nothing on the calendar today.")

	got := r.claim("core")
	waiting := false
	for _, m := range got.Messages {
		if m.From == "nando" {
			waiting = m.Waiting
		}
	}
	if !waiting {
		t.Errorf("claim after the reply = %+v; want the ask still waiting, since no run saw it", got.Messages)
	}
}
