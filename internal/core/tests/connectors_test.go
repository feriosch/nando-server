package tests

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// protectedResourceMetadata is where a Connector learns how to get a token
// for /mcp, on the API's own host under the domain example.com.
const protectedResourceMetadata = "https://api.example.com/.well-known/oauth-protected-resource"

func TestMCPWithoutAConnectorTokenIsUnauthorized(t *testing.T) {
	n := newNando(t)

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	n.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("POST /mcp without a token: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	want := `Bearer resource_metadata="` + protectedResourceMetadata + `"`
	if got := rec.Header().Get("WWW-Authenticate"); got != want {
		t.Errorf("WWW-Authenticate = %q, want %q", got, want)
	}
}
