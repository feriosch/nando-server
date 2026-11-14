package tests

import (
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/feriosch/nando-server/internal/testpush"
)

// subscribe registers push's Push Subscription, as the web app does once the
// Owner allows Notifications on this device.
func (d *browser) subscribe(push *testpush.Service) {
	d.nando.t.Helper()
	d.must(http.MethodPost, "/api/push-subscriptions", push.SubscriptionJSON(), http.StatusNoContent)
}

// subscribedPhone is a signed-in browser on the Owner's iPhone that has
// allowed Notifications, and the push service its Notifications reach.
func subscribedPhone(t *testing.T) (*nando, *browser, *testpush.Service) {
	t.Helper()
	n, phone := signedInBrowser(t)
	push := testpush.New(t)
	phone.subscribe(push)
	return n, phone, push
}

func TestAWordRequestNeedingAnAnswerNotifiesTheOwnerWithClaudesQuestion(t *testing.T) {
	n, phone, push := subscribedPhone(t)
	askAt(n, phone, "かける")
	run, wordRequestID := claimOne(t, phone)
	callTool[map[string]any](t, run, "japanese_resolve_word_request", map[string]any{
		"id":       wordRequestID,
		"outcome":  "needs_answer",
		"question": "Which かける: to hang, or to call?",
		"choices":  []string{"掛ける (to hang)", "架ける (to call)"},
	})

	n.tick()

	want := []testpush.Notification{{
		Title: "Claude has a question",
		Body:  "「かける」: Which かける: to hang, or to call?",
		URL:   "/japanese",
	}}
	if got := push.Received(); !slices.Equal(got, want) {
		t.Errorf("received %+v, want %+v", got, want)
	}
}

func TestAWordRequestClaudeGivesUpOnNotifiesTheOwnerWhy(t *testing.T) {
	n, phone, push := subscribedPhone(t)
	askAt(n, phone, "たべたべ")
	run, wordRequestID := claimOne(t, phone)
	callTool[map[string]any](t, run, "japanese_resolve_word_request", map[string]any{
		"id":      wordRequestID,
		"outcome": "failed",
		"summary": "jisho.org has no word たべたべ, and it isn't a conjugation of one",
	})

	n.tick()

	want := []testpush.Notification{{
		Title: "Couldn't add a word",
		Body:  "「たべたべ」: jisho.org has no word たべたべ, and it isn't a conjugation of one",
		URL:   "/japanese",
	}}
	if got := push.Received(); !slices.Equal(got, want) {
		t.Errorf("received %+v, want %+v", got, want)
	}
}

func TestAWordRequestClaudeFinishedDoesNotNotifyTheOwner(t *testing.T) {
	n, phone, push := subscribedPhone(t)
	eat := word("食べる", "たべる", "comer", "basic")
	n.store("words", eat)
	askAt(n, phone, "食べた")
	run, wordRequestID := claimOne(t, phone)
	callTool[map[string]any](t, run, "japanese_resolve_word_request", map[string]any{
		"id":      wordRequestID,
		"outcome": "done",
		"summary": "Added 食べる",
		"wordId":  id(eat),
	})

	n.tick()

	if got := push.Received(); len(got) != 0 {
		t.Errorf("received %+v, want nothing", got)
	}
}

func TestARetriedTickNeverNotifiesTheOwnerTwice(t *testing.T) {
	n, phone, push := subscribedPhone(t)
	askAt(n, phone, "かける")
	run, wordRequestID := claimOne(t, phone)
	callTool[map[string]any](t, run, "japanese_resolve_word_request", map[string]any{
		"id":       wordRequestID,
		"outcome":  "needs_answer",
		"question": "Which かける: to hang, or to call?",
		"choices":  []string{"掛ける (to hang)", "架ける (to call)"},
	})

	n.tick()
	n.clock.Set(n.clock.Now().Add(time.Minute)) // Cloud Scheduler retries a tick it thinks failed
	n.tick()
	n.clock.Set(n.clock.Now().Add(30 * time.Minute)) // the next tick, still unanswered
	n.tick()

	if got := push.Received(); len(got) != 1 {
		t.Errorf("received %d push messages, want 1: %+v", len(got), got)
	}
}

func TestASecondQuestionOnTheSameWordRequestNotifiesTheOwnerAgain(t *testing.T) {
	n, phone, push := subscribedPhone(t)
	askAt(n, phone, "かける")
	run, wordRequestID := claimOne(t, phone)
	ask := func(question string) {
		callTool[map[string]any](t, run, "japanese_resolve_word_request", map[string]any{
			"id": wordRequestID, "outcome": "needs_answer", "question": question, "choices": []string{"one", "two"},
		})
	}
	ask("Which かける?")
	n.tick()
	n.clock.Set(n.clock.Now().Add(10 * time.Minute)) // the Owner answers
	act(phone, wordRequestID, "answer", map[string]any{"answer": "one"})
	run, _ = claimOne(t, phone)

	ask("Which sense of 掛ける?")
	n.clock.Set(n.clock.Now().Add(30 * time.Minute))
	n.tick()

	want := []testpush.Notification{
		{Title: "Claude has a question", Body: "「かける」: Which かける?", URL: "/japanese"},
		{Title: "Claude has a question", Body: "「かける」: Which sense of 掛ける?", URL: "/japanese"},
	}
	if got := push.Received(); !slices.Equal(got, want) {
		t.Errorf("received %+v, want %+v", got, want)
	}
}

func TestAWordRequestNoRunFinishedNotifiesTheOwnerAtTheNextTick(t *testing.T) {
	n, _, push := subscribedPhone(t)
	// Its 3rd claim has run out, and neither the Owner nor a run has listed
	// Word Requests since: only the tick can tell the Owner.
	stuck := storedRequest("食べた", "add", "working")
	stuck["attempts"] = 3
	stuck["claimedUntil"] = n.clock.Now().Add(-time.Minute)
	n.store("word_requests", stuck)

	n.tick()

	got := push.Received()
	if len(got) != 1 || got[0].Title != "Couldn't add a word" {
		t.Errorf("received %+v; want one Notification that 食べた couldn't be added", got)
	}
}

func TestAnImproveClaudeGivesUpOnSaysItCouldntImproveTheWord(t *testing.T) {
	n, phone, push := subscribedPhone(t)
	hang := word("掛ける", "かける", "colgar", "jlpt_n2")
	n.store("words", hang)
	askFor(phone, map[string]any{"kind": "improve", "wordId": id(hang)}, http.StatusCreated)
	run, wordRequestID := claimOne(t, phone)
	callTool[map[string]any](t, run, "japanese_resolve_word_request", map[string]any{
		"id": wordRequestID, "outcome": "failed", "summary": "the Word was deleted meanwhile",
	})

	n.tick()

	got := push.Received()
	if len(got) != 1 || got[0].Title != "Couldn't improve a word" {
		t.Errorf("received %+v; want one Notification that 掛ける couldn't be improved", got)
	}
}

func TestAFailedWordRequestTheOwnerDismissedSendsNoNotification(t *testing.T) {
	n, phone, push := subscribedPhone(t)
	askAt(n, phone, "たべたべ")
	run, wordRequestID := claimOne(t, phone)
	callTool[map[string]any](t, run, "japanese_resolve_word_request", map[string]any{
		"id": wordRequestID, "outcome": "failed", "summary": "jisho.org has no word たべたべ",
	})
	// The Owner saw it in the Pending strip before the tick, and dismissed it.
	act(phone, wordRequestID, "dismiss", nil)

	n.tick()

	if got := push.Received(); len(got) != 0 {
		t.Errorf("received %+v; want nothing for a dismissed Word Request", got)
	}
}
