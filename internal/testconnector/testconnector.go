// Package testconnector is a test helper: a Connector the Owner approves with
// a Passkey and that calls /mcp with the official MCP client, so a Module's
// tests can call its Tools as Claude does. Core's own tests check each OAuth
// step; this only walks through them.
package testconnector

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/feriosch/nando-server/internal/testpasskey"
)

const (
	redirectURI = "https://connector.example.com/callback"
	// verifier is the PKCE code verifier the Connector makes up.
	verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
)

// Connect registers a Connector with handler, the whole API as main wires
// it; has the Owner approve it with passkey on the web app at origin;
// exchanges its code for tokens; and returns an MCP client session over real
// HTTP with its access token, closed when the test ends.
func Connect(t *testing.T, handler http.Handler, passkey *testpasskey.Passkey, origin string) *mcp.ClientSession {
	t.Helper()
	token := approve(t, handler, passkey, origin)

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	client := mcp.NewClient(&mcp.Implementation{Name: "test-connector", Version: "1.0"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:             srv.URL + "/mcp",
		HTTPClient:           &http.Client{Transport: bearer(token)},
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatalf("connect to /mcp: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// approve takes a new Connector through OAuth and returns its access token.
func approve(t *testing.T, handler http.Handler, passkey *testpasskey.Passkey, origin string) string {
	t.Helper()
	var client struct {
		ClientID string `json:"client_id"`
	}
	decode(t, send(t, handler, http.MethodPost, "/oauth/register", "", "application/json",
		`{"client_name":"Test Connector","redirect_uris":["`+redirectURI+`"]}`, http.StatusCreated), &client)

	sum := sha256.Sum256([]byte(verifier))
	authorize := url.Values{
		"response_type":         {"code"},
		"client_id":             {client.ClientID},
		"redirect_uri":          {redirectURI},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(sum[:])},
		"code_challenge_method": {"S256"},
		"state":                 {"test"},
	}
	consent, err := url.Parse(send(t, handler, http.MethodGet, "/oauth/authorize?"+authorize.Encode(), "", "", "", http.StatusFound).Header().Get("Location"))
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	request := consent.Query().Get("request")

	options := send(t, handler, http.MethodPost, "/auth/connect/"+request+"/begin", origin, "", "", http.StatusOK)
	answer := passkey.Get(t, origin, options.Body.Bytes())
	var approved struct {
		Redirect string `json:"redirect"`
	}
	decode(t, send(t, handler, http.MethodPost, "/auth/connect/"+request+"/finish", origin, "application/json", string(answer), http.StatusOK), &approved)
	back, err := url.Parse(approved.Redirect)
	if err != nil {
		t.Fatalf("approval redirect %q: %v", approved.Redirect, err)
	}

	exchange := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {back.Query().Get("code")},
		"redirect_uri":  {redirectURI},
		"client_id":     {client.ClientID},
		"code_verifier": {verifier},
	}
	var tokens struct {
		AccessToken string `json:"access_token"`
	}
	decode(t, send(t, handler, http.MethodPost, "/oauth/token", "", "application/x-www-form-urlencoded", exchange.Encode(), http.StatusOK), &tokens)
	return tokens.AccessToken
}

// send makes one request to handler, from origin if it isn't "", and fails
// the test unless it answers want.
func send(t *testing.T, handler http.Handler, method, path, origin, contentType, body string, want int) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != want {
		t.Fatalf("%s %s: status = %d, body %q; want %d", method, path, rec.Code, rec.Body, want)
	}
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.NewDecoder(bytes.NewReader(rec.Body.Bytes())).Decode(v); err != nil {
		t.Fatalf("%v in %q", err, rec.Body)
	}
}

// bearer sends every request with a Connector's access token.
type bearer string

func (b bearer) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+string(b))
	return http.DefaultTransport.RoundTrip(req)
}
