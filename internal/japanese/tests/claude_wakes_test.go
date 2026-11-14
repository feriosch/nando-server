package tests

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/feriosch/nando-server/internal/japanese"
)

// triggerToken is the scheduled task's API trigger token, as the Owner
// copied it from claude.ai.
const triggerToken = "sk-ant-oat01-trigger-token"

// triggerPath is where the fake scheduled task's API trigger is.
const triggerPath = "/v1/claude_code/routines/trig_01ABC/fire"

// fire is a call the fake trigger received.
type fire struct {
	method, path string
	header       http.Header
	body         string
}

// trigger is the API trigger of the cloud scheduled task that works through
// the Word Requests, faked: it records each fire.
type trigger struct {
	server *httptest.Server
	mu     sync.Mutex
	fires  []fire
	status int
}

func newTrigger(t *testing.T) *trigger {
	t.Helper()
	tr := &trigger{status: http.StatusOK}
	tr.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		tr.mu.Lock()
		tr.fires = append(tr.fires, fire{method: r.Method, path: r.URL.Path, header: r.Header.Clone(), body: string(body)})
		status := tr.status
		tr.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status == http.StatusOK {
			_, _ = io.WriteString(w, `{"type":"routine_fire","claude_code_session_id":"session_01XYZ","claude_code_session_url":"https://claude.ai/code/session_01XYZ"}`)
		}
	}))
	t.Cleanup(tr.server.Close)
	return tr
}

// received is every fire so far.
func (tr *trigger) received() []fire {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return append([]fire(nil), tr.fires...)
}

// wakingNando is Nando with the Owner's iPhone signed in and the scheduled
// task's API trigger configured, faked by the returned trigger.
func wakingNando(t *testing.T) (*nando, *browser, *trigger) {
	t.Helper()
	tr := newTrigger(t)
	n, phone := signedInBrowserWith(t, japanese.Config{
		ClaudeTriggerURL:   tr.server.URL + triggerPath,
		ClaudeTriggerToken: triggerToken,
	})
	return n, phone, tr
}

func TestANewWordRequestWakesClaudeOnce(t *testing.T) {
	_, phone, tr := wakingNando(t)

	askFor(phone, map[string]any{"text": "食べた", "kind": "add", "group": "gaming"}, http.StatusCreated)

	fires := tr.received()
	if len(fires) != 1 {
		t.Fatalf("the trigger was fired %d times; want once", len(fires))
	}
	f := fires[0]
	if f.method != http.MethodPost || f.path != triggerPath {
		t.Errorf("fired %s %s; want POST %s", f.method, f.path, triggerPath)
	}
	for name, want := range map[string]string{
		"Authorization":     "Bearer " + triggerToken,
		"Anthropic-Version": "2023-06-01",
		"Anthropic-Beta":    "experimental-cc-routine-2026-04-01",
		"Content-Type":      "application/json",
	} {
		if got := f.header.Get(name); got != want {
			t.Errorf("%s = %q; want %q", name, got, want)
		}
	}
	if f.body != "{}" {
		t.Errorf("body = %q; want {}", f.body)
	}
}

func TestAnAnsweredAndARetriedWordRequestWakeClaude(t *testing.T) {
	for action, tc := range map[string]struct {
		stored bson.M
		body   any
	}{
		"answer": {stored: parkedRequest("かける"), body: map[string]any{"answer": "掛ける (to hang)"}},
		"retry":  {stored: storedRequest("かける", "add", "failed")},
	} {
		t.Run(action, func(t *testing.T) {
			n, phone, tr := wakingNando(t)
			n.store("word_requests", tc.stored)

			act(phone, id(tc.stored), action, tc.body)

			if got := len(tr.received()); got != 1 {
				t.Fatalf("after the Owner's %s the trigger was fired %d times; want once", action, got)
			}
		})
	}
}

func TestClaudeIsWokenAtMostOnceEveryTwoMinutes(t *testing.T) {
	n, phone, tr := wakingNando(t)
	start := n.clock.Now()
	ask := func(after time.Duration, text string) int {
		n.clock.Set(start.Add(after))
		askFor(phone, map[string]any{"text": text, "kind": "add", "group": "gaming"}, http.StatusCreated)
		return len(tr.received())
	}

	first := ask(0, "食べた")
	soon := ask(2*time.Minute-time.Second, "見た")
	later := ask(2*time.Minute, "書いた")

	if first != 1 || soon != 1 || later != 2 {
		t.Fatalf("fired %d, %d, %d times by each ask; want 1, still 1 a second short of 2 minutes, then 2", first, soon, later)
	}
}

func TestClaudeIsNotWokenWhileARunHoldsAWordRequest(t *testing.T) {
	n, phone, tr := wakingNando(t)
	held := storedRequest("見た", "add", "working")
	held["claimedUntil"] = n.clock.Now().Add(10 * time.Minute)
	n.store("word_requests", held)

	askFor(phone, map[string]any{"text": "食べた", "kind": "add", "group": "gaming"}, http.StatusCreated)
	whileHeld := len(tr.received())
	n.clock.Set(n.clock.Now().Add(10 * time.Minute))
	askFor(phone, map[string]any{"text": "書いた", "kind": "add", "group": "gaming"}, http.StatusCreated)
	once := len(tr.received())

	if whileHeld != 0 || once != 1 {
		t.Fatalf("fired %d times while a run held a Word Request and %d once its claim ran out; want 0, then 1", whileHeld, once)
	}
}
