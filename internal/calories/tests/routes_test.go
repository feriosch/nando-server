package tests

import (
	"net/http"
	"strings"
	"testing"
)

// The web app's Calories only reads (feriosch/nando-web#89): the Calories
// Assistant writes through the Tools, so the write routes are gone (#141).
func TestTheWebAppCanNoLongerWriteCalories(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	entry := map[string]any{"name": "Onigiri", "kcalPerServing": 230, "servings": 1}
	meal := map[string]any{"name": "Onigiri", "kcal": 230}

	for _, route := range []struct {
		method, path string
		body         any
		want         int // 405 where a GET stays on the same path
	}{
		{http.MethodPost, "/api/calories/days/2026-10-21/entries", entry, http.StatusMethodNotAllowed},
		{http.MethodPut, "/api/calories/entries/0123456789abcdef01234567", entry, http.StatusNotFound},
		{http.MethodDelete, "/api/calories/entries/0123456789abcdef01234567", nil, http.StatusNotFound},
		{http.MethodPost, "/api/calories/meals", meal, http.StatusMethodNotAllowed},
		{http.MethodPut, "/api/calories/meals/0123456789abcdef01234567", meal, http.StatusNotFound},
		{http.MethodDelete, "/api/calories/meals/0123456789abcdef01234567", nil, http.StatusNotFound},
		{http.MethodPost, "/api/calories/meals/0123456789abcdef01234567/restore", nil, http.StatusNotFound},
		{http.MethodGet, "/api/calories/meals/quick", nil, http.StatusNotFound},
		{http.MethodGet, "/api/calories/days/2026-10-21/preview?kcal=500", nil, http.StatusNotFound},
		{http.MethodPut, "/api/calories/daily-limit", map[string]any{"kcal": 2000}, http.StatusMethodNotAllowed},
		{http.MethodPut, "/api/calories/settings", map[string]any{}, http.StatusMethodNotAllowed},
	} {
		rec := phone.do(route.method, route.path, route.body)
		if rec.Code != route.want {
			t.Errorf("%s %s: status = %d, want %d", route.method, route.path, rec.Code, route.want)
		}
		// The router's own 404, not a handler's for an ID it doesn't know.
		if route.want == http.StatusNotFound && !strings.Contains(rec.Body.String(), "404 page not found") {
			t.Errorf("%s %s: body %q; want no route at all", route.method, route.path, rec.Body)
		}
	}
}
