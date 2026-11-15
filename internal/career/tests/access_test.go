package tests

import (
	"net/http"
	"testing"
)

func TestTheAppsCareerRoutesNeedASession(t *testing.T) {
	_, phone := signedInBrowser(t)
	_, recruiters := fileThreads(t, phone, thread("tanaka", message("t1", "other", at(-3, 10, 15), "Hello!")))
	ids := seed(t, phone, seededOpportunity{role: "Backend Engineer", interest: "normal", stage: "introduced"})
	stranger := &browser{nando: phone.nando}

	for _, path := range []string{
		"/api/career/home-card",
		"/api/career",
		"/api/career/opportunities",
		"/api/career/opportunities/" + ids["Backend Engineer"],
		"/api/career/recruiters/" + recruiters["tanaka"],
		"/api/career/late-replies",
	} {
		if rec := stranger.do(http.MethodGet, path, nil); rec.Code != http.StatusUnauthorized {
			t.Errorf("GET %s without a Session: status = %d, want %d", path, rec.Code, http.StatusUnauthorized)
		}
	}
}
