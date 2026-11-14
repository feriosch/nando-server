package tests

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/feriosch/nando-server/internal/japanese"
)

// triggerPath is where the scheduled task's API trigger is, under the fake's
// URL, as claude.ai shows it.
const triggerPath = "/v1/claude_code/routines/trig_01ABC/fire"

// triggerToken is the trigger's bearer token, as claude.ai shows it.
const triggerToken = "sk-ant-oat01-trigger-token"

// fire is one call the fake trigger received.
type fire struct {
	method, path string
	header       http.Header
	body         string
}

// fakeTrigger is the scheduled task's API trigger: it records each fire and
// answers with status.
type fakeTrigger struct {
	url string

	mu     sync.Mutex
	fires  []fire
	status int
}

// newFakeTrigger starts a fake trigger that answers each fire as the real one
// does when a run starts.
func newFakeTrigger(t *testing.T) *fakeTrigger {
	t.Helper()
	f := &fakeTrigger{status: http.StatusOK}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.fires = append(f.fires, fire{method: r.Method, path: r.URL.Path, header: r.Header.Clone(), body: string(body)})
		status := f.status
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status == http.StatusOK {
			_, _ = io.WriteString(w, `{"type":"routine_fire","claude_code_session_id":"session_01XYZ","claude_code_session_url":"https://claude.ai/code/session_01XYZ"}`)
		}
	}))
	t.Cleanup(srv.Close)
	f.url = srv.URL
	return f
}

// received is every fire so far.
func (f *fakeTrigger) received() []fire {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fire(nil), f.fires...)
}

// config is the Japanese Module waking this fake.
func (f *fakeTrigger) config() japanese.Config {
	return japanese.Config{WordRequestsTriggerURL: f.url + triggerPath, WordRequestsTriggerToken: triggerToken}
}

func TestANewWordRequestWakesTheScheduledTask(t *testing.T) {
	trigger := newFakeTrigger(t)
	_, phone := signedInBrowserWith(t, trigger.config())

	askFor(phone, map[string]any{"text": "食べた", "kind": "add", "group": "gaming"}, http.StatusCreated)

	fires := trigger.received()
	if len(fires) != 1 {
		t.Fatalf("the trigger was fired %d times; want once", len(fires))
	}
	got := fires[0]
	if got.method != http.MethodPost || got.path != triggerPath || got.body != "{}" ||
		got.header.Get("Authorization") != "Bearer "+triggerToken ||
		got.header.Get("anthropic-version") != "2023-06-01" ||
		got.header.Get("anthropic-beta") != "experimental-cc-routine-2026-04-01" ||
		got.header.Get("Content-Type") != "application/json" {
		t.Fatalf("the trigger got %s %s with body %q and headers %v; want POST %s with {}, the bearer token and the anthropic headers",
			got.method, got.path, got.body, got.header, triggerPath)
	}
}

func TestAnAnsweredOrRetriedWordRequestWakesTheScheduledTask(t *testing.T) {
	for _, tc := range []struct {
		action string
		stored bson.M
		body   any
	}{
		{action: "answer", stored: parkedRequest("かける"), body: map[string]any{"answer": "掛ける (to hang)"}},
		{action: "retry", stored: storedRequest("かける", "add", "failed")},
	} {
		t.Run(tc.action, func(t *testing.T) {
			trigger := newFakeTrigger(t)
			n, phone := signedInBrowserWith(t, trigger.config())
			n.store("word_requests", tc.stored)

			act(phone, id(tc.stored), tc.action, tc.body)

			if fires := trigger.received(); len(fires) != 1 {
				t.Fatalf("after the Owner's %s the trigger was fired %d times; want once", tc.action, len(fires))
			}
		})
	}
}

func TestTheScheduledTaskIsWokenAtMostOnceEveryTwoMinutes(t *testing.T) {
	trigger := newFakeTrigger(t)
	n, phone := signedInBrowserWith(t, trigger.config())
	firstAt := n.clock.Now()
	askFor(phone, map[string]any{"text": "一", "kind": "add", "group": "gaming"}, http.StatusCreated)

	n.clock.Set(firstAt.Add(2*time.Minute - time.Second))
	askFor(phone, map[string]any{"text": "二", "kind": "add", "group": "gaming"}, http.StatusCreated)
	early := len(trigger.received())
	n.clock.Set(firstAt.Add(2 * time.Minute))
	askFor(phone, map[string]any{"text": "三", "kind": "add", "group": "gaming"}, http.StatusCreated)
	late := len(trigger.received())

	if early != 1 || late != 2 {
		t.Fatalf("fired %d times by a second short of 2 minutes and %d by 2 minutes; want 1, then 2", early, late)
	}
}

func TestTheScheduledTaskIsNotWokenWhileARunHoldsAWordRequest(t *testing.T) {
	trigger := newFakeTrigger(t)
	n, phone := signedInBrowserWith(t, trigger.config())
	askAt(n, phone, "一")
	claimOne(t, phone)

	n.clock.Set(n.clock.Now().Add(5 * time.Minute))
	askFor(phone, map[string]any{"text": "二", "kind": "add", "group": "gaming"}, http.StatusCreated)

	if fires := trigger.received(); len(fires) != 1 {
		t.Fatalf("the trigger was fired %d times; want only for 一, as the run holding it lists 二 next", len(fires))
	}
}

func TestARunResolvingItsLastWordRequestDoesNotWakeASecondRun(t *testing.T) {
	trigger := newFakeTrigger(t)
	n, phone := signedInBrowserWith(t, trigger.config())
	askAt(n, phone, "一")
	run, wordRequestID := claimOne(t, phone)
	askAt(n, phone, "二") // waiting, while the run holds 一

	n.clock.Set(n.clock.Now().Add(5 * time.Minute))
	callTool[map[string]any](t, run, "japanese_resolve_word_request", map[string]any{
		"id": wordRequestID, "outcome": "failed", "summary": "jisho.org has no word 一",
	})

	// The run lists again itself and takes 二: a second run would only
	// share the queue, at the Owner's usage. Leftovers after a run stops at
	// its limit wait for the Owner's next ask or the tick.
	if fires := trigger.received(); len(fires) != 1 {
		t.Fatalf("the trigger was fired %d times; want once, for 一 only", len(fires))
	}
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

func TestTheTickWakesTheScheduledTaskOnlyForWordRequestsWaitingForClaude(t *testing.T) {
	t.Run("one still pending", func(t *testing.T) {
		trigger := newFakeTrigger(t)
		n, phone := signedInBrowserWith(t, trigger.config())
		askAt(n, phone, "食べた") // say the run it woke hit the Owner's usage limit

		n.clock.Set(n.clock.Now().Add(30 * time.Minute))
		n.tick()

		if fires := trigger.received(); len(fires) != 2 {
			t.Fatalf("the trigger was fired %d times; want twice: when the Owner asked, and at the tick", len(fires))
		}
	})
	t.Run("one whose run stopped", func(t *testing.T) {
		trigger := newFakeTrigger(t)
		n, _ := signedInBrowserWith(t, trigger.config())
		stuck := storedRequest("食べた", "add", "working")
		stuck["claimedUntil"] = n.clock.Now().Add(-time.Minute)
		n.store("word_requests", stuck)

		n.tick()

		if fires := trigger.received(); len(fires) != 1 {
			t.Fatalf("the trigger was fired %d times; want once, for the Word Request no run finished", len(fires))
		}
	})
	t.Run("none", func(t *testing.T) {
		trigger := newFakeTrigger(t)
		n, _ := signedInBrowserWith(t, trigger.config())
		n.store("word_requests", storedRequest("見た", "add", "done"), parkedRequest("かける"), storedRequest("聞いた", "add", "failed"))

		n.tick()

		if fires := trigger.received(); len(fires) != 0 {
			t.Fatalf("the trigger was fired %d times with nothing waiting for Claude; want never", len(fires))
		}
	})
}

func TestWithoutATriggerNothingIsWokenAndWordRequestsStillWork(t *testing.T) {
	for name, half := range map[string]func(japanese.Config) japanese.Config{
		"no token": func(c japanese.Config) japanese.Config { c.WordRequestsTriggerToken = ""; return c },
		"no URL":   func(c japanese.Config) japanese.Config { c.WordRequestsTriggerURL = ""; return c },
	} {
		t.Run(name, func(t *testing.T) {
			trigger := newFakeTrigger(t)
			n, phone := signedInBrowserWith(t, half(trigger.config()))

			askAt(n, phone, "一")
			run, wordRequestID := claimOne(t, phone)
			askAt(n, phone, "二")
			callTool[map[string]any](t, run, "japanese_resolve_word_request", map[string]any{
				"id": wordRequestID, "outcome": "failed", "summary": "jisho.org has no word 一",
			})
			n.clock.Set(n.clock.Now().Add(30 * time.Minute))
			n.tick()

			if fires := trigger.received(); len(fires) != 0 {
				t.Fatalf("the trigger was fired %d times; want never, as it isn't fully configured", len(fires))
			}
		})
	}
}

// answer has the fake trigger answer every fire from now on with status.
func (f *fakeTrigger) answer(status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status = status
}

func TestATriggerThatFailsNeverFailsTheOwnersRequest(t *testing.T) {
	for _, status := range []int{http.StatusInternalServerError, http.StatusTooManyRequests} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			trigger := newFakeTrigger(t)
			trigger.answer(status)
			_, phone := signedInBrowserWith(t, trigger.config())

			asked := askFor(phone, map[string]any{"text": "食べた", "kind": "add", "group": "gaming"}, http.StatusCreated)

			if asked.State != "pending" || len(trigger.received()) != 1 {
				t.Fatalf("with the trigger answering %d, the Owner got %+v after %d fires; want the pending Word Request, after one",
					status, asked, len(trigger.received()))
			}
		})
	}
}

func TestAskingAgainWakesTheScheduledTaskWhenTheFirstWakeFailed(t *testing.T) {
	trigger := newFakeTrigger(t)
	trigger.answer(http.StatusUnauthorized) // say, a bad token, since fixed
	n, phone := signedInBrowserWith(t, trigger.config())
	askAt(n, phone, "度重なる")
	trigger.answer(http.StatusOK)

	n.clock.Set(n.clock.Now().Add(2 * time.Minute))
	askFor(phone, map[string]any{"text": "度重なる", "kind": "add", "group": "literature"}, http.StatusOK)

	if fires := trigger.received(); len(fires) != 2 {
		t.Fatalf("the trigger was fired %d times; want twice: the failed wake, then again when the Owner asked again", len(fires))
	}
}

// captureLog has the log write to the returned buffer until the test ends.
func captureLog(t *testing.T) *lockedBuffer {
	t.Helper()
	buf := &lockedBuffer{}
	prev := log.Writer()
	log.SetOutput(buf)
	t.Cleanup(func() { log.SetOutput(prev) })
	return buf
}

// lockedBuffer is a bytes.Buffer safe to write from several goroutines.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestATriggerPastedBadlyIsCleanedOrSaidAtStartup(t *testing.T) {
	for _, c := range []struct {
		name string
		// bad changes the fake trigger's config as a paste might.
		bad func(japanese.Config) japanese.Config
		// fires is how many times the trigger should be fired; log, what
		// startup should say (empty for nothing).
		fires int
		log   string
	}{
		{"a token with a trailing newline", func(c japanese.Config) japanese.Config { c.WordRequestsTriggerToken += "\n"; return c }, 1, ""},
		{"a URL with a trailing newline", func(c japanese.Config) japanese.Config { c.WordRequestsTriggerURL += "\n"; return c }, 1, ""},
		{"a token of pasted arrow keys", func(c japanese.Config) japanese.Config { c.WordRequestsTriggerToken = "\x1b[B\x1b[B\x1b[B"; return c }, 0,
			"WORD_REQUESTS_TRIGGER_TOKEN has control characters"},
		{"a token that is only a newline", func(c japanese.Config) japanese.Config { c.WordRequestsTriggerToken = "\n"; return c }, 0,
			"WORD_REQUESTS_TRIGGER_TOKEN is empty"},
		{"a URL with a space inside", func(c japanese.Config) japanese.Config {
			c.WordRequestsTriggerURL = strings.Replace(c.WordRequestsTriggerURL, "/fire", " /fire", 1)
			return c
		}, 0, "WORD_REQUESTS_TRIGGER_URL isn't a URL"},
	} {
		t.Run(c.name, func(t *testing.T) {
			trigger := newFakeTrigger(t)
			logged := captureLog(t)
			_, phone := signedInBrowserWith(t, c.bad(trigger.config()))

			askFor(phone, map[string]any{"text": "食べた", "kind": "add", "group": "gaming"}, http.StatusCreated)

			fires := trigger.received()
			if len(fires) != c.fires {
				t.Fatalf("the trigger was fired %d times; want %d", len(fires), c.fires)
			}
			if c.fires == 1 && fires[0].header.Get("Authorization") != "Bearer "+triggerToken {
				t.Errorf("Authorization = %q; want the token without what was pasted around it", fires[0].header.Get("Authorization"))
			}
			if c.log != "" && !strings.Contains(logged.String(), c.log) {
				t.Errorf("startup logged %q; want it to say %q", logged.String(), c.log)
			}
		})
	}
}
