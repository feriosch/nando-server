package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/feriosch/nando-server/internal/server"
)

func newHandler() http.Handler {
	return server.New(server.Config{
		AllowedOrigins: []string{"https://web.example.com", "http://localhost:5173"},
	})
}

func TestHealthReportsOK(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /health: status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestAllowedOriginMayCallWithCredentials(t *testing.T) {
	for _, origin := range []string{"https://web.example.com", "http://localhost:5173"} {
		t.Run(origin, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/health", nil)
			req.Header.Set("Origin", origin)
			rec := httptest.NewRecorder()
			newHandler().ServeHTTP(rec, req)

			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != origin {
				t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, origin)
			}
			if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
				t.Errorf("Access-Control-Allow-Credentials = %q, want %q", got, "true")
			}
		})
	}
}

func TestOtherOriginGetsNoCORSHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "https://evil.example.net")
	rec := httptest.NewRecorder()
	newHandler().ServeHTTP(rec, req)

	for _, h := range []string{"Access-Control-Allow-Origin", "Access-Control-Allow-Credentials"} {
		if got := rec.Header().Get(h); got != "" {
			t.Errorf("%s = %q, want none", h, got)
		}
	}
}

func TestAllowedOriginPreflightIsAnswered(t *testing.T) {
	req := httptest.NewRequest(http.MethodOptions, "/health", nil)
	req.Header.Set("Origin", "https://web.example.com")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "Content-Type")
	rec := httptest.NewRecorder()
	newHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight: status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://web.example.com" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "https://web.example.com")
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, http.MethodPost) {
		t.Errorf("Access-Control-Allow-Methods = %q, want it to include POST", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "Content-Type") {
		t.Errorf("Access-Control-Allow-Headers = %q, want it to include Content-Type", got)
	}
}

func TestPreflightIsCachedPerRequestedMethodAndHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodOptions, "/health", nil)
	req.Header.Set("Origin", "https://web.example.com")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	rec := httptest.NewRecorder()
	newHandler().ServeHTTP(rec, req)

	vary := strings.Join(rec.Header().Values("Vary"), ", ")
	for _, h := range []string{"Origin", "Access-Control-Request-Method", "Access-Control-Request-Headers"} {
		if !strings.Contains(vary, h) {
			t.Errorf("Vary = %q, want it to include %s", vary, h)
		}
	}
}
