package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeFrankfurter is Frankfurter's API for the ECB's daily rates, answering
// pesos in yen from a table by Day.
type fakeFrankfurter struct {
	URL string
	mu  sync.Mutex
	// calls are the paths asked, in order.
	calls []string
	// failing makes every call answer 500.
	failing bool
}

// mxnInJPY is the made-up rates the fake publishes: October's average is 8
// and its last published Day is 9; September's is 6 throughout.
var mxnInJPY = map[string]float64{
	"2026-09-01": 6, "2026-09-30": 6,
	"2026-10-01": 7, "2026-10-15": 8, "2026-10-30": 9,
	"2026-11-02": 10,
}

func newFakeFrankfurter(t *testing.T) *fakeFrankfurter {
	t.Helper()
	f := &fakeFrankfurter{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.calls = append(f.calls, r.URL.Path+"?"+r.URL.RawQuery)
		failing := f.failing
		f.mu.Unlock()
		if failing {
			http.Error(w, "down", http.StatusInternalServerError)
			return
		}
		from, to, ok := strings.Cut(strings.TrimPrefix(r.URL.Path, "/v1/"), "..")
		if !ok || r.URL.Query().Get("base") != "MXN" || r.URL.Query().Get("symbols") != "JPY" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		rates := map[string]map[string]float64{}
		for day, rate := range mxnInJPY {
			if day >= from && day <= to {
				rates[day] = map[string]float64{"JPY": rate}
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"amount": 1, "base": "MXN", "start_date": from, "end_date": to, "rates": rates})
	}))
	t.Cleanup(server.Close)
	f.URL = server.URL
	return f
}

// callCount is how many calls the fake had.
func (f *fakeFrankfurter) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}
