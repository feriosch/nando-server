package tests

import (
	"net/http"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/feriosch/nando-server/internal/testpush"
)

// marked is when the Owner marks Tanaka's Suggested Reply replied in
// markedTanaka: Monday at 21:00.
var marked = at(0, 21, 0)

// markReplied marks the Conversation's Suggested Reply as replied, as the
// app does once the Owner sent it in LinkedIn.
func (d *browser) markReplied(conversationID string) {
	d.nando.t.Helper()
	d.must(http.MethodPost, "/api/career/conversations/"+conversationID+"/replied", nil, http.StatusNoContent)
}

// undoReplied clears the mark, as the app's Undo does.
func (d *browser) undoReplied(conversationID string) {
	d.nando.t.Helper()
	d.must(http.MethodDelete, "/api/career/conversations/"+conversationID+"/replied", nil, http.StatusNoContent)
}

// suggestedReply reads the Conversation's Suggested Reply through
// career_get_conversation, as the run and the chat skills do.
func (d *browser) suggestedReply(conversationID string) *suggestedReplyJSON {
	d.nando.t.Helper()
	return callTool[conversationJSON](d.nando.t, d.connect(), "career_get_conversation", map[string]any{"id": conversationID}).SuggestedReply
}

// answeredTanaka files Tanaka's Conversation and has the run answer the
// pitch. It answers what career_file_conversation did.
func answeredTanaka(t *testing.T, phone *browser) filedJSON {
	t.Helper()
	run, filed := fileTanakasConversation(t, phone)
	answerTanaka(t, run, filed.Conversation.ID)
	return filed
}

// markedTanaka is answeredTanaka, and the Owner marks the answer replied at
// marked.
func markedTanaka(t *testing.T, n *nando, phone *browser) filedJSON {
	t.Helper()
	filed := answeredTanaka(t, phone)
	n.clock.Set(marked)
	phone.markReplied(filed.Conversation.ID)
	return filed
}

// markedTanakaInProcess is markedTanaka, with Tanaka's fintech Opportunity
// in process, so the tick follows it up.
func markedTanakaInProcess(t *testing.T, n *nando, phone *browser) filedJSON {
	t.Helper()
	run, filed := fileTanakasConversation(t, phone)
	o := newOpportunity(t, run, filed.Recruiter)
	callTool[fullOpportunityJSON](t, run, "career_save_opportunity", map[string]any{"id": o.ID, "stage": "in_process"})
	answerTanaka(t, run, filed.Conversation.ID)
	n.clock.Set(marked)
	phone.markReplied(filed.Conversation.ID)
	return filed
}

func TestTheOwnerMarksASuggestedReplyAsRepliedAndItStaysOnTheRecruiterPage(t *testing.T) {
	n, phone := signedInBrowser(t)
	filed := markedTanaka(t, n, phone)

	page := get[recruiterPageJSON](phone, "/api/career/recruiters/"+filed.Recruiter.ID)
	if r := page.Conversations[0].SuggestedReply; r == nil || r.MarkedRepliedAt == nil || !r.MarkedRepliedAt.Equal(marked) {
		t.Fatalf("Tanaka's page shows the Suggested Reply as %+v; want it still there, marked replied at %v", r, marked)
	}
}

func TestMarkingAgainKeepsTheFirstTime(t *testing.T) {
	n, phone := signedInBrowser(t)
	filed := markedTanaka(t, n, phone)

	n.clock.Set(marked.Add(time.Hour))
	phone.markReplied(filed.Conversation.ID)

	page := get[recruiterPageJSON](phone, "/api/career/recruiters/"+filed.Recruiter.ID)
	if r := page.Conversations[0].SuggestedReply; r == nil || r.MarkedRepliedAt == nil || !r.MarkedRepliedAt.Equal(marked) {
		t.Errorf("after marking twice, the Suggested Reply is %+v; want it marked replied at the first time, %v", r, marked)
	}
}

func TestMarkingAnUnknownConversationOrOneWithNoSuggestedReplyIsRefused(t *testing.T) {
	_, phone := signedInBrowser(t)
	_, filed := fileTanakasConversation(t, phone)

	for _, id := range []string{bson.NewObjectID().Hex(), "not-an-id"} {
		phone.must(http.MethodPost, "/api/career/conversations/"+id+"/replied", nil, http.StatusNotFound)
	}
	phone.must(http.MethodPost, "/api/career/conversations/"+filed.Conversation.ID+"/replied", nil, http.StatusConflict)
}

func TestUndoClearsTheMark(t *testing.T) {
	n, phone := signedInBrowser(t)
	filed := markedTanaka(t, n, phone)

	phone.undoReplied(filed.Conversation.ID)

	page := get[recruiterPageJSON](phone, "/api/career/recruiters/"+filed.Recruiter.ID)
	if r := page.Conversations[0].SuggestedReply; r == nil || r.MarkedRepliedAt != nil {
		t.Fatalf("after Undo, the Suggested Reply is %+v; want it there, not marked", r)
	}
	// Nothing is marked now: Undo again still answers 204.
	phone.undoReplied(filed.Conversation.ID)
	for _, id := range []string{bson.NewObjectID().Hex(), "not-an-id"} {
		phone.must(http.MethodDelete, "/api/career/conversations/"+id+"/replied", nil, http.StatusNotFound)
	}
}

func TestAMarkedSuggestedReplyIsNotWaiting(t *testing.T) {
	n, phone := signedInBrowser(t)
	conversations, _ := fileThreads(t, phone,
		thread("tanaka", message("t1", "other", at(-3, 10, 15), "I have a backend role at a fintech.")),
		thread("sato", message("s1", "other", at(-2, 10, 0), "Hello!")),
	)
	suggestReply(t, phone, conversations["2-tanaka"], "answer", "Thank you!", at(-3, 10, 15))
	suggestReply(t, phone, conversations["2-sato"], "answer", "Hello Sato-san!", at(-2, 10, 0))
	n.clock.Set(marked)

	phone.markReplied(conversations["2-sato"])

	screen := get[careerScreenJSON](phone, "/api/career")
	if len(screen.Waiting) != 1 || screen.Waiting[0].Name != "tanaka" {
		t.Errorf("waiting = %+v; want only Tanaka: Sato's reply is marked replied", screen.Waiting)
	}
	if card := get[homeCardJSON](phone, "/api/career/home-card"); card.Waiting != 1 {
		t.Errorf("the Home Card shows %d Recruiters waiting; want 1, not Sato", card.Waiting)
	}
}

func TestAMarkedLateReplyIsNotInLateReplies(t *testing.T) {
	n, phone := signedInBrowser(t)
	conversations, _ := fileThreads(t, phone,
		thread("mori", message("m1", "other", at(-45, 10, 0), "Are you open to new roles?")),
		thread("ueda", message("u1", "other", at(-400, 10, 0), "A new role in Osaka!")),
	)
	suggestReply(t, phone, conversations["2-mori"], "late", "Hello Mori-san!", at(-45, 10, 0))
	suggestReply(t, phone, conversations["2-ueda"], "late", "Hello Ueda-san!", at(-400, 10, 0))
	n.clock.Set(marked)

	phone.markReplied(conversations["2-ueda"])

	if late := get[[]waitingJSON](phone, "/api/career/late-replies"); len(late) != 1 || late[0].Name != "mori" {
		t.Errorf("Late Replies list %+v; want only Mori: Ueda's is marked replied", late)
	}
	if screen := get[careerScreenJSON](phone, "/api/career"); screen.LateReplies != 1 {
		t.Errorf("lateReplies = %d; want 1, not Ueda", screen.LateReplies)
	}
}

func TestNotLookingTheOwnerDoesNotHearOfAMarkedSuggestedReply(t *testing.T) {
	n, phone, push := subscribedPhone(t)
	filed := answeredTanaka(t, phone)
	// Before the 19:00 slot.
	n.clock.Set(at(0, 12, 0))
	phone.markReplied(filed.Conversation.ID)

	n.tickAt(at(0, 19, 0))
	received(t, push, nil, "Monday at 19:00, with only a marked Suggested Reply")
}

func TestLookingTheOwnerDoesNotHearOfASyncWhoseSuggestedReplyIsMarked(t *testing.T) {
	n, phone, push := subscribedPhone(t)
	phone.setLooking(true)
	answeredTanaka(t, phone)
	n.tickAt(at(0, 8, 10))
	received(t, push, []testpush.Notification{waiting("1")}, "once the first Sync's work was done, with Tanaka waiting")

	// The second Sync brings Sato, whom the Owner answers in LinkedIn
	// before the tick.
	callTool[recordedJSON](t, phone.macAt(at(1, 8, 0)), "career_record_sync", map[string]any{"threads": []any{recruiterThread(1, at(1, 7, 45))}})
	run := phone.connect()
	sato := callTool[workJSON](t, run, "career_list_work", nil).Items[0].Conversation.ID
	callTool[filedJSON](t, run, "career_file_conversation", map[string]any{
		"conversationId": sato, "recruiter": map[string]any{"name": "Sato Ken", "kind": "in_house", "organisation": "Kumo Pay"},
	})
	callTool[conversationJSON](t, run, "career_set_suggested_reply", map[string]any{
		"conversationId": sato, "text": "Hello, thank you! What is the role?",
		"language": "en", "kind": "answer", "answers": at(1, 7, 45).Format(time.RFC3339),
	})
	phone.markReplied(sato)

	n.tickAt(at(1, 8, 5))
	received(t, push, []testpush.Notification{waiting("1")}, "after the second Sync, whose only Suggested Reply is marked replied")
}

func TestTheToolsShowThatTheOwnerMarkedASuggestedReplyReplied(t *testing.T) {
	n, phone := signedInBrowser(t)
	filed := answeredTanaka(t, phone)
	if r := phone.suggestedReply(filed.Conversation.ID); r == nil || r.MarkedRepliedAt != nil {
		t.Fatalf("before the mark, career_get_conversation shows the Suggested Reply as %+v; want it not marked", r)
	}
	n.clock.Set(marked)

	phone.markReplied(filed.Conversation.ID)

	if r := phone.suggestedReply(filed.Conversation.ID); r == nil || r.MarkedRepliedAt == nil || !r.MarkedRepliedAt.Equal(marked) {
		t.Errorf("career_get_conversation shows the Suggested Reply as %+v; want it marked replied at %v", r, marked)
	}
}

func TestASyncBringingTheOwnersReplyClearsAMarkedSuggestedReply(t *testing.T) {
	n, phone := signedInBrowser(t)
	id := markedTanaka(t, n, phone).Conversation.ID

	callTool[recordedJSON](t, phone.macAt(at(1, 8, 0)), "career_record_sync", map[string]any{"threads": []any{
		tanakaThread(message("m2", "owner", at(0, 20, 50), "Thank you for reaching out! Could you share the job description?")),
	}})

	if r := phone.suggestedReply(id); r != nil {
		t.Errorf("after the Sync brought the Owner's reply, the Suggested Reply is %+v; want it gone", r)
	}
	if items := phone.claimWorkAt(at(1, 8, 1)); len(items) != 0 {
		t.Errorf("the run claimed %+v; want nothing: Tanaka hasn't written again", items)
	}
}

func TestASyncBringingTheOwnersReplyAndTheRecruitersAnswerClearsTheMarkAndQueuesAReply(t *testing.T) {
	n, phone := signedInBrowser(t)
	id := markedTanaka(t, n, phone).Conversation.ID

	callTool[recordedJSON](t, phone.macAt(at(1, 8, 0)), "career_record_sync", map[string]any{"threads": []any{
		tanakaThread(
			message("m2", "owner", at(0, 20, 50), "Thank you for reaching out! Could you share the job description?"),
			message("m3", "other", at(1, 7, 30), "Here it is!"),
		),
	}})

	if r := phone.suggestedReply(id); r != nil {
		t.Errorf("after the Sync, the Suggested Reply is %+v; want it gone", r)
	}
	if items := phone.claimWorkAt(at(1, 8, 1)); len(items) != 1 || items[0].Kind != "reply" || items[0].Conversation.ID != id {
		t.Errorf("the run claimed %+v; want one reply Item for Tanaka's answer", items)
	}
}

func TestTheRunsNewSuggestedReplyIsNotMarked(t *testing.T) {
	n, phone := signedInBrowser(t)
	id := markedTanaka(t, n, phone).Conversation.ID

	callTool[conversationJSON](t, phone.connect(), "career_set_suggested_reply", map[string]any{
		"conversationId": id, "text": "Thank you! I'm not looking right now, but let's keep in touch.",
		"language": "en", "kind": "answer", "answers": pitched.Format(time.RFC3339),
	})

	if r := phone.suggestedReply(id); r == nil || r.MarkedRepliedAt != nil {
		t.Errorf("after career_set_suggested_reply, the Suggested Reply is %+v; want the new one, not marked", r)
	}
	if screen := get[careerScreenJSON](phone, "/api/career"); len(screen.Waiting) != 1 {
		t.Errorf("waiting = %+v; want Tanaka waiting again", screen.Waiting)
	}
}

func TestAMarkedAnswerNeverTurnsLate(t *testing.T) {
	n, phone := signedInBrowser(t)
	id := markedTanaka(t, n, phone).Conversation.ID

	n.tickAt(pitched.Add(30*day + time.Minute))

	if r := phone.suggestedReply(id); r == nil || r.Kind != "answer" || r.MarkedRepliedAt == nil {
		t.Errorf("over 30 days after Tanaka's pitch, the marked Suggested Reply is %+v; want it still a marked answer", r)
	}
	if items := phone.claimWorkAt(pitched.Add(30*day + 2*time.Minute)); len(items) != 0 {
		t.Errorf("the run claimed %+v; want no late_reply Item", items)
	}
}

func TestAnOpportunityInProcessIsFollowedUp14DaysAfterTheMark(t *testing.T) {
	n, phone := signedInBrowser(t)
	id := markedTanakaInProcess(t, n, phone).Conversation.ID

	n.tickAt(marked.Add(14*day - time.Minute))
	if items := phone.claimWorkAt(marked.Add(14*day - 30*time.Second)); len(items) != 0 {
		t.Fatalf("a minute short of 14 days after the mark, the run claimed %+v; want nothing", items)
	}
	due := marked.Add(14 * day)
	n.tickAt(due)
	items := phone.claimWorkAt(due.Add(time.Minute))
	if len(items) != 1 || items[0].Kind != "follow_up" || items[0].Conversation.ID != id {
		t.Fatalf("14 days after the mark, the run claimed %+v; want one follow_up Item for Tanaka's Conversation", items)
	}

	// The run follows up the mark, as the Owner's latest message.
	callTool[conversationJSON](t, phone.connect(), "career_set_suggested_reply", map[string]any{
		"conversationId": id, "text": "Hello Tanaka-san, I wanted to follow up.",
		"language": "en", "kind": "follow_up", "answers": marked.Format(time.RFC3339),
	})
	if r := phone.suggestedReply(id); r == nil || r.Kind != "follow_up" || r.MarkedRepliedAt != nil {
		t.Errorf("after the run's follow-up, the Suggested Reply is %+v; want the follow-up, not marked", r)
	}
}

func TestUndoTakesBackTheMarksFollowUp(t *testing.T) {
	n, phone := signedInBrowser(t)
	id := markedTanakaInProcess(t, n, phone).Conversation.ID
	due := marked.Add(14 * day)
	n.tickAt(due)

	// The Owner never sent it after all.
	n.clock.Set(due.Add(time.Minute))
	phone.undoReplied(id)

	if items := phone.claimWorkAt(due.Add(2 * time.Minute)); len(items) != 0 {
		t.Errorf("after Undo, the run claimed %+v; want no follow_up Item: the Owner hasn't sent the answer yet", items)
	}
	if r := phone.suggestedReply(id); r == nil || r.Kind != "answer" || r.MarkedRepliedAt != nil {
		t.Errorf("after Undo, the Suggested Reply is %+v; want the answer back, not marked", r)
	}
}

func TestTheOwnersMessageASyncBringsAfterTheMarksFollowUpGetsNoSecond(t *testing.T) {
	n, phone := signedInBrowser(t)
	markedTanakaInProcess(t, n, phone)
	due := marked.Add(14 * day)
	n.tickAt(due)
	// No run writes the follow-up: after 3 lapsed claims it is set aside.
	for claim := 1; claim <= 3; claim++ {
		if items := phone.claimWorkAt(due.Add(time.Duration(claim-1)*16*time.Minute + time.Minute)); len(items) != 1 {
			t.Fatalf("claim %d got %+v; want the follow_up Item", claim, items)
		}
	}
	n.tickAt(due.Add(2 * time.Hour))

	// The Mac was closed: the Owner's reply, sent just before the mark,
	// comes only now.
	callTool[recordedJSON](t, phone.macAt(due.Add(3*time.Hour)), "career_record_sync", map[string]any{"threads": []any{
		tanakaThread(message("m2", "owner", at(0, 20, 50), "Thank you for reaching out! Could you share the job description?")),
	}})
	n.tickAt(due.Add(4 * time.Hour))

	if items := phone.claimWorkAt(due.Add(4*time.Hour + time.Minute)); len(items) != 0 {
		t.Errorf("the run claimed %+v; want no second follow-up: the mark stood for that message", items)
	}
}
