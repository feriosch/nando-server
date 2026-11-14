package tests

import (
	"net/http"
	"net/http/httptest"
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

// tick ticks as Cloud Scheduler does, at the time on Core's clock, failing the
// test unless the tick succeeds.
func (n *nando) tick() {
	n.t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/internal/tick", nil)
	req.Header.Set("Authorization", "Bearer scheduler-token")
	rec := httptest.NewRecorder()
	n.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		n.t.Fatalf("tick at %s: status = %d, body %q", n.clock.Now(), rec.Code, rec.Body)
	}
}

// subscribedPhone is Nando with the Owner's iPhone signed in and subscribed
// to Notifications, and the push service that receives them.
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

	want := []testpush.Notification{{Title: "Claude has a question", Body: "「かける」: Which かける: to hang, or to call?", URL: "/japanese"}}
	if got := push.Received(); !slices.Equal(got, want) {
		t.Errorf("received %+v, want %+v", got, want)
	}
}

func TestAFailedWordRequestNotifiesTheOwnerWhy(t *testing.T) {
	n, phone, push := subscribedPhone(t)
	askAt(n, phone, "たべたべ")
	run, wordRequestID := claimOne(t, phone)
	callTool[map[string]any](t, run, "japanese_resolve_word_request", map[string]any{
		"id":      wordRequestID,
		"outcome": "failed",
		"summary": "jisho.org has no word たべたべ, and it isn't a conjugation of one",
	})

	n.tick()

	want := []testpush.Notification{{Title: "Couldn't add a word", Body: "「たべたべ」: jisho.org has no word たべたべ, and it isn't a conjugation of one", URL: "/japanese"}}
	if got := push.Received(); !slices.Equal(got, want) {
		t.Errorf("received %+v, want %+v", got, want)
	}
}

func TestADoneWordRequestDoesNotNotifyTheOwner(t *testing.T) {
	n, phone, push := subscribedPhone(t)
	eat := word("食べる", "たべる", "comer", "basic")
	n.store("words", eat)
	askAt(n, phone, "食べた")
	run, wordRequestID := claimOne(t, phone)
	callTool[map[string]any](t, run, "japanese_resolve_word_request", map[string]any{
		"id":      wordRequestID,
		"outcome": "done",
		"summary": "Added 食べる · 3 Word Links · 1 Hint",
		"wordId":  id(eat),
	})

	n.tick()

	if got := push.Received(); len(got) != 0 {
		t.Errorf("received %+v, want nothing: a done Word Request shows in the Pending strip", got)
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
	n.tick() // Cloud Scheduler retries a tick it thinks failed
	n.clock.Set(n.clock.Now().Add(30 * time.Minute))
	n.tick() // the next tick, with the question still unanswered

	want := []testpush.Notification{{Title: "Claude has a question", Body: "「かける」: Which かける: to hang, or to call?", URL: "/japanese"}}
	if got := push.Received(); !slices.Equal(got, want) {
		t.Errorf("received %+v, want it once: %+v", got, want)
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
	n.clock.Set(n.clock.Now().Add(10 * time.Minute))
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
