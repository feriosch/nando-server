package tests

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/feriosch/nando-server/internal/career"
	"github.com/feriosch/nando-server/internal/testtrigger"
)

// config is the Career Module waking the fake trigger.
func config(trigger *testtrigger.Trigger) career.Config {
	return career.Config{SyncTriggerURL: trigger.URL, SyncTriggerToken: testtrigger.Token}
}

// syncAt records, at now, a Sync with a new thread from someone, which a run
// has to sort.
func syncAt(t *testing.T, phone *browser, now time.Time, threadID string) {
	t.Helper()
	thread := tanakaThread(message(threadID+"-m1", "other", now.Add(-time.Hour), "Hello! I have a role for you."))
	thread["threadId"] = threadID
	callTool[recordedJSON](t, phone.macAt(now), "career_record_sync", map[string]any{"threads": []any{thread}})
}

// tick calls POST /internal/tick as Cloud Scheduler does, and fails the test
// unless it succeeds.
func (n *nando) tick() {
	n.t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/internal/tick", nil)
	req.Header.Set("Authorization", "Bearer google-signed-scheduler-token")
	rec := httptest.NewRecorder()
	n.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		n.t.Fatalf("tick: status = %d, body %q", rec.Code, rec.Body)
	}
}

// firedTimes fails the test unless the trigger was fired want times.
func firedTimes(t *testing.T, trigger *testtrigger.Trigger, want int, why string) {
	t.Helper()
	if got := len(trigger.Received()); got != want {
		t.Fatalf("the trigger was fired %d times; want %d: %s", got, want, why)
	}
}

func TestASyncWithNewMessagesWakesTheScheduledTask(t *testing.T) {
	trigger := testtrigger.New(t)
	_, phone := signedInBrowserWith(t, config(trigger))
	phone.saveTimeZone("Asia/Tokyo")

	syncAt(t, phone, at(0, 8, 0), "2-tanaka")

	firedTimes(t, trigger, 1, "for the Sync's new thread")
	got := trigger.Received()[0]
	if got.Method != http.MethodPost || got.Path != testtrigger.Path || got.Body != "{}" ||
		got.Header.Get("Authorization") != "Bearer "+testtrigger.Token {
		t.Errorf("the trigger got %s %s with %q and headers %v; want POST with {} and the bearer token", got.Method, got.Path, got.Body, got.Header)
	}
}

func TestASyncSentInBatchesWakesTheScheduledTaskOnce(t *testing.T) {
	trigger := testtrigger.New(t)
	_, phone := signedInBrowserWith(t, config(trigger))
	phone.saveTimeZone("Asia/Tokyo")
	start := at(0, 8, 0)
	sent := at(-1, 18, 0)

	mac := phone.macAt(start)
	first := callTool[recordedJSON](t, mac, "career_record_sync", map[string]any{"threads": batch(0, 10, sent)})
	phone.nando.clock.Set(start.Add(4 * time.Second))
	callTool[recordedJSON](t, mac, "career_record_sync", map[string]any{"sync": first.Sync, "threads": batch(10, 10, sent)})
	phone.nando.clock.Set(start.Add(8 * time.Second))
	callTool[recordedJSON](t, mac, "career_record_sync", map[string]any{"sync": first.Sync, "threads": batch(20, 5, sent)})

	firedTimes(t, trigger, 1, "the run the first batch woke lists the later batches' threads too")
}

func TestASyncWithNothingNewDoesNotWakeTheScheduledTask(t *testing.T) {
	trigger := testtrigger.New(t)
	_, phone := signedInBrowserWith(t, config(trigger))
	phone.saveTimeZone("Asia/Tokyo")

	callTool[recordedJSON](t, phone.macAt(at(0, 8, 0)), "career_record_sync", map[string]any{"threads": []any{}})
	callTool[recordedJSON](t, phone.macAt(at(0, 8, 1)), "career_record_sync", map[string]any{"problem": map[string]any{"kind": "session_expired"}})

	firedTimes(t, trigger, 0, "no Sync brought anything")
}

func TestAFireIsInFlightUntilItsRunClaims(t *testing.T) {
	trigger := testtrigger.New(t)
	n, phone := signedInBrowserWith(t, config(trigger))
	phone.saveTimeZone("Asia/Tokyo")
	syncAt(t, phone, at(0, 8, 0), "2-tanaka")

	// The run it woke is still starting: it hasn't claimed anything yet.
	syncAt(t, phone, at(0, 8, 3), "2-sato")
	n.clock.Set(at(0, 8, 5))
	n.tick()
	firedTimes(t, trigger, 1, "the run starting lists the second thread too")

	n.clock.Set(at(0, 8, 10))
	n.tick()
	firedTimes(t, trigger, 2, "10 minutes after a fire no run claimed, the tick fires again")
}

func TestTheScheduledTaskIsNotWokenWhileARunHoldsAClaim(t *testing.T) {
	trigger := testtrigger.New(t)
	n, phone := signedInBrowserWith(t, config(trigger))
	phone.saveTimeZone("Asia/Tokyo")
	syncAt(t, phone, at(0, 8, 0), "2-tanaka")
	n.clock.Set(at(0, 8, 1))
	callTool[workJSON](t, phone.connect(), "career_list_work", nil)

	syncAt(t, phone, at(0, 8, 5), "2-sato")
	n.clock.Set(at(0, 8, 14))
	n.tick()

	firedTimes(t, trigger, 1, "the run holding 2-tanaka lists 2-sato next")
}

func TestASyncJustAfterARunDidAnItemLeavesItToThatRun(t *testing.T) {
	trigger := testtrigger.New(t)
	n, phone := signedInBrowserWith(t, config(trigger))
	phone.saveTimeZone("Asia/Tokyo")
	syncAt(t, phone, at(0, 8, 0), "2-tanaka")
	n.clock.Set(at(0, 8, 1))
	run := phone.connect()
	item := callTool[workJSON](t, run, "career_list_work", nil).Items[0]
	n.clock.Set(at(0, 8, 5))
	callTool[conversationJSON](t, run, "career_ignore_thread", map[string]any{"conversationId": item.Conversation.ID})

	// The run lists the work again right after, so it takes 2-sato.
	syncAt(t, phone, at(0, 8, 6), "2-sato")
	firedTimes(t, trigger, 1, "a minute after the run did an item, it lists again itself")
	// It didn't: it stopped, and no run holds a claim.
	syncAt(t, phone, at(0, 8, 8), "2-suzuki")
	firedTimes(t, trigger, 2, "3 minutes after, no run is left to take the work")
}

func TestASyncJustAfterARunFoundNothingLeftWakesTheScheduledTask(t *testing.T) {
	trigger := testtrigger.New(t)
	n, phone := signedInBrowserWith(t, config(trigger))
	phone.saveTimeZone("Asia/Tokyo")
	syncAt(t, phone, at(0, 8, 0), "2-tanaka")
	n.clock.Set(at(0, 8, 1))
	run := phone.connect()
	item := callTool[workJSON](t, run, "career_list_work", nil).Items[0]
	callTool[conversationJSON](t, run, "career_ignore_thread", map[string]any{"conversationId": item.Conversation.ID})
	if left := callTool[workJSON](t, run, "career_list_work", nil); len(left.Items) != 0 {
		t.Fatalf("the run claimed %+v; want nothing left", left.Items)
	}

	syncAt(t, phone, at(0, 8, 2), "2-sato")

	firedTimes(t, trigger, 2, "the run stopped, so no run is left to take 2-sato")
}

func TestTheTickWakesTheScheduledTaskOnlyWhileWorkWaits(t *testing.T) {
	t.Run("a wake that failed", func(t *testing.T) {
		trigger := testtrigger.New(t)
		trigger.Answer(http.StatusTooManyRequests) // the trigger's limit, 30 fires an hour
		n, phone := signedInBrowserWith(t, config(trigger))
		phone.saveTimeZone("Asia/Tokyo")
		syncAt(t, phone, at(0, 8, 0), "2-tanaka")
		trigger.Answer(http.StatusOK)

		n.clock.Set(at(0, 8, 5))
		n.tick()

		firedTimes(t, trigger, 2, "the failed wake, then at the next tick")
	})
	t.Run("a claim that lapsed", func(t *testing.T) {
		trigger := testtrigger.New(t)
		n, phone := signedInBrowserWith(t, config(trigger))
		phone.saveTimeZone("Asia/Tokyo")
		syncAt(t, phone, at(0, 8, 0), "2-tanaka")
		n.clock.Set(at(0, 8, 1))
		callTool[workJSON](t, phone.connect(), "career_list_work", nil) // and the run stops, say as the Owner's usage ran out

		n.clock.Set(at(0, 8, 15))
		n.tick()
		firedTimes(t, trigger, 1, "the claim is still live at 08:15")
		n.clock.Set(at(0, 8, 20))
		n.tick()
		firedTimes(t, trigger, 2, "the claim lapsed at 08:16")
	})
	t.Run("nothing waiting", func(t *testing.T) {
		trigger := testtrigger.New(t)
		n, phone := signedInBrowserWith(t, config(trigger))
		phone.saveTimeZone("Asia/Tokyo")
		syncAt(t, phone, at(0, 8, 0), "2-tanaka")
		n.clock.Set(at(0, 8, 1))
		run := phone.connect()
		item := callTool[workJSON](t, run, "career_list_work", nil).Items[0]
		callTool[conversationJSON](t, run, "career_ignore_thread", map[string]any{"conversationId": item.Conversation.ID})

		n.clock.Set(at(0, 9, 0))
		n.tick()

		firedTimes(t, trigger, 1, "only for the Sync: nothing waits at the tick")
	})
}

func TestWithoutATriggerNothingIsWoken(t *testing.T) {
	for name, cfg := range map[string]func(career.Config) career.Config{
		"no token": func(c career.Config) career.Config { c.SyncTriggerToken = ""; return c },
		"no URL":   func(c career.Config) career.Config { c.SyncTriggerURL = ""; return c },
	} {
		t.Run(name, func(t *testing.T) {
			trigger := testtrigger.New(t)
			n, phone := signedInBrowserWith(t, cfg(config(trigger)))
			phone.saveTimeZone("Asia/Tokyo")

			syncAt(t, phone, at(0, 8, 0), "2-tanaka")
			n.clock.Set(at(0, 9, 0))
			n.tick()

			firedTimes(t, trigger, 0, "the trigger isn't fully configured")
		})
	}
}
