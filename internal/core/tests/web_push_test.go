package tests

import (
	"net/http"
	"testing"
	"time"
)

// Web Push is retired: Notifications go through the Messenger only (#128, #137).
func TestThePushSubscriptionRoutesAreGone(t *testing.T) {
	n, phone := signedInBrowser(t, time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC))
	_ = n

	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/push-subscriptions/vapid-public-key"},
		{http.MethodPost, "/api/push-subscriptions"},
		{http.MethodDelete, "/api/push-subscriptions?endpoint=https%3A%2F%2Fweb.push.apple.com%2FQGxt"},
		{http.MethodPost, "/api/push-subscriptions/test"},
	} {
		if rec := phone.do(route.method, route.path, []byte(`{}`)); rec.Code != http.StatusNotFound {
			t.Errorf("%s %s: status = %d, want 404", route.method, route.path, rec.Code)
		}
	}
}
