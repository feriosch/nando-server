// Package testtelegram gives tests a fake Telegram Bot API (ADR 0008), which
// records each call a bot makes.
package testtelegram

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// Call is one call a bot made to the fake.
type Call struct {
	// Token is the bot's token, from the call's path.
	Token string
	// Method is the Bot API method, such as sendMessage.
	Method string
	// Params is the call's JSON body.
	Params map[string]any
}

// Telegram is the fake Bot API.
type Telegram struct {
	// URL is the API's base URL, in place of https://api.telegram.org.
	URL string

	mu       sync.Mutex
	calls    []Call
	failing  map[string]bool
	webhooks map[string]Webhook
}

// Webhook is one bot's webhook, as setWebhook last set it.
type Webhook struct {
	URL string
	// SecretToken is the secret_token the webhook was last set with. The real
	// Bot API never tells it; the fake keeps it for tests to check.
	SecretToken string
}

// New starts a fake Bot API that answers every call as successful. It stops
// when the test ends.
func New(t *testing.T) *Telegram {
	t.Helper()
	f := &Telegram{failing: map[string]bool{}, webhooks: map[string]Webhook{}}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	f.URL = srv.URL
	return f
}

func (f *Telegram) serve(w http.ResponseWriter, r *http.Request) {
	// The path is /bot<token>/<method>.
	token, method, ok := strings.Cut(strings.TrimPrefix(r.URL.Path, "/bot"), "/")
	if !ok || !strings.HasPrefix(r.URL.Path, "/bot") {
		http.NotFound(w, r)
		return
	}
	body, _ := io.ReadAll(r.Body)
	params := map[string]any{}
	if len(body) > 0 {
		_ = json.Unmarshal(body, &params)
	}

	f.mu.Lock()
	f.calls = append(f.calls, Call{Token: token, Method: method, Params: params})
	failing := f.failing[method]
	var result any = true
	switch method {
	case "setWebhook":
		if !failing {
			url, _ := params["url"].(string)
			secret, _ := params["secret_token"].(string)
			f.webhooks[token] = Webhook{URL: url, SecretToken: secret}
		}
	case "sendMessage":
		result = map[string]any{"message_id": len(f.calls)}
	}
	f.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	if failing {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"ok":false,"error_code":500,"description":"Internal Server Error"}`)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": result})
}

// Calls is every call so far.
func (f *Telegram) Calls() []Call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Call(nil), f.calls...)
}

// CallsTo is every call so far to method.
func (f *Telegram) CallsTo(method string) []Call {
	var calls []Call
	for _, c := range f.Calls() {
		if c.Method == method {
			calls = append(calls, c)
		}
	}
	return calls
}

// Fail has every call to method fail from now on, as when Telegram is down.
func (f *Telegram) Fail(method string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failing[method] = true
}

// WebhookOf is the webhook the bot with token has now.
func (f *Telegram) WebhookOf(token string) Webhook {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.webhooks[token]
}
