package tests

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
)

// serpAPIKey is the fake SerpApi's key.
const serpAPIKey = "serp-test-key"

// fakeSerpAPI is SerpApi's Google Flights search, answering with what a test
// sets and recording every request.
type fakeSerpAPI struct {
	URL string

	mu       sync.Mutex
	requests []url.Values
	status   int
	body     string
}

func newFakeSerpAPI(t *testing.T) *fakeSerpAPI {
	t.Helper()
	f := &fakeSerpAPI{status: http.StatusOK, body: googleFlightsAnswer}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.URL.Path != "/search.json" {
			http.NotFound(w, r)
			return
		}
		f.requests = append(f.requests, r.URL.Query())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(f.status)
		_, _ = w.Write([]byte(f.body))
	}))
	t.Cleanup(server.Close)
	f.URL = server.URL
	return f
}

// answer makes the fake answer every later request with status and body.
func (f *fakeSerpAPI) answer(status int, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status, f.body = status, body
}

// calls are the query strings of every request so far.
func (f *fakeSerpAPI) calls() []url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]url.Values(nil), f.requests...)
}

// googleFlightsAnswer is a made-up Google Flights answer, trimmed to what
// Nando reads: two offers and the price insights.
const googleFlightsAnswer = `{
  "search_metadata": {"status": "Success"},
  "best_flights": [
    {"price": 230000, "flights": [{"airline": "Air X", "flight_number": "XX 1"}, {"airline": "Air Y", "flight_number": "YY 2"}]}
  ],
  "other_flights": [
    {"price": 221000, "flights": [{"airline": "Air Z", "flight_number": "ZZ 3"}]},
    {"price": 260000, "flights": [{"airline": "Air X", "flight_number": "XX 5"}]}
  ],
  "price_insights": {
    "lowest_price": 221000,
    "price_level": "low",
    "typical_price_range": [240000, 300000],
    "price_history": [[1795000000, 250000], [1795086400, 240000], [1795172800, 221000]]
  }
}`
