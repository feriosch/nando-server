package tests

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

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
	return japanese.Config{ClaudeTriggerURL: f.url + triggerPath, ClaudeTriggerToken: triggerToken}
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
