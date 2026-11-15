package tests

import (
	"testing"
	"time"

	"github.com/feriosch/nando-server/internal/testtrigger"
)

// day is 24 hours, for moving the clock by Days.
const day = 24 * time.Hour

// tickAt moves the clock to now and ticks, as Cloud Scheduler does.
func (n *nando) tickAt(now time.Time) {
	n.t.Helper()
	n.clock.Set(now)
	n.tick()
}

// opportunity reads the Opportunity with id through career_get_opportunity,
// on a fresh Connector session: the clock may have moved past the last one's
// access token.
func (d *browser) opportunity(id string) fullOpportunityJSON {
	d.nando.t.Helper()
	return callTool[fullOpportunityJSON](d.nando.t, d.connect(), "career_get_opportunity", map[string]any{"id": id})
}

// isFaded reports whether o was closed as faded.
func isFaded(o fullOpportunityJSON) bool {
	return o.Stage == "closed" && o.Outcome != nil && *o.Outcome == "faded"
}

func TestAnOpportunityFades30DaysAfterTheLastMessageInItsRecruitersConversations(t *testing.T) {
	n, phone := signedInBrowser(t)
	run, tanaka := fileTanaka(t, phone)
	o := newOpportunity(t, run, tanaka)
	// Tanaka writes again five days after the Opportunity was saved.
	lastMessage := at(5, 9, 0)
	callTool[recordedJSON](t, phone.macAt(at(5, 10, 0)), "career_record_sync", map[string]any{"threads": []any{
		tanakaThread(message("m2", "other", lastMessage, "The fintech is still reviewing your profile.")),
	}})

	n.tickAt(lastMessage.Add(30*day - time.Minute))
	if got := phone.opportunity(o.ID); got.Stage != "introduced" || got.Outcome != nil {
		t.Fatalf("a minute short of 30 days after Tanaka's last message, the Opportunity is %s (%v); want it still introduced", got.Stage, got.Outcome)
	}

	n.tickAt(lastMessage.Add(30 * day))
	if got := phone.opportunity(o.ID); !isFaded(got) {
		t.Errorf("30 days after Tanaka's last message, the Opportunity is %s (%v); want closed as faded", got.Stage, got.Outcome)
	}
}

func TestAnOpportunityWithNoRecruiterFades30DaysAfterItLastChanged(t *testing.T) {
	n, phone := signedInBrowser(t)
	phone.saveTimeZone("Asia/Tokyo")
	// The Owner found this one themselves.
	n.clock.Set(at(0, 8, 0))
	o := callTool[fullOpportunityJSON](t, phone.connect(), "career_save_opportunity", map[string]any{
		"company": map[string]any{"name": "Kumo Pay"}, "role": "Backend Engineer",
	})
	changed := at(10, 21, 0)
	n.clock.Set(changed)
	callTool[fullOpportunityJSON](t, phone.connect(), "career_save_opportunity", map[string]any{"id": o.ID, "interest": "high"})

	n.tickAt(changed.Add(30*day - time.Minute))
	if got := phone.opportunity(o.ID); got.Stage != "introduced" || got.Outcome != nil {
		t.Fatalf("a minute short of 30 days after its last change, the Opportunity is %s (%v); want it still introduced", got.Stage, got.Outcome)
	}

	n.tickAt(changed.Add(30 * day))
	if got := phone.opportunity(o.ID); !isFaded(got) || got.Interest != "high" {
		t.Errorf("30 days after its last change, the Opportunity is %s (%v), %s Interest; want closed as faded, still high", got.Stage, got.Outcome, got.Interest)
	}
}

func TestAnAnswerToAMessageOver30DaysOldTurnsLateAndIsQueuedToRewrite(t *testing.T) {
	n, phone := signedInBrowser(t)
	run, filed := fileTanakasConversation(t, phone)
	id := filed.Conversation.ID
	callTool[conversationJSON](t, run, "career_set_suggested_reply", map[string]any{
		"conversationId": id, "text": "Thank you for reaching out! Could you share the job description?",
		"language": "en", "kind": "answer", "answers": pitched.Format(time.RFC3339),
	})

	n.tickAt(pitched.Add(30 * day))
	if got := callTool[conversationJSON](t, phone.connect(), "career_get_conversation", map[string]any{"id": id}); got.SuggestedReply.Kind != "answer" {
		t.Fatalf("30 days after Tanaka's pitch, the Suggested Reply is %s; want it still an answer", got.SuggestedReply.Kind)
	}

	n.tickAt(pitched.Add(30*day + time.Minute))
	run = phone.connect()
	if got := callTool[conversationJSON](t, run, "career_get_conversation", map[string]any{"id": id}); got.SuggestedReply.Kind != "late" {
		t.Errorf("over 30 days after Tanaka's pitch, the Suggested Reply is %s; want late", got.SuggestedReply.Kind)
	}
	work := callTool[workJSON](t, run, "career_list_work", nil)
	if len(work.Items) != 1 || work.Items[0].Kind != "late_reply" || work.Items[0].Conversation.ID != id {
		t.Errorf("the run claimed %+v; want one late_reply item for Tanaka's Conversation", work.Items)
	}
}

// ownersMessage is when the Owner wrote to Tanaka last, in
// tanakaWaitsOnTheOwner.
var ownersMessage = at(1, 20, 0)

// tanakaWaitsOnTheOwner files Tanaka's Conversation, saves the fintech
// Opportunity in stage, and has a Sync bring the Owner's answer, sent at
// ownersMessage: the last message is the Owner's. It returns the
// Conversation's id.
func tanakaWaitsOnTheOwner(t *testing.T, phone *browser, stage string) string {
	t.Helper()
	run, filed := fileTanakasConversation(t, phone)
	o := newOpportunity(t, run, filed.Recruiter)
	if stage != "introduced" {
		callTool[fullOpportunityJSON](t, run, "career_save_opportunity", map[string]any{"id": o.ID, "stage": stage})
	}
	callTool[recordedJSON](t, phone.macAt(at(2, 8, 0)), "career_record_sync", map[string]any{"threads": []any{
		tanakaThread(message("m2", "owner", ownersMessage, "Thank you, I'd be glad to talk with them.")),
	}})
	return filed.Conversation.ID
}

// claimWorkAt has a new run claim the work at now, and answers the Items.
func (d *browser) claimWorkAt(now time.Time) []itemJSON {
	d.nando.t.Helper()
	d.nando.clock.Set(now)
	return callTool[workJSON](d.nando.t, d.connect(), "career_list_work", nil).Items
}

func TestAnOpportunityInProcessGetsOneFollowUp14DaysAfterTheOwnersMessage(t *testing.T) {
	trigger := testtrigger.New(t)
	n, phone := signedInBrowserWith(t, config(trigger))
	id := tanakaWaitsOnTheOwner(t, phone, "in_process")
	fired := len(trigger.Received())

	n.tickAt(ownersMessage.Add(14*day - time.Minute))
	firedTimes(t, trigger, fired, "nothing is queued a minute short of 14 days")

	due := ownersMessage.Add(14 * day)
	n.tickAt(due)
	firedTimes(t, trigger, fired+1, "the follow-up the tick queued wakes the run")
	items := phone.claimWorkAt(due.Add(time.Minute))
	if len(items) != 1 || items[0].Kind != "follow_up" || items[0].Conversation.ID != id {
		t.Fatalf("the run claimed %+v; want one follow_up item for Tanaka's Conversation", items)
	}

	// No run writes it: after 3 lapsed claims it is set aside.
	for claim := 2; claim <= 3; claim++ {
		if again := phone.claimWorkAt(due.Add(time.Duration(claim-1)*16*time.Minute + time.Minute)); len(again) != 1 {
			t.Fatalf("claim %d got %+v; want the follow_up item back", claim, again)
		}
	}
	n.tickAt(due.Add(2 * time.Hour))
	n.tickAt(due.Add(3 * time.Hour))
	if again := phone.claimWorkAt(due.Add(3*time.Hour + time.Minute)); len(again) != 0 {
		t.Errorf("after the follow-up was set aside, the run claimed %+v; want no second follow-up for the same message", again)
	}
}

func TestAnIntroducedOpportunityGetsNoFollowUp(t *testing.T) {
	n, phone := signedInBrowser(t)
	tanakaWaitsOnTheOwner(t, phone, "introduced")

	n.tickAt(ownersMessage.Add(15 * day))

	if items := phone.claimWorkAt(ownersMessage.Add(15*day + time.Minute)); len(items) != 0 {
		t.Errorf("the run claimed %+v; want nothing: only an Opportunity in process or at an offer is followed up", items)
	}
}
