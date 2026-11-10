package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
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

// protectedResourceJSON is the protected resource metadata (RFC 9728) a
// Connector reads first.
type protectedResourceJSON struct {
	Resource             string   `json:"resource"`
	AuthorizationServers []string `json:"authorization_servers"`
}

// authorizationServerJSON is the authorization server metadata (RFC 8414) a
// Connector reads next, to find the OAuth endpoints.
type authorizationServerJSON struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	RegistrationEndpoint              string   `json:"registration_endpoint"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	GrantTypesSupported               []string `json:"grant_types_supported"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
}

// getMetadata GETs a /.well-known/ document as a Connector does, with no
// Session or Origin, and decodes it into a T.
func getMetadata[T any](n *nando, path string) T {
	n.t.Helper()
	rec := httptest.NewRecorder()
	n.handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != http.StatusOK {
		n.t.Fatalf("GET %s: status = %d, body %q", path, rec.Code, rec.Body)
	}
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		n.t.Fatalf("GET %s: %v in %q", path, err, rec.Body)
	}
	return v
}

func TestAConnectorFindsTheOAuthEndpointsFromTheMetadata(t *testing.T) {
	n := newNando(t)

	resource := getMetadata[protectedResourceJSON](n, "/.well-known/oauth-protected-resource")
	wantResource := protectedResourceJSON{
		Resource:             "https://api.example.com/mcp",
		AuthorizationServers: []string{"https://api.example.com"},
	}
	if !reflect.DeepEqual(resource, wantResource) {
		t.Errorf("protected resource metadata = %+v, want %+v", resource, wantResource)
	}

	server := getMetadata[authorizationServerJSON](n, "/.well-known/oauth-authorization-server")
	wantServer := authorizationServerJSON{
		Issuer:                            "https://api.example.com",
		AuthorizationEndpoint:             "https://api.example.com/oauth/authorize",
		TokenEndpoint:                     "https://api.example.com/oauth/token",
		RegistrationEndpoint:              "https://api.example.com/oauth/register",
		ResponseTypesSupported:            []string{"code"},
		GrantTypesSupported:               []string{"authorization_code", "refresh_token"},
		CodeChallengeMethodsSupported:     []string{"S256"},
		TokenEndpointAuthMethodsSupported: []string{"none"},
	}
	if !reflect.DeepEqual(server, wantServer) {
		t.Errorf("authorization server metadata = %+v,\nwant %+v", server, wantServer)
	}
}

// clientJSON is a Connector's registration (RFC 7591), as it is sent and
// as the API answers it.
type clientJSON struct {
	ClientID                string   `json:"client_id,omitempty"`
	ClientName              string   `json:"client_name,omitempty"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types,omitempty"`
	ResponseTypes           []string `json:"response_types,omitempty"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method,omitempty"`
	ClientSecret            string   `json:"client_secret,omitempty"`
}

// oauthErrorJSON is an OAuth error answer.
type oauthErrorJSON struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// claude is a registration like the Claude app's.
var claude = clientJSON{
	ClientName:              "Claude",
	RedirectURIs:            []string{"https://claude.ai/api/mcp/auth_callback"},
	GrantTypes:              []string{"authorization_code", "refresh_token"},
	ResponseTypes:           []string{"code"},
	TokenEndpointAuthMethod: "none",
}

// connectorDo sends a request as a Connector does: from its own server or a
// local app, with no Session or Origin.
func (n *nando) connectorDo(method, path string, body []byte) *httptest.ResponseRecorder {
	n.t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	n.handler.ServeHTTP(rec, req)
	return rec
}

// register registers client and returns the response.
func (n *nando) register(client any) *httptest.ResponseRecorder {
	n.t.Helper()
	return n.connectorDo(http.MethodPost, "/oauth/register", mustJSON(n.t, client))
}

// mustRegister registers client and returns the registration as answered.
func (n *nando) mustRegister(client clientJSON) clientJSON {
	n.t.Helper()
	rec := n.register(client)
	if rec.Code != http.StatusCreated {
		n.t.Fatalf("POST /oauth/register: status = %d, body %q", rec.Code, rec.Body)
	}
	var registered clientJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &registered); err != nil {
		n.t.Fatalf("POST /oauth/register: %v in %q", err, rec.Body)
	}
	return registered
}

func TestAConnectorRegistersAsAPublicClient(t *testing.T) {
	n := newNando(t)

	got := n.mustRegister(claude)

	if got.ClientID == "" {
		t.Errorf("registration has no client_id: %+v", got)
	}
	want := claude
	want.ClientID = got.ClientID
	if !reflect.DeepEqual(got, want) {
		t.Errorf("registration = %+v, want %+v", got, want)
	}
	if again := n.mustRegister(claude); again.ClientID == got.ClientID {
		t.Errorf("two registrations share client_id %q", got.ClientID)
	}
}

func TestAConnectorAskingForAClientSecretIsRegisteredAsAPublicClient(t *testing.T) {
	n := newNando(t)
	asked := claude
	asked.TokenEndpointAuthMethod = "client_secret_basic"

	got := n.mustRegister(asked)

	if got.TokenEndpointAuthMethod != "none" || got.ClientSecret != "" {
		t.Errorf("registration asking for a secret = %+v, want token_endpoint_auth_method none and no client_secret", got)
	}
}

func TestAConnectorWithoutANameIsNamedAfterItsRedirectHost(t *testing.T) {
	n := newNando(t)

	got := n.mustRegister(clientJSON{RedirectURIs: []string{"https://chat.example.org/oauth/callback"}})

	if got.ClientName != "chat.example.org" {
		t.Errorf("client_name = %q, want the redirect URI's host, chat.example.org", got.ClientName)
	}
}

func TestLocalAppsMayRedirectToLoopbackOverHTTP(t *testing.T) {
	n := newNando(t)

	for _, uri := range []string{"http://localhost:53682/callback", "http://127.0.0.1:8080/cb", "http://localhost/callback"} {
		got := n.mustRegister(clientJSON{ClientName: "Claude Code", RedirectURIs: []string{uri}})
		if !reflect.DeepEqual(got.RedirectURIs, []string{uri}) {
			t.Errorf("redirect_uris = %q, want [%q]", got.RedirectURIs, uri)
		}
	}
}

func TestRegistrationsWithAnUntrustworthyRedirectURIAreRejected(t *testing.T) {
	n := newNando(t)

	for _, uris := range [][]string{
		nil,
		{},
		{"http://example.com/callback"}, // plain HTTP off the device
		{"http://localhost.example.com/callback"}, // only looks local
		{"http://[::1]:8080/callback"},            // not one of the two loopback names
		{"ftp://example.com/callback"},
		{"javascript:alert(1)"},
		{"/callback"},
		{"https:///callback"},
		{"https://claude.ai/callback#fragment"},
		{"https://user@claude.ai/callback"},
		{"https://claude.ai/api/mcp/auth_callback", "http://example.com/callback"},
	} {
		rec := n.register(clientJSON{ClientName: "Claude", RedirectURIs: uris})
		var got oauthErrorJSON
		_ = json.Unmarshal(rec.Body.Bytes(), &got)
		if rec.Code != http.StatusBadRequest || got.Error != "invalid_redirect_uri" {
			t.Errorf("register with redirect_uris %q: status = %d, body %q; want 400 invalid_redirect_uri", uris, rec.Code, rec.Body)
		}
	}
}

func TestInvalidRegistrationsAreRejected(t *testing.T) {
	n := newNando(t)
	tooMany := make([]string, 11)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("https://claude.ai/callback/%d", i)
	}

	for name, body := range map[string]any{
		"not JSON":       "{",
		"name too long":  clientJSON{ClientName: strings.Repeat("x", 101), RedirectURIs: claude.RedirectURIs},
		"too many URIs":  clientJSON{ClientName: "Claude", RedirectURIs: tooMany},
		"implicit grant": clientJSON{ClientName: "Claude", RedirectURIs: claude.RedirectURIs, GrantTypes: []string{"implicit"}},
		"token response": clientJSON{ClientName: "Claude", RedirectURIs: claude.RedirectURIs, ResponseTypes: []string{"token"}},
	} {
		rec := n.register(body)
		var got oauthErrorJSON
		_ = json.Unmarshal(rec.Body.Bytes(), &got)
		if rec.Code != http.StatusBadRequest || got.Error != "invalid_client_metadata" {
			t.Errorf("%s: status = %d, body %q; want 400 invalid_client_metadata", name, rec.Code, rec.Body)
		}
	}
}
