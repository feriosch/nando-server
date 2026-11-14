package tests

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/feriosch/nando-server/internal/testpush"
)

// subscribe registers push's Push Subscription, as the web app does once the
// Owner allows Notifications on this device.
func (d *browser) subscribe(push *testpush.Service) {
	d.nando.t.Helper()
	d.must(http.MethodPost, "/api/push-subscriptions", push.SubscriptionJSON(), http.StatusNoContent)
}

// tick ticks as Cloud Scheduler does, at the time on Core's clock, failing
// the test unless the tick succeeds.
func (n *nando) tick() {
	n.t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/internal/tick", nil)
	req.Header.Set("Authorization", "Bearer scheduler-token")
	rec := httptest.NewRecorder()
	n.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		n.t.Fatalf("tick: status = %d, body %q", rec.Code, rec.Body)
	}
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
