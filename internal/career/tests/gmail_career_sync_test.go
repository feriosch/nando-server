package tests

import (
	"net/http"
	"testing"
	"time"
)

// gmailCareerSyncJSON is the Gmail Career Sync in Career's settings (ADR 0010).
type gmailCareerSyncJSON struct {
	Enabled bool   `json:"enabled"`
	Time    string `json:"time"`
}

// careerSettingsJSON is Career's settings with the Gmail Career Sync.
type careerSettingsJSON struct {
	Looking         bool                 `json:"looking"`
	GmailCareerSync *gmailCareerSyncJSON `json:"gmailCareerSync"`
}

func TestTheGmailCareerSyncIsOffUntilTurnedOnAndKeptBySavesWithoutIt(t *testing.T) {
	_, phone := signedInBrowser(t)

	got := get[careerSettingsJSON](phone, "/api/career/settings")
	if got.GmailCareerSync == nil || *got.GmailCareerSync != (gmailCareerSyncJSON{Enabled: false, Time: "08:00"}) {
		t.Fatalf("settings = %+v; want the Gmail Career Sync off at 08:00", got)
	}

	on := gmailCareerSyncJSON{Enabled: true, Time: "07:30"}
	phone.must(http.MethodPut, "/api/career/settings", map[string]any{"looking": true, "gmailCareerSync": on}, http.StatusNoContent)
	// A web app that doesn't know it yet sends Looking alone.
	phone.must(http.MethodPut, "/api/career/settings", map[string]any{"looking": false}, http.StatusNoContent)

	got = get[careerSettingsJSON](phone, "/api/career/settings")
	if got.Looking || got.GmailCareerSync == nil || *got.GmailCareerSync != on {
		t.Errorf("settings = %+v; want Not Looking and the Gmail Career Sync kept as %+v", got, on)
	}
}

func TestAGmailCareerSyncTimeOffTheHalfHourIsRefused(t *testing.T) {
	_, phone := signedInBrowser(t)

	for _, at := range []string{"07:15", "24:00", ""} {
		phone.must(http.MethodPut, "/api/career/settings",
			map[string]any{"looking": true, "gmailCareerSync": gmailCareerSyncJSON{Enabled: true, Time: at}}, http.StatusBadRequest)
	}
}

func TestTheCareerBotAsksForAGmailCareerSyncOnceAtItsTime(t *testing.T) {
	n, phone := signedInBrowser(t)
	phone.saveTimeZone("Asia/Tokyo")
	phone.must(http.MethodPut, "/api/career/settings",
		map[string]any{"looking": true, "gmailCareerSync": gmailCareerSyncJSON{Enabled: true, Time: "07:30"}}, http.StatusNoContent)

	n.tickAt(at(1, 7, 0))
	n.tickAt(at(1, 7, 30))
	n.tickAt(at(1, 7, 35)) // a retried tick

	var asks []string
	for _, c := range n.telegram.CallsTo("sendMessage") {
		if text, _ := c.Params["text"].(string); c.Token == "222:career-bot" && text == "📨 Gmail Career Sync\nFiling your Career mail…" {
			asks = append(asks, text)
		}
	}
	if len(asks) != 1 {
		t.Errorf("the Career bot asked %d times, want once at 07:30; Telegram got %+v", len(asks), n.telegram.CallsTo("sendMessage"))
	}
}

// passJSON is what career_start_gmail_career_sync answers.
type passJSON struct {
	PassID string    `json:"passId"`
	Since  time.Time `json:"since"`
}

func TestAGmailCareerSyncReadsSinceTheLastFinishedPass(t *testing.T) {
	n, phone := signedInBrowser(t)
	syncTanaka(t, phone)
	statusBefore := callTool[syncStatusJSON](t, phone.macAt(at(3, 8, 0)), "career_sync_status", nil)
	backlog := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	// Each day on a new Connector session: the clock moves past the last
	// one's access token.
	n.clock.Set(at(1, 8, 0))
	run := phone.connect()
	first := callTool[passJSON](t, run, "career_start_gmail_career_sync", nil)
	if !first.Since.Equal(backlog) {
		t.Fatalf("first pass since %s, want the backlog from 2025-01-01", first.Since)
	}

	// The first pass never finished: the next reads from the backlog again.
	n.clock.Set(at(2, 8, 0))
	run = phone.connect()
	second := callTool[passJSON](t, run, "career_start_gmail_career_sync", nil)
	if !second.Since.Equal(backlog) || second.PassID == first.PassID {
		t.Fatalf("second pass = %+v; want a new pass, still from 2025-01-01", second)
	}
	callTool[map[string]any](t, run, "career_finish_gmail_career_sync", map[string]any{"passId": second.PassID})

	n.clock.Set(at(3, 8, 0))
	run = phone.connect()
	third := callTool[passJSON](t, run, "career_start_gmail_career_sync", nil)
	if !third.Since.Equal(at(2, 8, 0)) {
		t.Errorf("third pass since %s, want %s, when the last finished pass started", third.Since, at(2, 8, 0))
	}

	if after := callTool[syncStatusJSON](t, phone.macAt(at(3, 8, 0)), "career_sync_status", nil); after.Due != statusBefore.Due || after.Since != statusBefore.Since {
		t.Errorf("career_sync_status after Gmail passes = %+v, want it as before: %+v", after, statusBefore)
	}
}

func TestStartingAPassAnswersTheGmailThreadsNandoHolds(t *testing.T) {
	_, phone := signedInBrowser(t)
	syncTanaka(t, phone) // a LinkedIn thread, which isn't listed
	run := phone.connect()
	filed := addGmail(t, run, hays(mail("g1", "other", at(-3, 10, 15), "Hello!")))
	newsletter := hays(mail("n1", "other", at(-2, 10, 15), "Our newsletter."))
	newsletter["threadId"] = "18c2f0cccccccccc"
	ignored := addGmail(t, run, newsletter)
	callTool[conversationJSON](t, run, "career_ignore_thread", map[string]any{"conversationId": ignored.Conversation.ID})

	got := callTool[struct {
		Held []struct {
			ThreadID string    `json:"threadId"`
			Newest   time.Time `json:"newest"`
			Ignored  bool      `json:"ignored"`
		} `json:"held"`
	}](t, run, "career_start_gmail_career_sync", nil)

	byID := map[string]bool{}
	for _, h := range got.Held {
		byID[h.ThreadID] = h.Ignored
	}
	if len(got.Held) != 2 || byID[filed.Conversation.ThreadID] || !byID["18c2f0cccccccccc"] {
		t.Errorf("held = %+v; want the two Gmail threads, the newsletter marked ignored, and no LinkedIn one", got.Held)
	}
}
