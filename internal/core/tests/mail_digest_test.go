package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/feriosch/nando-server/internal/testpush"
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

func TestSettingsSavedWithoutTheMailDigestKeepIt(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	on := mailDigestJSON{Enabled: true, Time: "07:30"}
	phone.saveMailDigest(on)

	// A web app that doesn't know the Mail Digest yet sends the rest only.
	phone.saveTimeZone("Asia/Tokyo")

	if got := phone.mailDigest(); got != on {
		t.Errorf("Mail Digest after saving the time zone = %+v, want it kept as %+v", got, on)
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

func TestTheMailDigestAsksTheCoreAssistantAtItsTimeAndWakesARun(t *testing.T) {
	tokyo, _ := time.LoadLocation("Asia/Tokyo")
	morning := time.Date(2026, 11, 20, 8, 0, 0, 0, tokyo)
	n, telegram, trigger := messengerNandoWithTrigger(t)
	n.clock.Set(morning.Add(-30 * time.Minute))
	phone := n.newBrowser("iPhone")
	phone.mustEnroll()
	phone.saveTimeZone("Asia/Tokyo")
	phone.saveMailDigest(mailDigestJSON{Enabled: true, Time: "08:00"})

	n.mustTick() // 07:30
	if sent := telegram.CallsTo("sendMessage"); len(sent) != 0 {
		t.Fatalf("at 07:30 sent %+v; want nothing yet", sent)
	}

	n.clock.Set(morning)
	n.mustTick()
	n.clock.Advance(2 * time.Minute) // a retried tick
	n.mustTick()

	sent := telegram.CallsTo("sendMessage")
	if len(sent) != 1 || sent[0].Token != coreBotToken || sent[0].Params["text"] != "📬 Mail Digest\nReading your new mail…" {
		t.Fatalf("sendMessage calls = %+v; want the Core bot asking for the Mail Digest once", sent)
	}
	if fires := trigger.Received(); len(fires) != 1 {
		t.Errorf("fires = %d, want 1: the ask wakes a run", len(fires))
	}

	r := n.startRun()
	got := r.claim("")
	if got.Assistant != "core" || len(got.Messages) != 1 || got.Messages[0].From != "nando" || !got.Messages[0].Waiting ||
		got.Messages[0].Text != "📬 Mail Digest\nReading your new mail…" {
		t.Fatalf("claim = %+v; want the Core Talk with Nando's ask, waiting", got)
	}
	r.reply("core", "3 new since yesterday: …")
	if again := r.claim("core"); len(again.Messages) != 0 {
		t.Errorf("claim after the reply = %+v; want nothing waiting", again)
	}
	if reactions := telegram.CallsTo("setMessageReaction"); len(reactions) != 0 {
		t.Errorf("setMessageReaction calls = %+v; want none: only the Owner's messages get a 👀", reactions)
	}
}

func TestTheMailDigestAndTheOwnersMessagesAreAnsweredTogether(t *testing.T) {
	n, _, _ := messengerNandoWithTrigger(t)
	n.clock.Set(time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC))
	phone := n.newBrowser("iPhone")
	phone.mustEnroll()
	phone.saveMailDigest(mailDigestJSON{Enabled: true, Time: "08:00"})

	n.mustTick()
	// Telegram numbers the bot's and the Owner's messages in one sequence.
	n.mustPostUpdate("core", ownerSays(5001, 100, "and anything from the landlord?"))

	r := n.startRun()
	got := r.claim("")
	if len(got.Messages) != 2 || !got.Messages[0].Waiting || !got.Messages[1].Waiting {
		t.Fatalf("claim = %+v; want the ask and the Owner's message, both waiting", got.Messages)
	}
	r.reply("core", "Here's your digest, and nothing from the landlord.")
	for _, m := range n.talk("core").Messages {
		if m.State == "waiting" {
			t.Errorf("message %q still waits after the reply", m.Text)
		}
	}
}

func TestTheMailDigestIsNeverSentByWebPush(t *testing.T) {
	at := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)
	n, phone := signedInBrowser(t, at)
	push := testpush.New(t)
	phone.subscribe(push)
	phone.saveMailDigest(mailDigestJSON{Enabled: true, Time: "08:00"})

	n.mustTick()

	if got := push.Received(); len(got) != 0 {
		t.Errorf("Web Push received %+v; want nothing: only the Core Assistant can answer the Mail Digest", got)
	}
}

func TestAMailDigestNoRunAnsweredFailsWithoutAskingTheOwnerToResend(t *testing.T) {
	n, telegram, _ := messengerNandoWithTrigger(t)
	n.clock.Set(time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC))
	phone := n.newBrowser("iPhone")
	phone.mustEnroll()
	phone.saveMailDigest(mailDigestJSON{Enabled: true, Time: "08:00"})
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
	if last := sent[len(sent)-1].Params["text"]; last != "Sorry, something went wrong and I couldn't do that. Ask me if you still want it." {
		t.Errorf("last message = %q; want the line for a failed ask", last)
	}
	if reactions := telegram.CallsTo("setMessageReaction"); len(reactions) != 0 {
		t.Errorf("setMessageReaction calls = %+v; want none for Nando's own message", reactions)
	}
}
