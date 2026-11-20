package tests

import (
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// received fails the test unless the Owner has received want in Telegram, in
// order.
func received(t *testing.T, n *nando, want []notification, when string) {
	t.Helper()
	if got := n.received(); !slices.Equal(got, want) {
		t.Fatalf("%s: received %+v; want %+v", when, got, want)
	}
}

// waiting is the Notification that Recruiters wait for the Owner's reply.
func waiting(n string) notification {
	body := n + " Recruiters are waiting for your reply"
	if n == "1" {
		body = "1 Recruiter is waiting for your reply"
	}
	return notification{Title: "Recruiters waiting", Body: body}
}

// answerTanaka has the run write the answer to Tanaka's pitch.
func answerTanaka(t *testing.T, run *mcp.ClientSession, conversationID string) {
	t.Helper()
	callTool[conversationJSON](t, run, "career_set_suggested_reply", map[string]any{
		"conversationId": conversationID, "text": "Thank you for reaching out! Could you share the job description?",
		"language": "en", "kind": "answer", "answers": pitched.Format(time.RFC3339),
	})
}

// emptySyncAt has the Mac record a Sync at now that found nothing new.
func (d *browser) emptySyncAt(now time.Time) {
	d.nando.t.Helper()
	callTool[recordedJSON](d.nando.t, d.macAt(now), "career_record_sync", map[string]any{"threads": []any{}})
}

func TestNotLookingTheOwnerHearsOfWaitingRecruitersOnMondayAndThursdayAt19(t *testing.T) {
	n, phone := signedInBrowser(t)
	run, filed := fileTanakasConversation(t, phone)
	answerTanaka(t, run, filed.Conversation.ID)

	n.tickAt(at(0, 18, 55))
	received(t, n, nil, "Monday at 18:55")
	n.tickAt(at(0, 19, 0))
	received(t, n, []notification{waiting("1")}, "Monday at 19:00")
	n.tickAt(at(0, 19, 25))
	received(t, n, []notification{waiting("1")}, "Monday at 19:25, a later tick in the same slot")

	n.tickAt(at(1, 19, 0))
	received(t, n, []notification{waiting("1")}, "Tuesday at 19:00")

	phone.emptySyncAt(at(3, 8, 0))
	n.tickAt(at(3, 19, 0))
	received(t, n, []notification{waiting("1"), waiting("1")}, "Thursday at 19:00")
}

// setLooking saves whether the Owner is Looking, as the web app's Settings do.
func (d *browser) setLooking(looking bool) {
	d.nando.t.Helper()
	d.must(http.MethodPut, "/api/career/settings", map[string]any{"looking": looking}, http.StatusNoContent)
}

func TestLookingTheOwnerHearsOfWaitingRecruitersOnceASyncsWorkIsDone(t *testing.T) {
	n, phone := signedInBrowser(t)
	phone.setLooking(true)
	run, filed := fileTanakasConversation(t, phone)
	n.tickAt(at(0, 8, 5))
	received(t, n, nil, "after the first Sync's thread was sorted, with no answer yet")

	answerTanaka(t, run, filed.Conversation.ID)
	n.tickAt(at(0, 8, 10))
	n.tickAt(at(0, 8, 15))
	received(t, n, []notification{waiting("1")}, "once the first Sync's work was done, with an answer")
	n.tickAt(at(0, 19, 0))
	received(t, n, []notification{waiting("1")}, "Monday at 19:00, Looking")

	// The second Sync brings Tanaka's answer and a new Recruiter.
	asked := at(1, 7, 30)
	sato := recruiterThread(1, at(1, 7, 45))
	callTool[recordedJSON](t, phone.macAt(at(1, 8, 0)), "career_record_sync", map[string]any{"threads": []any{
		tanakaThread(message("m2", "other", asked, "Great! Here is the job description.")), sato,
	}})
	run = phone.connect()
	items := callTool[workJSON](t, run, "career_list_work", nil).Items
	if len(items) != 2 {
		t.Fatalf("the run claimed %+v; want Tanaka's reply and the new thread's sort", items)
	}
	callTool[conversationJSON](t, run, "career_set_suggested_reply", map[string]any{
		"conversationId": filed.Conversation.ID, "text": "Thank you! I'll read it tonight.",
		"language": "en", "kind": "answer", "answers": asked.Format(time.RFC3339),
	})
	n.tickAt(at(1, 8, 5))
	received(t, n, []notification{waiting("1")}, "while the second Sync's new thread is still unsorted")

	satoID := items[1].Conversation.ID
	if items[0].Kind == "sort" {
		satoID = items[0].Conversation.ID
	}
	callTool[filedJSON](t, run, "career_file_conversation", map[string]any{
		"conversationId": satoID, "recruiter": map[string]any{"name": "Sato Ken", "kind": "in_house", "organisation": "Kumo Pay"},
	})
	callTool[conversationJSON](t, run, "career_set_suggested_reply", map[string]any{
		"conversationId": satoID, "text": "Hello, thank you! What is the role?",
		"language": "en", "kind": "answer", "answers": at(1, 7, 45).Format(time.RFC3339),
	})
	n.tickAt(at(1, 8, 10))
	received(t, n, []notification{waiting("1"), waiting("2")}, "once the second Sync's work was done")
}

// saveStep adds a Step to the Opportunity with id.
func saveStep(t *testing.T, run *mcp.ClientSession, id string, step map[string]any) {
	t.Helper()
	step["opportunityId"] = id
	callTool[fullOpportunityJSON](t, run, "career_save_step", step)
}

func TestTheOwnerHearsOfTomorrowsStepsAt20TheEveningBefore(t *testing.T) {
	n, phone := signedInBrowser(t)
	run, tanaka := fileTanaka(t, phone)
	o := newOpportunity(t, run, tanaka)
	saveStep(t, run, o.ID, map[string]any{"kind": "Casual interview", "date": "2026-11-17", "time": "14:00"})
	saveStep(t, run, o.ID, map[string]any{"kind": "Take-home assignment", "date": "2026-11-17"})
	saveStep(t, run, o.ID, map[string]any{"kind": "Final interview", "date": "2026-11-18", "time": "10:00"})

	n.tickAt(at(0, 19, 55))
	received(t, n, nil, "Monday at 19:55")
	n.tickAt(at(0, 20, 0))
	n.tickAt(at(0, 20, 5))
	want := []notification{
		{Title: "Tomorrow: Casual interview", Body: "Kumo Pay, 14:00"},
		{Title: "Tomorrow: Take-home assignment", Body: "Kumo Pay"},
	}
	slices.SortFunc(want, func(a, b notification) int { return strings.Compare(a.Title, b.Title) })
	got := n.received()
	slices.SortFunc(got, func(a, b notification) int { return strings.Compare(a.Title, b.Title) })
	if !slices.Equal(got, want) {
		t.Fatalf("Monday at 20:00 and 20:05: received %+v; want %+v", got, want)
	}
}

// sessionExpiredAt has the Mac tell Nando at now that LinkedIn wants the Owner
// to log in again.
func (d *browser) sessionExpiredAt(now time.Time) {
	d.nando.t.Helper()
	callTool[recordedJSON](d.nando.t, d.macAt(now), "career_record_sync", map[string]any{"threads": []any{}, "problem": map[string]any{"kind": "session_expired"}})
}

func TestTheOwnerHearsOnceThatLinkedInWantsThemToLogInAgain(t *testing.T) {
	n, phone := signedInBrowser(t)
	phone.saveTimeZone("Asia/Tokyo")
	phone.emptySyncAt(at(0, 8, 0))
	logIn := notification{Title: "Log in to LinkedIn again", Body: "Open the Mac and run the Sync's login"}

	phone.sessionExpiredAt(at(0, 14, 0))
	n.tickAt(at(0, 14, 5))
	received(t, n, []notification{logIn}, "after the Sync reported session_expired")

	// The Mac tries again every hour until the Owner logs in.
	phone.sessionExpiredAt(at(0, 15, 0))
	n.tickAt(at(0, 15, 5))
	received(t, n, []notification{logIn}, "after the next hour's session_expired")

	phone.emptySyncAt(at(0, 20, 0))
	phone.sessionExpiredAt(at(1, 8, 0))
	n.tickAt(at(1, 8, 5))
	received(t, n, []notification{logIn, logIn}, "after a Sync read LinkedIn, and the session expired again")
}

func TestTheOwnerHearsWhenNoSyncHasReadLinkedInFor36Hours(t *testing.T) {
	n, phone := signedInBrowser(t)
	phone.saveTimeZone("Asia/Tokyo")
	phone.emptySyncAt(at(0, 8, 0))
	noSync := func(day string) notification {
		return notification{Title: "No LinkedIn Sync since " + day, Body: "Check that the Mac is on and its Sync job is loaded"}
	}
	// A Sync that couldn't read LinkedIn doesn't count.
	callTool[recordedJSON](t, phone.macAt(at(1, 10, 0)), "career_record_sync", map[string]any{
		"threads": []any{}, "problem": map[string]any{"kind": "other", "text": "the reader stopped answering"},
	})

	n.tickAt(at(1, 19, 55))
	received(t, n, nil, "35 hours and 55 minutes after the last Sync")
	n.tickAt(at(1, 20, 0))
	n.tickAt(at(2, 7, 0))
	received(t, n, []notification{noSync("Monday")}, "36 hours after the last Sync, and later")

	phone.emptySyncAt(at(2, 8, 0))
	n.tickAt(at(3, 19, 55))
	received(t, n, []notification{noSync("Monday")}, "under 36 hours after Wednesday's Sync")
	n.tickAt(at(3, 20, 0))
	received(t, n, []notification{noSync("Monday"), noSync("Wednesday")}, "36 hours after Wednesday's Sync")
}

func TestALateReplyNeverNotifiesTheOwner(t *testing.T) {
	n, phone := signedInBrowser(t)
	run, filed := fileTanakasConversation(t, phone)
	answerTanaka(t, run, filed.Conversation.ID)
	// Monday 2026-12-14, over 30 days after Tanaka's pitch.
	phone.emptySyncAt(at(28, 8, 0))
	n.tickAt(at(28, 8, 5))
	if got := callTool[conversationJSON](t, phone.connect(), "career_get_conversation", map[string]any{"id": filed.Conversation.ID}); got.SuggestedReply.Kind != "late" {
		t.Fatalf("the Suggested Reply is %s; want late", got.SuggestedReply.Kind)
	}

	n.tickAt(at(28, 19, 0))
	received(t, n, nil, "Monday at 19:00, with only a Late Reply")
}

func TestAReplyTurningLateAtThe19SlotIsNotCountedAsWaiting(t *testing.T) {
	n, phone := signedInBrowser(t)
	_, filed := fileTanakasConversation(t, phone)
	// Thirty days and two minutes before Monday 2026-12-14 at 19:00.
	asked := at(-2, 18, 58)
	callTool[recordedJSON](t, phone.macAt(at(0, 9, 0)), "career_record_sync", map[string]any{"threads": []any{
		tanakaThread(message("m2", "other", asked, "Are you free for a call?")),
	}})
	callTool[conversationJSON](t, phone.connect(), "career_set_suggested_reply", map[string]any{
		"conversationId": filed.Conversation.ID, "text": "Yes, thank you!",
		"language": "en", "kind": "answer", "answers": asked.Format(time.RFC3339),
	})
	phone.emptySyncAt(at(28, 8, 0))

	// The tick sends Notifications before its rules turn the reply late.
	n.tickAt(at(28, 19, 0))
	received(t, n, nil, "Monday at 19:00, the first tick after the answer's message turned 30 days old")
}

func TestSyncProblemsAreToldOnlyWithin7Days(t *testing.T) {
	n, phone := signedInBrowser(t)
	phone.saveTimeZone("Asia/Tokyo")
	phone.emptySyncAt(at(0, 8, 0))
	phone.sessionExpiredAt(at(0, 9, 0))

	// Core forgets a sent ID after 30 days, so a problem told for longer
	// would be told again: the first tick in 9 days tells neither.
	n.tickAt(at(9, 9, 5))
	received(t, n, nil, "9 days after the last Sync and the expired session, at the first tick since")
}
