// Package testtrigger gives tests a fake API trigger of a cloud scheduled
// task (ADR 0006), which records each fire.
package testtrigger

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// Path is where the trigger is, under the fake's URL, as claude.ai shows it.
const Path = "/v1/claude_code/routines/trig_01ABC/fire"

// Token is the trigger's bearer token, as claude.ai shows it.
const Token = "sk-ant-oat01-trigger-token"

// Fire is one call the fake trigger received.
type Fire struct {
	Method, Path string
	Header       http.Header
	Body         string
}

// Trigger is the fake trigger: it records each fire and answers with a
// status.
type Trigger struct {
	// URL is the trigger's whole URL, Path included.
	URL string

	mu     sync.Mutex
	fires  []Fire
	status int
}

// New starts a fake trigger that answers each fire as the real one does when
// a run starts. It stops when the test ends.
func New(t *testing.T) *Trigger {
	t.Helper()
	f := &Trigger{status: http.StatusOK}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.fires = append(f.fires, Fire{Method: r.Method, Path: r.URL.Path, Header: r.Header.Clone(), Body: string(body)})
		status := f.status
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status == http.StatusOK {
			_, _ = io.WriteString(w, `{"type":"routine_fire","claude_code_session_id":"session_01XYZ","claude_code_session_url":"https://claude.ai/code/session_01XYZ"}`)
		}
	}))
	t.Cleanup(srv.Close)
	f.URL = srv.URL + Path
	return f
}

// Received is every fire so far.
func (f *Trigger) Received() []Fire {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Fire(nil), f.fires...)
}

// Answer has the trigger answer every fire from now on with status.
func (f *Trigger) Answer(status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status = status
}
